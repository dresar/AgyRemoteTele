package workspace

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type FileItem struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	Mode    string
	ModTime time.Time
}

func ListDirectory(dirPath string) ([]FileItem, error) {
	cleanPath := filepath.Clean(dirPath)
	entries, err := os.ReadDir(cleanPath)
	if err != nil {
		return nil, err
	}

	var items []FileItem
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		itemPath := filepath.Join(cleanPath, entry.Name())
		items = append(items, FileItem{
			Name:    entry.Name(),
			Path:    itemPath,
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			Mode:    info.Mode().String(),
			ModTime: info.ModTime(),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDir != items[j].IsDir {
			return items[i].IsDir
		}
		return items[i].Name < items[j].Name
	})

	return items, nil
}

func ReadFile(filePath string, maxBytes int64) (string, bool, error) {
	cleanPath := filepath.Clean(filePath)
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", false, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", false, err
	}

	if info.IsDir() {
		return "", false, fmt.Errorf("path is a directory: %s", cleanPath)
	}

	if maxBytes <= 0 {
		maxBytes = 32768
	}

	buf := make([]byte, maxBytes+1)
	n, err := io.ReadFull(file, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", false, err
	}

	truncated := false
	if int64(n) > maxBytes {
		truncated = true
		n = int(maxBytes)
	}

	return string(buf[:n]), truncated, nil
}

func ValidateRegularFile(filePath string) (string, error) {
	cleanPath := filepath.Clean(filePath)
	info, err := os.Stat(cleanPath)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("specified target is a directory, not a file: %s", cleanPath)
	}
	return cleanPath, nil
}

func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
