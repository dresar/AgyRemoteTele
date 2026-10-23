package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStoreOperations(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "session-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	filePath := filepath.Join(tmpDir, "sessions.json")
	store, err := NewStore(filePath, "agy01", "/opt/data/home")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	sess := store.GetOrCreate(1001, "testuser")
	if sess.ChatID != 1001 || sess.Username != "testuser" {
		t.Fatalf("session values mismatch: %+v", sess)
	}

	store.SetProfile(1001, "agy04")
	store.SetConversation(1001, "conv-123")
	store.SetWorkingDir(1001, "/tmp")
	store.Authenticate(1001)

	updated, ok := store.Get(1001)
	if !ok {
		t.Fatal("session not found")
	}

	if updated.ActiveProfile != "agy04" {
		t.Errorf("expected agy04, got %s", updated.ActiveProfile)
	}
	if updated.ActiveConversationID != "conv-123" {
		t.Errorf("expected conv-123, got %s", updated.ActiveConversationID)
	}
	if updated.ActiveWorkingDir != "/tmp" {
		t.Errorf("expected /tmp, got %s", updated.ActiveWorkingDir)
	}
	if !updated.IsAuthenticated {
		t.Error("expected authenticated to be true")
	}

	store.ClearConversation(1001)
	cleared, _ := store.Get(1001)
	if cleared.ActiveConversationID != "" {
		t.Errorf("expected empty conversation id, got %s", cleared.ActiveConversationID)
	}

	if store.IsExpired(updated, 1*time.Millisecond) {
		time.Sleep(2 * time.Millisecond)
		if !store.IsExpired(updated, 1*time.Millisecond) {
			t.Error("expected session to be expired")
		}
	}
}
