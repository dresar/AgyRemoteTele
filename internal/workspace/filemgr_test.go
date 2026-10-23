package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileMgr(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "filemgr-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	subDir := filepath.Join(tmpDir, "subdir")
	_ = os.Mkdir(subDir, 0755)

	filePath := filepath.Join(tmpDir, "sample.txt")
	_ = os.WriteFile(filePath, []byte("Hello Golang!"), 0644)

	items, err := ListDirectory(tmpDir)
	if err != nil {
		t.Fatalf("failed to list directory: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if !items[0].IsDir {
		t.Error("expected directory to be listed first")
	}

	content, truncated, err := ReadFile(filePath, 100)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if truncated {
		t.Error("expected truncated to be false")
	}
	if content != "Hello Golang!" {
		t.Errorf("content mismatch: %s", content)
	}

	if s := FormatSize(500); s != "500 B" {
		t.Errorf("expected 500 B, got %s", s)
	}
	if s := FormatSize(2048); s != "2.0 KB" {
		t.Errorf("expected 2.0 KB, got %s", s)
	}
}
