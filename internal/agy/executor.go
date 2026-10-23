package agy

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ExecutionRequest struct {
	Profile        string
	Prompt         string
	ConversationID string
	WorkingDir     string
	OnToolEvent    func(tool ToolInvocation)
}

type Executor struct {
	profileManager *ProfileManager
	binPath        string
}

func NewExecutor(pm *ProfileManager, binPath string) *Executor {
	if binPath == "" {
		binPath = "/opt/data/home/.local/bin/agy"
	}
	return &Executor{
		profileManager: pm,
		binPath:        binPath,
	}
}

func (e *Executor) Execute(ctx context.Context, req ExecutionRequest) (*StreamResult, error) {
	if _, err := e.profileManager.EnsureProfile(req.Profile); err != nil {
		return nil, fmt.Errorf("profile verification failed: %w", err)
	}

	args := []string{}
	if req.ConversationID != "" {
		args = append(args, "--conversation", req.ConversationID)
	}

	args = append(args,
		"-p", req.Prompt,
		"--output-format", "stream-json",
		"--dangerously-skip-permissions",
	)

	cmd := exec.CommandContext(ctx, e.binPath, args...)
	cmd.Env = e.profileManager.BuildEnv(req.Profile)
	if req.WorkingDir != "" {
		cmd.Dir = req.WorkingDir
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open stdout pipe: %w", err)
	}

	var stderrBuf strings.Builder
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to spawn agy process: %w", err)
	}

	parser := NewStreamParser()
	scanner := bufio.NewScanner(stdoutPipe)
	buf := make([]byte, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)

	start := time.Now()
	for scanner.Scan() {
		line := scanner.Text()
		parser.FeedLine(line)

		if req.OnToolEvent != nil && len(parser.Result.ToolsInvoked) > 0 {
			lastTool := parser.Result.ToolsInvoked[len(parser.Result.ToolsInvoked)-1]
			req.OnToolEvent(lastTool)
		}
	}

	waitErr := cmd.Wait()
	duration := time.Since(start)

	res := parser.Finalize()
	if res.DurationSeconds == 0 {
		res.DurationSeconds = duration.Seconds()
	}

	if waitErr != nil {
		stderrStr := strings.TrimSpace(stderrBuf.String())
		if res.Response == "" {
			if stderrStr != "" {
				return &res, fmt.Errorf("agy process exited with error: %s (%w)", stderrStr, waitErr)
			}
			return &res, fmt.Errorf("agy execution terminated unexpectedly: %w", waitErr)
		}
	}

	return &res, nil
}
