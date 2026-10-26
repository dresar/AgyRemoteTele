package watchdog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func createMockMeminfo(t *testing.T, dir string, totalKB, availKB uint64) string {
	content := fmt.Sprintf("MemTotal:       %d kB\nMemFree:        %d kB\nMemAvailable:   %d kB\nBuffers:        100 kB\nCached:         100 kB\n",
		totalKB, availKB/2, availKB)
	filePath := filepath.Join(dir, "meminfo")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mock meminfo: %v", err)
	}
	return filePath
}

func createMockToken(t *testing.T, root, profile string, expiry time.Time) {
	dir := filepath.Join(root, profile, ".gemini/antigravity-cli")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("failed to create profile dir: %v", err)
	}

	content := fmt.Sprintf(`{"token":{"expiry":"%s"}}`, expiry.Format(time.RFC3339))
	filePath := filepath.Join(dir, "antigravity-oauth-token")
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mock token: %v", err)
	}
}

func TestWatchdogMeminfoThresholds(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := AlertConfig{
		HighMemThreshold: 85.0,
		CritMemThreshold: 92.0,
	}
	w := NewWatchdog(cfg, nil)

	normalMem := createMockMeminfo(t, tmpDir, 10000, 3000)
	w.SetMeminfoPath(normalMem)
	alerts := w.CheckNow()
	for _, a := range alerts {
		if a.Metric == "RAM" {
			t.Errorf("expected no RAM alert for normal memory, got: %v", a)
		}
	}

	warnMem := createMockMeminfo(t, tmpDir, 10000, 1200)
	w.SetMeminfoPath(warnMem)
	alerts = w.CheckNow()
	foundWarn := false
	for _, a := range alerts {
		if a.Metric == "RAM" && a.Level == "WARNING" {
			foundWarn = true
		}
	}
	if !foundWarn {
		t.Errorf("expected RAM WARNING alert for 88%% usage")
	}

	critMem := createMockMeminfo(t, tmpDir, 10000, 500)
	w.SetMeminfoPath(critMem)
	alerts = w.CheckNow()
	foundCrit := false
	for _, a := range alerts {
		if a.Metric == "RAM" && a.Level == "CRITICAL" {
			foundCrit = true
		}
	}
	if !foundCrit {
		t.Errorf("expected RAM CRITICAL alert for 95%% usage")
	}
}

func TestWatchdogDiskUsage(t *testing.T) {
	cfg := AlertConfig{
		HighDiskThreshold: 99.99,
	}
	w := NewWatchdog(cfg, nil)
	w.SetDiskPath("/")

	alerts := w.checkDisk()
	if len(alerts) > 0 {
		t.Logf("got disk alert on /: %v", alerts[0].Message)
	}

	w.cfg.HighDiskThreshold = 0.01
	alerts = w.checkDisk()
	if len(alerts) == 0 {
		t.Errorf("expected disk alert when threshold is 0.01%%")
	}
}

func TestWatchdogTokenAlerts(t *testing.T) {
	tmpDir := t.TempDir()

	createMockToken(t, tmpDir, "agy01", time.Now().Add(2*time.Hour))
	createMockToken(t, tmpDir, "agy02", time.Now().Add(10*time.Minute))
	createMockToken(t, tmpDir, "agy03", time.Now().Add(-5*time.Minute))

	cfg := AlertConfig{
		TokenWarningMinutes: 15,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)

	alerts := w.checkTokens()
	if len(alerts) != 2 {
		t.Fatalf("expected 2 token alerts, got %d", len(alerts))
	}

	var hasWarn, hasCrit bool
	for _, a := range alerts {
		if a.Metric == "TOKEN_agy02" && a.Level == "WARNING" {
			hasWarn = true
		}
		if a.Metric == "TOKEN_agy03" && a.Level == "CRITICAL" {
			hasCrit = true
		}
	}

	if !hasWarn {
		t.Errorf("expected WARNING alert for agy02")
	}
	if !hasCrit {
		t.Errorf("expected CRITICAL alert for agy03")
	}
}

func TestWatchdogDeduplication(t *testing.T) {
	tmpDir := t.TempDir()
	critMem := createMockMeminfo(t, tmpDir, 10000, 500)

	var notifyCalls atomic.Int32
	notify := func(level, title, message string) {
		notifyCalls.Add(1)
	}

	cfg := AlertConfig{
		HighMemThreshold: 85.0,
		CritMemThreshold: 92.0,
	}
	w := NewWatchdog(cfg, notify)
	w.SetMeminfoPath(critMem)
	w.SetProfilesRoot(filepath.Join(tmpDir, "empty"))
	w.SetDiskPath(tmpDir)
	w.cfg.HighDiskThreshold = 99.99
	w.SetCooldown(10 * time.Minute)

	w.CheckNow()
	time.Sleep(50 * time.Millisecond)

	w.CheckNow()
	time.Sleep(50 * time.Millisecond)

	if notifyCalls.Load() != 1 {
		t.Errorf("expected notify to be called exactly 1 time due to deduplication, got %d", notifyCalls.Load())
	}
}

func TestWatchdogStartStop(t *testing.T) {
	tmpDir := t.TempDir()
	normalMem := createMockMeminfo(t, tmpDir, 10000, 5000)

	cfg := AlertConfig{
		CheckInterval: 50 * time.Millisecond,
	}
	w := NewWatchdog(cfg, nil)
	w.SetMeminfoPath(normalMem)
	w.SetProfilesRoot(filepath.Join(tmpDir, "empty"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	w.Stop()
}

func TestWatchdogGetRecentAlerts(t *testing.T) {
	tmpDir := t.TempDir()
	critMem := createMockMeminfo(t, tmpDir, 10000, 500)

	cfg := AlertConfig{
		CritMemThreshold: 90.0,
	}
	w := NewWatchdog(cfg, nil)
	w.SetMeminfoPath(critMem)
	w.SetProfilesRoot(filepath.Join(tmpDir, "empty"))
	w.SetDiskPath(tmpDir)
	w.cfg.HighDiskThreshold = 99.99

	w.CheckNow()

	recent := w.GetRecentAlerts()
	if len(recent) == 0 {
		t.Fatalf("expected recent alerts to contain events")
	}
	if recent[0].Metric != "RAM" {
		t.Errorf("expected RAM alert, got %s", recent[0].Metric)
	}
}
