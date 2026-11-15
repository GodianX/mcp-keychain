package bash

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"mcp-keychain/internal/services/commands"
)

type (
	Runner struct {
		path string
	}
)

func New() (*Runner, error) {
	path, err := exec.LookPath("bash")
	if err != nil {
		return nil, fmt.Errorf("find bash: %w", err)
	}

	return &Runner{path: path}, nil
}

func (r *Runner) Run(ctx context.Context, dir, command string) (commands.Result, error) {
	cmd := exec.CommandContext(ctx, r.path, "--noprofile", "--norc", "-c", command)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.WaitDelay = time.Second
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}

		return err
	}

	output, err := cmd.CombinedOutput()

	result := commands.Result{Output: string(output), ExitCode: -1}
	if cmd.ProcessState != nil {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}

	if ctx.Err() != nil {
		return result, fmt.Errorf("command interrupted: %w", ctx.Err())
	}

	if err != nil {
		return result, fmt.Errorf("execute command: %w", err)
	}

	return result, nil
}
