package cluster

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewSwarmManager(t *testing.T) {
	sm1 := NewSwarmManager("", "")
	if sm1.profilesRoot != "/opt/data/home/agy-profiles" {
		t.Errorf("expected default profiles root, got %s", sm1.profilesRoot)
	}
	if sm1.orchestratorPy != "/opt/data/home/agy-profiles/orchestrator.py" {
		t.Errorf("expected default orchestratorPy, got %s", sm1.orchestratorPy)
	}

	sm2 := NewSwarmManager("/custom/root", "/custom/orch.py")
	if sm2.profilesRoot != "/custom/root" {
		t.Errorf("expected custom profiles root, got %s", sm2.profilesRoot)
	}
	if sm2.orchestratorPy != "/custom/orch.py" {
		t.Errorf("expected custom orchestratorPy, got %s", sm2.orchestratorPy)
	}
}

func TestExtractTaskID(t *testing.T) {
	output1 := `🚀 Task didispatch ke worker: agy02 (PID: 12345)
   Task ID  : agy02_1792985000
   Prompt   : test prompt...`
	if id := extractTaskID(output1); id != "agy02_1792985000" {
		t.Errorf("expected task id agy02_1792985000, got %s", id)
	}

	output2 := `Task ID: custom_id_999`
	if id := extractTaskID(output2); id != "custom_id_999" {
		t.Errorf("expected custom_id_999, got %s", id)
	}

	output3 := `Some random output without task id`
	if id := extractTaskID(output3); id != "" {
		t.Errorf("expected empty task id, got %s", id)
	}
}

func TestParseWorkersOutput(t *testing.T) {
	table := `==============================================================================
PROFIL   | REMOTE-ID  | AUTH       | DAEMON       | EMAIL GOOGLE
------------------------------------------------------------------------------
agy01    | agy01-eka  | ✅ Aktif    | 🟢 PID 1509  | eka@gmail.com
agy02    | eka-2      | ⚪ Belum Login | ⚪ Mati       | -
==============================================================================`

	res := parseWorkersOutput(table)
	if len(res) != 2 {
		t.Fatalf("expected 2 profiles parsed, got %d", len(res))
	}
	if !strings.Contains(res["agy01"], "Aktif") {
		t.Errorf("expected agy01 to be Aktif, got %s", res["agy01"])
	}
	if !strings.Contains(res["agy02"], "Belum Login") {
		t.Errorf("expected agy02 to be Belum Login, got %s", res["agy02"])
	}
}

func TestBroadcastPromptMock(t *testing.T) {
	tmpDir := t.TempDir()
	mockPy := filepath.Join(tmpDir, "mock_orchestrator.py")

	script := `import sys
account = sys.argv[2]
print(f"🚀 Task didispatch ke worker: {account}")
print(f"   Task ID  : mock_{account}_98765")
print("Done")
`
	if err := os.WriteFile(mockPy, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock orchestrator script: %v", err)
	}

	sm := NewSwarmManager(tmpDir, mockPy)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	results, err := sm.BroadcastPrompt(ctx, []string{"agy03", "agy02"}, "check status", "/tmp")
	if err != nil {
		t.Fatalf("BroadcastPrompt failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].Account != "agy02" || results[1].Account != "agy03" {
		t.Errorf("expected results sorted by account, got %s and %s", results[0].Account, results[1].Account)
	}

	if results[0].TaskID != "mock_agy02_98765" {
		t.Errorf("expected task ID mock_agy02_98765, got %s", results[0].TaskID)
	}
	if results[1].TaskID != "mock_agy03_98765" {
		t.Errorf("expected task ID mock_agy03_98765, got %s", results[1].TaskID)
	}

	if results[0].Error != nil || results[1].Error != nil {
		t.Errorf("expected no error, got %v / %v", results[0].Error, results[1].Error)
	}
}

func TestBroadcastPromptTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	mockPy := filepath.Join(tmpDir, "slow_orchestrator.py")

	script := `import time
time.sleep(2)
`
	if err := os.WriteFile(mockPy, []byte(script), 0755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}

	sm := NewSwarmManager(tmpDir, mockPy)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	results, err := sm.BroadcastPrompt(ctx, []string{"agy04"}, "test", "")
	if err != nil {
		t.Fatalf("BroadcastPrompt unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Error == nil {
		t.Errorf("expected error due to timeout context")
	}
}

func TestQuickStatusDirectInspection(t *testing.T) {
	tmpDir := t.TempDir()

	for _, name := range []string{"agy01", "agy02"} {
		pdir := filepath.Join(tmpDir, name, ".gemini/antigravity-cli")
		if err := os.MkdirAll(pdir, 0755); err != nil {
			t.Fatalf("failed to create dir: %v", err)
		}
	}

	token1 := filepath.Join(tmpDir, "agy01", ".gemini/antigravity-cli/antigravity-oauth-token")
	expFuture := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
	_ = os.WriteFile(token1, []byte(fmt.Sprintf(`{"token":{"expiry":"%s"}}`, expFuture)), 0644)

	pid1 := filepath.Join(tmpDir, "agy01", ".gemini/antigravity-cli/antigravity-cli-daemon.pid")
	_ = os.WriteFile(pid1, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)

	token2 := filepath.Join(tmpDir, "agy02", ".gemini/antigravity-cli/antigravity-oauth-token")
	expPast := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	_ = os.WriteFile(token2, []byte(fmt.Sprintf(`{"token":{"expiry":"%s"}}`, expPast)), 0644)

	sm := NewSwarmManager(tmpDir, filepath.Join(tmpDir, "missing.py"))
	statuses, err := sm.QuickStatus(context.Background())
	if err != nil {
		t.Fatalf("QuickStatus failed: %v", err)
	}

	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}

	if !strings.Contains(statuses["agy01"], "active") || !strings.Contains(statuses["agy01"], "running") {
		t.Errorf("expected agy01 to be active and running, got %s", statuses["agy01"])
	}

	if !strings.Contains(statuses["agy02"], "expired") {
		t.Errorf("expected agy02 to be expired, got %s", statuses["agy02"])
	}
}
