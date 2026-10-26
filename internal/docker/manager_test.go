package docker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDockerDaemonGracefulHandling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	_, errList := ListContainers(ctx, false)
	if errList == nil {
		t.Log("docker daemon appears to be active; ListContainers succeeded")
	} else {
		if !strings.Contains(strings.ToLower(errList.Error()), "daemon") && !strings.Contains(strings.ToLower(errList.Error()), "docker") {
			t.Errorf("unexpected error format: %v", errList)
		}
	}

	_, errLogs := GetContainerLogs(ctx, "test_container_nonexistent", 10)
	if errLogs == nil {
		t.Log("logs succeeded unexpectedly")
	}

	errRestart := RestartContainer(ctx, "test_container_nonexistent")
	if errRestart == nil {
		t.Log("restart succeeded unexpectedly")
	}

	_, errStats := GetContainerStats(ctx)
	if errStats == nil {
		t.Log("stats succeeded unexpectedly")
	}
}

func TestValidationRules(t *testing.T) {
	ctx := context.Background()

	if err := RestartContainer(ctx, ""); err == nil {
		t.Error("expected error for empty container ID")
	}

	if err := RestartContainer(ctx, "-flag"); err == nil {
		t.Error("expected error for flag injection")
	}

	if err := RestartContainer(ctx, "cid; rm -rf /"); err == nil {
		t.Error("expected error for shell injection")
	}

	if _, err := GetContainerLogs(ctx, "", 10); err == nil {
		t.Error("expected error for empty container ID in logs")
	}

	if _, err := GetContainerLogs(ctx, "id & whoami", 10); err == nil {
		t.Error("expected error for command injection in logs")
	}
}

func TestParseContainerInfoLines(t *testing.T) {
	mockJSON := `{"ID":"1a2b3c4d5e6f","Names":"web-app","Image":"nginx:alpine","Status":"Up 2 hours","State":"running","Ports":"0.0.0.0:80->80/tcp","CreatedAt":"2026-10-26 01:00:00"}
{"ID":"9z8y7x6w5v4u","Names":"db","Image":"postgres:16","Status":"Exited (0) 1 hour ago","State":"exited","Ports":"","CreatedAt":"2026-10-26 00:00:00"}
`
	list, err := parseContainerInfoLines(mockJSON)
	if err != nil {
		t.Fatalf("parseContainerInfoLines failed: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(list))
	}

	c1 := list[0]
	if c1.ID != "1a2b3c4d5e6f" {
		t.Errorf("expected ID 1a2b3c4d5e6f, got %s", c1.ID)
	}
	if c1.Names != "web-app" {
		t.Errorf("expected Names web-app, got %s", c1.Names)
	}
	if c1.Image != "nginx:alpine" {
		t.Errorf("expected Image nginx:alpine, got %s", c1.Image)
	}
	if c1.Status != "Up 2 hours" {
		t.Errorf("expected Status Up 2 hours, got %s", c1.Status)
	}
	if c1.State != "running" {
		t.Errorf("expected State running, got %s", c1.State)
	}
	if c1.Ports != "0.0.0.0:80->80/tcp" {
		t.Errorf("expected Ports 0.0.0.0:80->80/tcp, got %s", c1.Ports)
	}
	if c1.Created != "2026-10-26 01:00:00" {
		t.Errorf("expected Created 2026-10-26 01:00:00, got %s", c1.Created)
	}

	c2 := list[1]
	if c2.ID != "9z8y7x6w5v4u" || c2.Names != "db" || c2.State != "exited" {
		t.Errorf("c2 mismatch: %+v", c2)
	}
}

func TestParseContainerStatsLines(t *testing.T) {
	mockStats := `{"ID":"1a2b3c4d5e6f","Name":"web-app","CPUPerc":"0.50%","MemUsage":"15.2MiB / 3.8GiB","MemPerc":"0.39%","NetIO":"1.2kB / 5.6kB","BlockIO":"0B / 0B"}
{"ID":"9z8y7x6w5v4u","Name":"db","CPUPerc":"1.20%","MemUsage":"120MiB / 3.8GiB","MemPerc":"3.12%","NetIO":"10kB / 20kB","BlockIO":"1MB / 500kB"}
`
	list, err := parseContainerStatsLines(mockStats)
	if err != nil {
		t.Fatalf("parseContainerStatsLines failed: %v", err)
	}

	if len(list) != 2 {
		t.Fatalf("expected 2 stats entries, got %d", len(list))
	}

	s1 := list[0]
	if s1.ID != "1a2b3c4d5e6f" {
		t.Errorf("expected ID 1a2b3c4d5e6f, got %s", s1.ID)
	}
	if s1.Name != "web-app" {
		t.Errorf("expected Name web-app, got %s", s1.Name)
	}
	if s1.CPUPerc != "0.50%" {
		t.Errorf("expected CPUPerc 0.50%%, got %s", s1.CPUPerc)
	}
	if s1.MemUsage != "15.2MiB / 3.8GiB" {
		t.Errorf("expected MemUsage 15.2MiB / 3.8GiB, got %s", s1.MemUsage)
	}
	if s1.MemPerc != "0.39%" {
		t.Errorf("expected MemPerc 0.39%%, got %s", s1.MemPerc)
	}
	if s1.NetIO != "1.2kB / 5.6kB" {
		t.Errorf("expected NetIO 1.2kB / 5.6kB, got %s", s1.NetIO)
	}
	if s1.BlockIO != "0B / 0B" {
		t.Errorf("expected BlockIO 0B / 0B, got %s", s1.BlockIO)
	}
}

func TestHandleDockerError(t *testing.T) {
	err := handleDockerError("ps", []byte("Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?"), nil)
	if !strings.Contains(err.Error(), "docker daemon is not running") {
		t.Errorf("expected daemon not running error, got %v", err)
	}

	errGeneric := handleDockerError("logs", []byte("Error response from daemon: No such container: abc"), nil)
	if !strings.Contains(errGeneric.Error(), "docker logs failed") {
		t.Errorf("expected docker logs failed, got %v", errGeneric)
	}
}
