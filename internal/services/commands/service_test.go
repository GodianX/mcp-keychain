package commands_test

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"mcp-keychain/utils/logging"

	"mcp-keychain/internal/adapters/bash"
	"mcp-keychain/internal/config"
	"mcp-keychain/internal/services/commands"
)

func TestExecute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		tool       string
		cmd        string
		wantOutput string
		wantCode   int
		wantError  string
		timeout    time.Duration
	}{
		{name: "allowlisted command", tool: "one_test", cmd: "printf one", wantOutput: "one"},
		{name: "other project", tool: "two_test", cmd: "printf one", wantOutput: "two"},
		{name: "unknown tool", tool: "one_other", cmd: "touch unexpected", wantCode: -1, wantError: "unknown tool"},
		{
			name:       "failed command",
			tool:       "one_test",
			cmd:        "printf error; exit 3",
			wantOutput: "error",
			wantCode:   3,
			wantError:  "exit status 3",
		},
		{
			name:       "configured timeout",
			tool:       "one_test",
			cmd:        "printf started; sleep 30",
			wantOutput: "started",
			wantCode:   -1,
			wantError:  "deadline exceeded",
			timeout:    100 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := bash.New()
			if err != nil {
				t.Fatal(err)
			}

			timeout := tt.timeout
			if timeout == 0 {
				timeout = time.Minute
			}

			projects := []config.Project{
				{Name: "one", Dir: t.TempDir(), Tools: []config.Tool{{Name: "test", Desc: "first", Cmd: tt.cmd}}},
				{Name: "two", Dir: t.TempDir(), Tools: []config.Tool{{Name: "test", Desc: "second", Cmd: "printf two"}}},
			}
			service := commands.New(projects, runner, timeout, logging.New(io.Discard))
			projects[0].Tools[0].Cmd = "printf modified"

			result, err := service.Execute(t.Context(), tt.tool)
			if result.Output != tt.wantOutput || result.ExitCode != tt.wantCode {
				t.Fatalf("result = %+v", result)
			}

			if tt.wantError == "" && err != nil {
				t.Fatal(err)
			}

			if tt.wantError != "" && (err == nil || !strings.Contains(err.Error(), tt.wantError)) {
				t.Fatalf("error = %v, want %q", err, tt.wantError)
			}
		})
	}
}

func TestTools(t *testing.T) {
	t.Parallel()

	service := commands.New(
		[]config.Project{{Name: "one", Tools: []config.Tool{{Name: "test", Desc: "Run tests"}}}},
		nil,
		time.Minute,
		logging.New(io.Discard),
	)

	tools := service.Tools()
	if len(tools) != 1 || tools[0].Name != "one_test" || tools[0].Description != "Project one: Run tests" {
		t.Fatalf("Tools() = %+v", tools)
	}

	tools[0].Name = "changed"
	if service.Tools()[0].Name != "one_test" {
		t.Fatal("Tools() exposes the internal registry")
	}
}

func TestExecuteCanceled(t *testing.T) {
	t.Parallel()

	runner, err := bash.New()
	if err != nil {
		t.Fatal(err)
	}

	service := commands.New(
		[]config.Project{{Name: "one", Dir: t.TempDir(), Tools: []config.Tool{{Name: "test", Cmd: "sleep 30"}}}},
		runner,
		time.Minute,
		logging.New(io.Discard))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err = service.Execute(ctx, "one_test")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v", err)
	}
}
