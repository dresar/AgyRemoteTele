package system

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ServiceInfo struct {
	Name        string
	Status      string
	Active      bool
	SubState    string
	Description string
}

var commonServiceNames = []string{
	"docker",
	"cloudflared",
	"nginx",
	"cron",
	"ssh",
	"systemd-resolved",
}

var restartAllowlist = map[string]bool{
	"docker":      true,
	"nginx":       true,
	"cloudflared": true,
	"cron":        true,
}

func ListCommonServices(ctx context.Context) ([]ServiceInfo, error) {
	_, errSystemctl := exec.LookPath("systemctl")
	if errSystemctl == nil {
		return listServicesViaSystemctl(ctx, commonServiceNames)
	}

	return listServicesViaProc(commonServiceNames), nil
}

func listServicesViaSystemctl(ctx context.Context, names []string) ([]ServiceInfo, error) {
	var list []ServiceInfo

	for _, name := range names {
		cmd := exec.CommandContext(ctx, "systemctl", "show", name, "--property=ActiveState,SubState,Description")
		out, err := cmd.Output()
		if err != nil {
			activeCmd := exec.CommandContext(ctx, "systemctl", "is-active", name)
			activeOut, _ := activeCmd.Output()
			statusStr := strings.TrimSpace(string(activeOut))
			if statusStr == "" {
				statusStr = "unknown"
			}
			isActive := statusStr == "active"
			list = append(list, ServiceInfo{
				Name:        name,
				Status:      statusStr,
				Active:      isActive,
				SubState:    statusStr,
				Description: name + " service",
			})
			continue
		}

		info := parseSystemctlShow(name, string(out))
		list = append(list, info)
	}

	return list, nil
}

func parseSystemctlShow(name string, output string) ServiceInfo {
	activeState := "unknown"
	subState := "unknown"
	description := name + " service"

	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "ActiveState=") {
			activeState = strings.TrimPrefix(line, "ActiveState=")
		} else if strings.HasPrefix(line, "SubState=") {
			subState = strings.TrimPrefix(line, "SubState=")
		} else if strings.HasPrefix(line, "Description=") {
			desc := strings.TrimPrefix(line, "Description=")
			if desc != "" {
				description = desc
			}
		}
	}

	return ServiceInfo{
		Name:        name,
		Status:      activeState,
		Active:      activeState == "active",
		SubState:    subState,
		Description: description,
	}
}

func listServicesViaProc(names []string) []ServiceInfo {
	runningProcs := getRunningProcessNames()
	var list []ServiceInfo

	for _, name := range names {
		isActive := checkProcMatch(name, runningProcs)
		status := "inactive"
		subState := "dead"
		if isActive {
			status = "active"
			subState = "running"
		}
		list = append(list, ServiceInfo{
			Name:        name,
			Status:      status,
			Active:      isActive,
			SubState:    subState,
			Description: name + " service",
		})
	}

	return list
}

func checkProcMatch(name string, procs map[string]bool) bool {
	if procs[name] {
		return true
	}
	switch name {
	case "docker":
		return procs["dockerd"] || procs["containerd"]
	case "cron":
		return procs["crond"]
	case "ssh":
		return procs["sshd"]
	case "systemd-resolved":
		return procs["systemd-resolve"]
	}
	return false
}

func getRunningProcessNames() map[string]bool {
	m := make(map[string]bool)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return m
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue
		}

		commData, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
		if err == nil {
			name := strings.TrimSpace(string(commData))
			if name != "" {
				m[name] = true
			}
		}

		cmdBytes, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", pid))
		if err == nil && len(cmdBytes) > 0 {
			cmdBytes = bytes.ReplaceAll(cmdBytes, []byte{0}, []byte(" "))
			fields := strings.Fields(string(cmdBytes))
			if len(fields) > 0 {
				m[filepath.Base(fields[0])] = true
			}
		}
	}

	return m
}

func RestartService(ctx context.Context, serviceName string) (string, error) {
	cleanName := strings.TrimSpace(strings.ToLower(serviceName))
	if cleanName == "" {
		return "", fmt.Errorf("service name cannot be empty")
	}

	if !restartAllowlist[cleanName] {
		return "", fmt.Errorf("service %q is not in the allowed restart whitelist", serviceName)
	}

	_, errLook := exec.LookPath("systemctl")
	if errLook != nil {
		return "", fmt.Errorf("systemctl not available on this system")
	}

	cmd := exec.CommandContext(ctx, "systemctl", "restart", cleanName)
	out, err := cmd.CombinedOutput()
	if err != nil {
		outStr := strings.TrimSpace(string(out))
		if outStr != "" {
			return outStr, fmt.Errorf("failed to restart service %s: %s (%w)", cleanName, outStr, err)
		}
		return "", fmt.Errorf("failed to restart service %s: %w", cleanName, err)
	}

	return fmt.Sprintf("service %s restarted successfully", cleanName), nil
}
