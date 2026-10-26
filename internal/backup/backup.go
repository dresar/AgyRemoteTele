package backup

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type BackupResult struct {
	ArchivePath    string        `json:"archive_path"`
	FileCount      int           `json:"file_count"`
	TotalSizeBytes int64         `json:"total_size_bytes"`
	Duration       time.Duration `json:"duration"`
}

func CreateTarGz(sourceDirs []string, destFile string, excludePatterns []string) (*BackupResult, error) {
	if len(sourceDirs) == 0 {
		return nil, fmt.Errorf("no source directories provided")
	}
	if destFile == "" {
		return nil, fmt.Errorf("destFile cannot be empty")
	}

	start := time.Now()

	destDir := filepath.Dir(destFile)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return nil, err
	}

	absDest, _ := filepath.Abs(destFile)

	outFile, err := os.Create(destFile)
	if err != nil {
		return nil, err
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	fileCount := 0

	for _, src := range sourceDirs {
		cleanSrc := filepath.Clean(src)
		info, err := os.Lstat(cleanSrc)
		if err != nil {
			return nil, err
		}

		baseDir := filepath.Dir(cleanSrc)

		if info.IsDir() {
			err = filepath.Walk(cleanSrc, func(curPath string, curInfo os.FileInfo, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}

				absCur, _ := filepath.Abs(curPath)
				if absCur == absDest {
					return nil
				}

				if shouldExclude(curPath, curInfo.Name(), excludePatterns) {
					if curInfo.IsDir() {
						return filepath.SkipDir
					}
					return nil
				}

				rel, err := filepath.Rel(baseDir, curPath)
				if err != nil {
					rel = curPath
				}

				var linkTarget string
				if curInfo.Mode()&os.ModeSymlink != 0 {
					linkTarget, _ = os.Readlink(curPath)
				}

				hdr, err := tar.FileInfoHeader(curInfo, linkTarget)
				if err != nil {
					return err
				}
				hdr.Name = filepath.ToSlash(rel)
				if curInfo.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
					hdr.Name += "/"
				}

				if err := tw.WriteHeader(hdr); err != nil {
					return err
				}

				if curInfo.Mode().IsRegular() {
					f, err := os.Open(curPath)
					if err != nil {
						return err
					}
					_, err = io.Copy(tw, f)
					f.Close()
					if err != nil {
						return err
					}
					fileCount++
				} else if !curInfo.IsDir() {
					fileCount++
				}

				return nil
			})
			if err != nil {
				return nil, err
			}
		} else {
			if shouldExclude(cleanSrc, info.Name(), excludePatterns) {
				continue
			}

			hdr, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return nil, err
			}
			hdr.Name = filepath.Base(cleanSrc)

			if err := tw.WriteHeader(hdr); err != nil {
				return nil, err
			}

			if info.Mode().IsRegular() {
				f, err := os.Open(cleanSrc)
				if err != nil {
					return nil, err
				}
				_, err = io.Copy(tw, f)
				f.Close()
				if err != nil {
					return nil, err
				}
				fileCount++
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	if err := outFile.Close(); err != nil {
		return nil, err
	}

	fi, err := os.Stat(destFile)
	if err != nil {
		return nil, err
	}

	return &BackupResult{
		ArchivePath:    destFile,
		FileCount:      fileCount,
		TotalSizeBytes: fi.Size(),
		Duration:       time.Since(start),
	}, nil
}

func ListBackups(backupDir string) ([]os.FileInfo, error) {
	if backupDir == "" {
		return nil, fmt.Errorf("backupDir cannot be empty")
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}

	var results []os.FileInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tgz") {
			info, err := entry.Info()
			if err == nil {
				results = append(results, info)
			}
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].ModTime().After(results[j].ModTime())
	})

	return results, nil
}

func shouldExclude(path, name string, patterns []string) bool {
	for _, pat := range patterns {
		pat = strings.TrimSpace(pat)
		if pat == "" {
			continue
		}
		if match, _ := filepath.Match(pat, name); match {
			return true
		}
		if match, _ := filepath.Match(pat, path); match {
			return true
		}
		cleanPat := strings.Trim(pat, "*")
		if cleanPat != "" && strings.Contains(path, cleanPat) {
			return true
		}
		if strings.HasPrefix(pat, "*") && strings.HasSuffix(name, strings.TrimPrefix(pat, "*")) {
			return true
		}
	}
	return false
}
