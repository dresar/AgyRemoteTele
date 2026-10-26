package backup

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCreateTarGz(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	destArchive := filepath.Join(outDir, "backup.tar.gz")

	_ = os.WriteFile(filepath.Join(srcDir, "doc.txt"), []byte("hello world"), 0644)
	_ = os.WriteFile(filepath.Join(srcDir, "debug.log"), []byte("log data"), 0644)

	subDir := filepath.Join(srcDir, "nested")
	_ = os.MkdirAll(subDir, 0755)
	_ = os.WriteFile(filepath.Join(subDir, "inner.go"), []byte("package nested"), 0644)

	exDir := filepath.Join(srcDir, "node_modules", "pkg")
	_ = os.MkdirAll(exDir, 0755)
	_ = os.WriteFile(filepath.Join(exDir, "mod.js"), []byte("module"), 0644)

	res, err := CreateTarGz([]string{srcDir}, destArchive, []string{"*.log", "node_modules"})
	if err != nil {
		t.Fatalf("CreateTarGz failed: %v", err)
	}

	if res.ArchivePath != destArchive {
		t.Errorf("expected archive path %s, got %s", destArchive, res.ArchivePath)
	}
	if res.FileCount == 0 {
		t.Errorf("expected file count > 0")
	}
	if res.TotalSizeBytes == 0 {
		t.Errorf("expected total size > 0")
	}

	f, err := os.Open(destArchive)
	if err != nil {
		t.Fatalf("failed to open archive: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	foundFiles := make(map[string]bool)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read error: %v", err)
		}
		foundFiles[hdr.Name] = true
	}

	var hasDoc, hasInner, hasLog, hasModules bool
	for name := range foundFiles {
		if strings.Contains(name, "doc.txt") {
			hasDoc = true
		}
		if strings.Contains(name, "inner.go") {
			hasInner = true
		}
		if strings.Contains(name, "debug.log") {
			hasLog = true
		}
		if strings.Contains(name, "node_modules") || strings.Contains(name, "mod.js") {
			hasModules = true
		}
	}

	if !hasDoc {
		t.Errorf("doc.txt missing from archive")
	}
	if !hasInner {
		t.Errorf("inner.go missing from archive")
	}
	if hasLog {
		t.Errorf("debug.log should have been excluded")
	}
	if hasModules {
		t.Errorf("node_modules should have been excluded")
	}
}

func TestCreateTarGzSingleFile(t *testing.T) {
	srcDir := t.TempDir()
	outDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "single.txt")
	_ = os.WriteFile(srcFile, []byte("single file data"), 0644)

	destArchive := filepath.Join(outDir, "single.tar.gz")
	res, err := CreateTarGz([]string{srcFile}, destArchive, nil)
	if err != nil {
		t.Fatalf("failed to archive single file: %v", err)
	}

	if res.FileCount != 1 {
		t.Errorf("expected 1 file, got %d", res.FileCount)
	}
}

func TestCreateTarGzValidation(t *testing.T) {
	tmpDir := t.TempDir()
	dest := filepath.Join(tmpDir, "out.tar.gz")

	_, err := CreateTarGz(nil, dest, nil)
	if err == nil {
		t.Errorf("expected error on empty sources")
	}

	_, err = CreateTarGz([]string{tmpDir}, "", nil)
	if err == nil {
		t.Errorf("expected error on empty destFile")
	}

	_, err = CreateTarGz([]string{filepath.Join(tmpDir, "nonexistent")}, dest, nil)
	if err == nil {
		t.Errorf("expected error on nonexistent source")
	}
}

func TestListBackups(t *testing.T) {
	tmpDir := t.TempDir()

	b1 := filepath.Join(tmpDir, "backup_old.tar.gz")
	b2 := filepath.Join(tmpDir, "backup_new.tar.gz")
	b3 := filepath.Join(tmpDir, "backup_mid.tgz")
	nonBackup := filepath.Join(tmpDir, "notes.txt")

	_ = os.WriteFile(b1, []byte("old"), 0644)
	time.Sleep(10 * time.Millisecond)
	_ = os.WriteFile(b3, []byte("mid"), 0644)
	time.Sleep(10 * time.Millisecond)
	_ = os.WriteFile(b2, []byte("new"), 0644)
	_ = os.WriteFile(nonBackup, []byte("text"), 0644)

	backups, err := ListBackups(tmpDir)
	if err != nil {
		t.Fatalf("ListBackups failed: %v", err)
	}

	if len(backups) != 3 {
		t.Fatalf("expected 3 backups, got %d", len(backups))
	}

	if backups[0].Name() != "backup_new.tar.gz" {
		t.Errorf("expected newest backup first, got %s", backups[0].Name())
	}
	if backups[2].Name() != "backup_old.tar.gz" {
		t.Errorf("expected oldest backup last, got %s", backups[2].Name())
	}

	_, err = ListBackups("")
	if err == nil {
		t.Errorf("expected error on empty backup dir")
	}

	_, err = ListBackups(filepath.Join(tmpDir, "missing"))
	if err == nil {
		t.Errorf("expected error on nonexistent backup dir")
	}
}
