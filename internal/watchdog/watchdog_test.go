package watchdog

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	w.SetProfilesRoot(filepath.Join(tmpDir, "empty"))

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
	w.AutoRefreshFunc = nil
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

func TestWatchdogCanonicalProfileFiltering(t *testing.T) {
	tmpDir := t.TempDir()

	createMockToken(t, tmpDir, "agy01", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, "account1", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, "account01", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, "account10", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, "ORCHESTRATOR", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, "random_dir", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, ".git", time.Now().Add(-10*time.Minute))
	createMockToken(t, tmpDir, "orchestrator_tasks", time.Now().Add(-10*time.Minute))

	cfg := AlertConfig{
		TokenWarningMinutes: 15,
	}
	w := NewWatchdog(cfg, nil)
	w.AutoRefreshFunc = nil
	w.SetProfilesRoot(tmpDir)

	alerts := w.checkTokens()
	if len(alerts) != 1 {
		t.Fatalf("expected exactly 1 alert for agy01, got %d alerts: %v", len(alerts), alerts)
	}

	if alerts[0].Metric != "TOKEN_agy01" {
		t.Errorf("expected alert for TOKEN_agy01, got %s", alerts[0].Metric)
	}
}

func TestWatchdogAutoRefreshSuccess(t *testing.T) {
	tmpDir := t.TempDir()

	createMockToken(t, tmpDir, "agy02", time.Now().Add(5*time.Minute))

	cfg := AlertConfig{
		TokenWarningMinutes: 15,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)

	var refreshedProfile string
	w.AutoRefreshFunc = func(name string) error {
		refreshedProfile = name
		createMockToken(t, tmpDir, name, time.Now().Add(2*time.Hour))
		return nil
	}

	alerts := w.checkTokens()
	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts after successful auto-refresh, got %d: %v", len(alerts), alerts)
	}

	if refreshedProfile != "agy02" {
		t.Errorf("expected agy02 to be refreshed, got %s", refreshedProfile)
	}
}

func TestWatchdogAutoRefreshFailure(t *testing.T) {
	tmpDir := t.TempDir()

	createMockToken(t, tmpDir, "agy04", time.Now().Add(-5*time.Minute))

	cfg := AlertConfig{
		TokenWarningMinutes: 15,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)

	w.AutoRefreshFunc = func(name string) error {
		return fmt.Errorf("oauth token refresh rejected")
	}

	alerts := w.checkTokens()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert on refresh failure, got %d", len(alerts))
	}

	if alerts[0].Level != "CRITICAL" {
		t.Errorf("expected CRITICAL level, got %s", alerts[0].Level)
	}

	if !strings.Contains(alerts[0].Message, "Token refresh failed for agy04") {
		t.Errorf("expected failure message, got: %s", alerts[0].Message)
	}
}

func TestWatchdogAutoRefreshStillExpired(t *testing.T) {
	tmpDir := t.TempDir()

	createMockToken(t, tmpDir, "agy05", time.Now().Add(-5*time.Minute))

	cfg := AlertConfig{
		TokenWarningMinutes: 15,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)

	w.AutoRefreshFunc = func(name string) error {
		return nil
	}

	alerts := w.checkTokens()
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert when token is still expired, got %d", len(alerts))
	}

	if alerts[0].Level != "CRITICAL" {
		t.Errorf("expected CRITICAL level, got %s", alerts[0].Level)
	}

	if !strings.Contains(alerts[0].Message, "Token refresh failed for agy05") {
		t.Errorf("expected failure message, got: %s", alerts[0].Message)
	}
}

func TestWatchdogAutoRefreshActiveRemainingNoAlert(t *testing.T) {
	tmpDir := t.TempDir()

	createMockToken(t, tmpDir, "agy02", time.Now().Add(4*time.Minute))

	cfg := AlertConfig{
		TokenWarningMinutes: 5,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)

	w.mu.Lock()
	w.lastNotified["TOKEN_agy02:WARNING"] = time.Now().Add(-1 * time.Minute)
	w.lastNotified["TOKEN_agy02:CRITICAL"] = time.Now().Add(-1 * time.Minute)
	w.mu.Unlock()

	refreshCalled := false
	w.AutoRefreshFunc = func(name string) error {
		refreshCalled = true
		return nil
	}

	alerts := w.checkTokens()
	if !refreshCalled {
		t.Fatalf("expected auto-refresh to be invoked")
	}

	if len(alerts) != 0 {
		t.Fatalf("expected 0 alerts when token still has active remaining time, got %d: %v", len(alerts), alerts)
	}

	w.mu.RLock()
	_, hasWarn := w.lastNotified["TOKEN_agy02:WARNING"]
	_, hasCrit := w.lastNotified["TOKEN_agy02:CRITICAL"]
	w.mu.RUnlock()

	if hasWarn || hasCrit {
		t.Errorf("expected lastNotified to be cleared for agy02, got warn=%v crit=%v", hasWarn, hasCrit)
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

func TestWatchdogDefaultTokenCheckInterval(t *testing.T) {
	cfg := AlertConfig{}
	w := NewWatchdog(cfg, nil)
	if w.cfg.TokenCheckInterval != 24*time.Hour {
		t.Fatalf("expected default TokenCheckInterval 24h, got %v", w.cfg.TokenCheckInterval)
	}
}

func TestWatchdogTokenCheckDailyInterval(t *testing.T) {
	tmpDir := t.TempDir()
	createMockToken(t, tmpDir, "agy02", time.Now().Add(5*time.Minute))

	var refreshCount atomic.Int32
	cfg := AlertConfig{
		TokenCheckInterval:  24 * time.Hour,
		TokenWarningMinutes: 15,
		HighMemThreshold:    99.9,
		CritMemThreshold:    99.9,
		HighDiskThreshold:   99.9,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)
	w.SetDiskPath(tmpDir)
	memPath := createMockMeminfo(t, tmpDir, 10000, 9000)
	w.SetMeminfoPath(memPath)

	w.AutoRefreshFunc = func(name string) error {
		refreshCount.Add(1)
		return nil
	}

	w.CheckNow()
	if refreshCount.Load() != 1 {
		t.Fatalf("expected refreshCount 1 on initial CheckNow, got %d", refreshCount.Load())
	}

	for i := 0; i < 5; i++ {
		w.CheckNow()
	}
	if refreshCount.Load() != 1 {
		t.Fatalf("expected refreshCount still 1 after rapid CheckNow calls, got %d", refreshCount.Load())
	}

	w.mu.Lock()
	w.lastTokenCheck = time.Now().Add(-25 * time.Hour)
	w.mu.Unlock()

	w.CheckNow()
	if refreshCount.Load() != 2 {
		t.Fatalf("expected refreshCount 2 after interval elapsed, got %d", refreshCount.Load())
	}
}

func TestWatchdogForceCheckTokens(t *testing.T) {
	tmpDir := t.TempDir()
	createMockToken(t, tmpDir, "agy02", time.Now().Add(5*time.Minute))

	var refreshCount atomic.Int32
	cfg := AlertConfig{
		TokenCheckInterval:  24 * time.Hour,
		TokenWarningMinutes: 15,
		HighMemThreshold:    99.9,
		CritMemThreshold:    99.9,
		HighDiskThreshold:   99.9,
	}
	w := NewWatchdog(cfg, nil)
	w.SetProfilesRoot(tmpDir)
	w.SetDiskPath(tmpDir)
	memPath := createMockMeminfo(t, tmpDir, 10000, 9000)
	w.SetMeminfoPath(memPath)

	w.AutoRefreshFunc = func(name string) error {
		refreshCount.Add(1)
		return nil
	}

	w.CheckNow()
	if refreshCount.Load() != 1 {
		t.Fatalf("expected refreshCount 1 after initial CheckNow, got %d", refreshCount.Load())
	}

	w.CheckNow()
	if refreshCount.Load() != 1 {
		t.Fatalf("expected refreshCount still 1, got %d", refreshCount.Load())
	}

	w.ForceCheckTokens()
	if refreshCount.Load() != 2 {
		t.Fatalf("expected refreshCount 2 after ForceCheckTokens, got %d", refreshCount.Load())
	}
}
