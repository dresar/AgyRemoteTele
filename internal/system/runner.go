package system

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type ExecutionOutput struct {
	Stdout   string
	Stderr   string
	Combined string
	ExitCode int
	Duration time.Duration
}

func RunCommand(ctx context.Context, cmdStr string, workingDir string) (*ExecutionOutput, error) {
	start := time.Now()

	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", cmdStr)
	if workingDir != "" {
		cmd.Dir = workingDir
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	duration := time.Since(start)

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()

	combined := stdoutStr
	if stderrStr != "" {
		if combined != "" && !strings.HasSuffix(combined, "\n") {
			combined += "\n"
		}
		combined += stderrStr
	}

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}

	const maxOutputLength = 65536
	if len(combined) > maxOutputLength {
		combined = combined[:maxOutputLength] + "\n... [output truncated]"
	}

	res := &ExecutionOutput{
		Stdout:   stdoutStr,
		Stderr:   stderrStr,
		Combined: combined,
		ExitCode: exitCode,
		Duration: duration,
	}

	if err != nil && exitCode != 0 {
		return res, fmt.Errorf("command failed with exit code %d: %s", exitCode, strings.TrimSpace(stderrStr))
	}

	return res, err
}
