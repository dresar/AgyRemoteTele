package workspace

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type GitSummary struct {
	Branch        string
	Status        string
	RecentCommits string
	DiffStat      string
	IsRepository  bool
}

func InspectRepository(repoPath string) (*GitSummary, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	branchOut, err := runGit(ctx, repoPath, "branch", "--show-current")
	if err != nil {
		return &GitSummary{IsRepository: false}, err
	}

	branch := strings.TrimSpace(branchOut)
	if branch == "" {
		branch = "HEAD (detached)"
	}

	statusOut, _ := runGit(ctx, repoPath, "status", "--short")
	commitsOut, _ := runGit(ctx, repoPath, "log", "-n", "5", "--oneline")
	diffStatOut, _ := runGit(ctx, repoPath, "diff", "--stat")

	return &GitSummary{
		Branch:        branch,
		Status:        strings.TrimSpace(statusOut),
		RecentCommits: strings.TrimSpace(commitsOut),
		DiffStat:      strings.TrimSpace(diffStatOut),
		IsRepository:  true,
	}, nil
}

func GetDiffSummary(repoPath string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := runGit(ctx, repoPath, "diff", "--stat")
	if err != nil || strings.TrimSpace(out) == "" {
		st, errSt := runGit(ctx, repoPath, "status", "--short")
		if errSt == nil && strings.TrimSpace(st) != "" {
			return strings.TrimSpace(st)
		}
		return "No changes detected"
	}
	return strings.TrimSpace(out)
}

func CommitAndPush(ctx context.Context, repoPath string, message string) (string, error) {
	if strings.TrimSpace(message) == "" {
		message = "chore: automated sync via AgyRemoteTele"
	}

	if _, err := runGit(ctx, repoPath, "add", "."); err != nil {
		return "", fmt.Errorf("git add failed: %w", err)
	}

	commitOut, err := runGit(ctx, repoPath, "commit", "-m", message)
	if err != nil {
		if strings.Contains(commitOut, "nothing to commit") {
			return "Nothing to commit, working tree clean", nil
		}
		return "", fmt.Errorf("git commit failed: %w", err)
	}

	pushOut, err := runGit(ctx, repoPath, "push")
	if err != nil {
		return "", fmt.Errorf("git push failed: %s (%w)", pushOut, err)
	}

	return strings.TrimSpace(commitOut + "\n" + pushOut), nil
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	output := stdout.String()
	if err != nil {
		errStr := strings.TrimSpace(stderr.String())
		if errStr != "" {
			return output + "\n" + errStr, err
		}
		return output, err
	}

	return output, nil
}
