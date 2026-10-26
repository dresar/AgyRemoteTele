package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var profileRegex = regexp.MustCompile(`^agy(0[1-9]|1[0-5])$`)

type SwarmTaskResult struct {
	Account  string        `json:"account"`
	TaskID   string        `json:"task_id"`
	Output   string        `json:"output"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"error,omitempty"`
}

type SwarmManager struct {
	profilesRoot   string
	orchestratorPy string
	pythonBin      string
}

func NewSwarmManager(profilesRoot string, orchestratorPy string) *SwarmManager {
	if profilesRoot == "" {
		profilesRoot = "/opt/data/home/agy-profiles"
	}
	if orchestratorPy == "" {
		orchestratorPy = "/opt/data/home/agy-profiles/orchestrator.py"
	}
	return &SwarmManager{
		profilesRoot:   profilesRoot,
		orchestratorPy: orchestratorPy,
		pythonBin:      "python3",
	}
}

func (sm *SwarmManager) SetPythonBin(bin string) {
	sm.pythonBin = bin
}

func (sm *SwarmManager) BroadcastPrompt(ctx context.Context, accounts []string, prompt string, workspace string) ([]SwarmTaskResult, error) {
	if len(accounts) == 0 {
		var err error
		accounts, err = sm.discoverAccounts()
		if err != nil || len(accounts) == 0 {
			accounts = []string{"agy02", "agy03", "agy04", "agy05", "agy06", "agy07", "agy08", "agy09", "agy10"}
		}
	}

	resultsChan := make(chan SwarmTaskResult, len(accounts))
	var wg sync.WaitGroup

	for _, acc := range accounts {
		wg.Add(1)
		go func(account string) {
			defer wg.Done()
			start := time.Now()

			args := []string{sm.orchestratorPy, "dispatch", account, prompt}
			if workspace != "" {
				args = append(args, "--workspace", workspace)
			}

			cmd := exec.CommandContext(ctx, sm.pythonBin, args...)
			out, err := cmd.CombinedOutput()
			outputStr := string(out)

			taskID := extractTaskID(outputStr)
			if taskID == "" {
				taskID = fmt.Sprintf("%s_%d", account, start.Unix())
			}

			resultsChan <- SwarmTaskResult{
				Account:  account,
				TaskID:   taskID,
				Output:   outputStr,
				Duration: time.Since(start),
				Error:    err,
			}
		}(acc)
	}

	wg.Wait()
	close(resultsChan)

	var results []SwarmTaskResult
	for r := range resultsChan {
		results = append(results, r)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Account < results[j].Account
	})

	return results, nil
}

func (sm *SwarmManager) QuickStatus(ctx context.Context) (map[string]string, error) {
	if _, err := os.Stat(sm.orchestratorPy); err == nil {
		cmd := exec.CommandContext(ctx, sm.pythonBin, sm.orchestratorPy, "workers")
		out, err := cmd.CombinedOutput()
		if err == nil {
			parsed := parseWorkersOutput(string(out))
			if len(parsed) > 0 {
				return parsed, nil
			}
		}
	}

	return sm.inspectProfilesDirectly()
}

func (sm *SwarmManager) discoverAccounts() ([]string, error) {
	entries, err := os.ReadDir(sm.profilesRoot)
	if err != nil {
		return nil, err
	}

	var accounts []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() && profileRegex.MatchString(name) {
			accounts = append(accounts, name)
		}
	}
	sort.Strings(accounts)
	return accounts, nil
}

func (sm *SwarmManager) inspectProfilesDirectly() (map[string]string, error) {
	entries, err := os.ReadDir(sm.profilesRoot)
	if err != nil {
		return nil, err
	}

	statusMap := make(map[string]string)
	now := time.Now()

	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() || !profileRegex.MatchString(name) {
			continue
		}

		pdir := filepath.Join(sm.profilesRoot, name)
		pidFile := filepath.Join(pdir, ".gemini/antigravity-cli/antigravity-cli-daemon.pid")
		tokenFile := filepath.Join(pdir, ".gemini/antigravity-cli/antigravity-oauth-token")

		daemonActive := false
		if pidBytes, err := os.ReadFile(pidFile); err == nil {
			pidStr := strings.TrimSpace(string(pidBytes))
			if pid, err := strconv.Atoi(pidStr); err == nil {
				if _, err := os.Stat(fmt.Sprintf("/proc/%d", pid)); err == nil {
					daemonActive = true
				}
			}
		}

		authStatus := "unauthenticated"
		if tokenBytes, err := os.ReadFile(tokenFile); err == nil {
			type tokDoc struct {
				Token struct {
					Expiry string `json:"expiry"`
				} `json:"token"`
			}
			var doc tokDoc
			if err := json.Unmarshal(tokenBytes, &doc); err == nil && doc.Token.Expiry != "" {
				if exp, err := time.Parse(time.RFC3339Nano, doc.Token.Expiry); err == nil {
					if exp.After(now) {
						authStatus = "active"
					} else {
						authStatus = "expired"
					}
				}
			}
		}

		if daemonActive {
			statusMap[name] = fmt.Sprintf("%s (daemon: running)", authStatus)
		} else {
			statusMap[name] = fmt.Sprintf("%s (daemon: stopped)", authStatus)
		}
	}

	return statusMap, nil
}

func extractTaskID(out string) string {
	for _, line := range strings.Split(out, "\n") {
		clean := strings.TrimSpace(line)
		lower := strings.ToLower(clean)
		if strings.HasPrefix(lower, "task id") {
			parts := strings.SplitN(clean, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

func parseWorkersOutput(out string) map[string]string {
	statusMap := make(map[string]string)
	lines := strings.Split(out, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "=") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, "PROFIL") {
			continue
		}
		parts := strings.Split(line, "|")
		if len(parts) >= 4 {
			profile := strings.TrimSpace(parts[0])
			auth := strings.TrimSpace(parts[2])
			daemon := strings.TrimSpace(parts[3])
			if profile != "" && profileRegex.MatchString(profile) {
				statusMap[profile] = fmt.Sprintf("%s, %s", auth, daemon)
			}
		}
	}

	return statusMap
}
