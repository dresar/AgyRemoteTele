package system

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func TestListProcesses(t *testing.T) {
	procs, err := ListProcesses(10, "cpu")
	if err != nil {
		t.Fatalf("ListProcesses failed: %v", err)
	}

	if len(procs) == 0 {
		t.Fatal("expected at least one process")
	}

	if len(procs) > 10 {
		t.Fatalf("expected at most 10 processes, got %d", len(procs))
	}

	first := procs[0]
	if first.PID <= 0 {
		t.Errorf("invalid PID: %d", first.PID)
	}
	if first.State == "" {
		t.Error("expected non-empty state")
	}
	if first.Command == "" {
		t.Error("expected non-empty command")
	}

	memProcs, err := ListProcesses(5, "mem")
	if err != nil {
		t.Fatalf("ListProcesses mem sort failed: %v", err)
	}
	if len(memProcs) == 0 {
		t.Fatal("expected at least one process for mem sort")
	}

	pidProcs, err := ListProcesses(5, "pid")
	if err != nil {
		t.Fatalf("ListProcesses pid sort failed: %v", err)
	}
	if len(pidProcs) >= 2 && pidProcs[0].PID > pidProcs[1].PID {
		t.Errorf("pid sort order incorrect: %d > %d", pidProcs[0].PID, pidProcs[1].PID)
	}
}

func TestKillProcessSafety(t *testing.T) {
	if err := KillProcess(0, syscall.SIGTERM); err == nil {
		t.Error("expected error killing PID 0")
	}
	if err := KillProcess(1, syscall.SIGTERM); err == nil {
		t.Error("expected error killing PID 1")
	}
	if err := KillProcess(-5, syscall.SIGTERM); err == nil {
		t.Error("expected error killing negative PID")
	}
	if err := KillProcess(os.Getpid(), syscall.SIGTERM); err == nil {
		t.Error("expected error killing self PID")
	}
	if err := KillProcess(os.Getppid(), syscall.SIGTERM); err == nil {
		t.Error("expected error killing parent PID")
	}
	if err := KillProcess(2, syscall.SIGTERM); err == nil {
		t.Error("expected error killing kernel PID 2")
	}
}

func TestDecodeHexAddr(t *testing.T) {
	ip, port, err := decodeHexAddr("0100007F:4F11")
	if err != nil {
		t.Fatalf("decodeHexAddr failed: %v", err)
	}
	if ip != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", ip)
	}
	if port != 20241 {
		t.Errorf("expected 20241, got %d", port)
	}

	ip6, port6, err6 := decodeHexAddr("0000000000000000FFFF00000100007F:1F4B")
	if err6 != nil {
		t.Fatalf("decodeHexAddr IPv6 failed: %v", err6)
	}
	if ip6 != "127.0.0.1" && ip6 != "::ffff:127.0.0.1" {
		t.Errorf("expected 127.0.0.1 or ::ffff:127.0.0.1, got %s", ip6)
	}
	if port6 != 8011 {
		t.Errorf("expected 8011, got %d", port6)
	}
}

func TestListListeningPorts(t *testing.T) {
	ports, err := ListListeningPorts()
	if err != nil {
		t.Fatalf("ListListeningPorts failed: %v", err)
	}
	_ = ports
}

func TestListCommonServices(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	services, err := ListCommonServices(ctx)
	if err != nil {
		t.Fatalf("ListCommonServices failed: %v", err)
	}
	if len(services) == 0 {
		t.Fatal("expected common services list to not be empty")
	}

	foundKnown := false
	for _, s := range services {
		if s.Name == "docker" || s.Name == "cloudflared" || s.Name == "nginx" || s.Name == "cron" {
			foundKnown = true
			break
		}
	}
	if !foundKnown {
		t.Error("expected to find known common services in result")
	}
}

func TestParseSystemctlShow(t *testing.T) {
	mockOutput := "ActiveState=active\nSubState=running\nDescription=Docker Container Engine\n"
	info := parseSystemctlShow("docker", mockOutput)
	if info.Name != "docker" {
		t.Errorf("expected docker, got %s", info.Name)
	}
	if !info.Active {
		t.Error("expected active true")
	}
	if info.Status != "active" {
		t.Errorf("expected active, got %s", info.Status)
	}
	if info.SubState != "running" {
		t.Errorf("expected running, got %s", info.SubState)
	}
	if info.Description != "Docker Container Engine" {
		t.Errorf("expected Docker Container Engine, got %s", info.Description)
	}
}

func TestRestartServiceWhitelist(t *testing.T) {
	ctx := context.Background()

	_, err := RestartService(ctx, "malicious_service")
	if err == nil {
		t.Error("expected error for non-whitelisted service")
	}

	_, errEmpty := RestartService(ctx, "")
	if errEmpty == nil {
		t.Error("expected error for empty service name")
	}

	_, errInjection := RestartService(ctx, "docker; rm -rf /")
	if errInjection == nil {
		t.Error("expected error for injection attempt")
	}
}
