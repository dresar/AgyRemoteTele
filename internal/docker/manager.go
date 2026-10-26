package docker

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type ContainerInfo struct {
	ID      string
	Names   string
	Image   string
	Status  string
	State   string
	Ports   string
	Created string
}

type ContainerStats struct {
	ID       string
	Name     string
	CPUPerc  string
	MemUsage string
	MemPerc  string
	NetIO    string
	BlockIO  string
}

type dockerPSJSON struct {
	ID        string `json:"ID"`
	Names     string `json:"Names"`
	Name      string `json:"Name"`
	Image     string `json:"Image"`
	Status    string `json:"Status"`
	State     string `json:"State"`
	Ports     string `json:"Ports"`
	CreatedAt string `json:"CreatedAt"`
	Created   string `json:"Created"`
}

type dockerStatsJSON struct {
	ID        string `json:"ID"`
	Container string `json:"Container"`
	Name      string `json:"Name"`
	CPUPerc   string `json:"CPUPerc"`
	MemUsage  string `json:"MemUsage"`
	MemPerc   string `json:"MemPerc"`
	NetIO     string `json:"NetIO"`
	BlockIO   string `json:"BlockIO"`
}

func ListContainers(ctx context.Context, all bool) ([]ContainerInfo, error) {
	_, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker binary not found: %w", err)
	}

	args := []string{"ps", "--format", "{{json .}}"}
	if all {
		args = []string{"ps", "-a", "--format", "{{json .}}"}
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, handleDockerError("ps", out, err)
	}

	return parseContainerInfoLines(string(out))
}

func parseContainerInfoLines(raw string) ([]ContainerInfo, error) {
	var list []ContainerInfo
	scanner := bufio.NewScanner(strings.NewReader(raw))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var item dockerPSJSON
		if err := json.Unmarshal([]byte(line), &item); err == nil && (item.ID != "" || item.Names != "" || item.Image != "") {
			name := item.Names
			if name == "" {
				name = item.Name
			}
			created := item.CreatedAt
			if created == "" {
				created = item.Created
			}
			list = append(list, ContainerInfo{
				ID:      item.ID,
				Names:   name,
				Image:   item.Image,
				Status:  item.Status,
				State:   item.State,
				Ports:   item.Ports,
				Created: created,
			})
			continue
		}

		fields := strings.Split(line, "\t")
		if len(fields) >= 5 {
			list = append(list, ContainerInfo{
				ID:      strings.TrimSpace(fields[0]),
				Image:   strings.TrimSpace(fields[1]),
				Status:  strings.TrimSpace(fields[2]),
				Ports:   strings.TrimSpace(fields[3]),
				Names:   strings.TrimSpace(fields[4]),
				State:   "running",
				Created: "",
			})
		}
	}

	return list, scanner.Err()
}

func GetContainerLogs(ctx context.Context, containerID string, tailLines int) (string, error) {
	trimmedID := strings.TrimSpace(containerID)
	if trimmedID == "" {
		return "", fmt.Errorf("container ID cannot be empty")
	}

	if err := validateContainerID(trimmedID); err != nil {
		return "", err
	}

	if tailLines <= 0 {
		tailLines = 100
	}

	_, err := exec.LookPath("docker")
	if err != nil {
		return "", fmt.Errorf("docker binary not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "logs", "--tail", strconv.Itoa(tailLines), trimmedID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", handleDockerError("logs", out, err)
	}

	return string(out), nil
}

func RestartContainer(ctx context.Context, containerID string) error {
	trimmedID := strings.TrimSpace(containerID)
	if trimmedID == "" {
		return fmt.Errorf("container ID cannot be empty")
	}

	if err := validateContainerID(trimmedID); err != nil {
		return err
	}

	_, err := exec.LookPath("docker")
	if err != nil {
		return fmt.Errorf("docker binary not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "restart", trimmedID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return handleDockerError("restart", out, err)
	}

	return nil
}

func GetContainerStats(ctx context.Context) ([]ContainerStats, error) {
	_, err := exec.LookPath("docker")
	if err != nil {
		return nil, fmt.Errorf("docker binary not found: %w", err)
	}

	cmd := exec.CommandContext(ctx, "docker", "stats", "--no-stream", "--format", "{{json .}}")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, handleDockerError("stats", out, err)
	}

	return parseContainerStatsLines(string(out))
}

func parseContainerStatsLines(raw string) ([]ContainerStats, error) {
	var list []ContainerStats
	scanner := bufio.NewScanner(strings.NewReader(raw))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var item dockerStatsJSON
		if err := json.Unmarshal([]byte(line), &item); err == nil && (item.ID != "" || item.Container != "" || item.Name != "") {
			id := item.ID
			if id == "" {
				id = item.Container
			}
			list = append(list, ContainerStats{
				ID:       id,
				Name:     item.Name,
				CPUPerc:  item.CPUPerc,
				MemUsage: item.MemUsage,
				MemPerc:  item.MemPerc,
				NetIO:    item.NetIO,
				BlockIO:  item.BlockIO,
			})
		}
	}

	return list, scanner.Err()
}

func handleDockerError(action string, out []byte, err error) error {
	outStr := strings.TrimSpace(string(out))
	lower := strings.ToLower(outStr)

	if strings.Contains(lower, "cannot connect to the docker daemon") ||
		strings.Contains(lower, "is the docker daemon running") ||
		strings.Contains(lower, "docker.sock") ||
		strings.Contains(lower, "daemon is not running") {
		return fmt.Errorf("docker daemon is not running: %s", outStr)
	}

	if outStr != "" {
		return fmt.Errorf("docker %s failed: %s (%w)", action, outStr, err)
	}
	return fmt.Errorf("docker %s failed: %w", action, err)
}

func validateContainerID(id string) error {
	if strings.HasPrefix(id, "-") || strings.ContainsAny(id, " \t\r\n;\"'`$&|<>*?\\") {
		return fmt.Errorf("invalid container ID format: %s", id)
	}
	return nil
}
