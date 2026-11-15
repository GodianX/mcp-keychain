package bash_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcp-keychain/internal/adapters/bash"
)

func TestRun(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		command    string
		wantOutput string
		wantCode   int
		wantError  bool
		missingDir bool
	}{
		{name: "combined output", command: "printf 'out'; printf 'err' >&2", wantOutput: "outerr"},
		{name: "exit status", command: "printf 'failure'; exit 7", wantOutput: "failure", wantCode: 7, wantError: true},
		{name: "working directory", command: "cat fixture.txt", wantOutput: "project file"},
		{name: "empty output", command: "true"},
		{name: "large output", command: "head -c 131072 /dev/zero | tr '\\0' x", wantOutput: strings.Repeat("x", 131072)},
		{name: "missing directory", command: "true", missingDir: true, wantCode: -1, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := bash.New()
			if err != nil {
				t.Fatal(err)
			}

			dir := t.TempDir()

			err = os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte("project file"), 0o600)
			if err != nil {
				t.Fatal(err)
			}

			if tt.missingDir {
				dir = filepath.Join(dir, "missing")
			}

			result, err := runner.Run(t.Context(), dir, tt.command)
			if (err != nil) != tt.wantError || result.Output != tt.wantOutput || result.ExitCode != tt.wantCode {
				t.Fatalf(
					"Run() = %+v, %v; want output length %d, code %d, error %v",
					result,
					err,
					len(tt.wantOutput),
					tt.wantCode,
					tt.wantError,
				)
			}
		})
	}
}

func TestCancellation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		canceled  bool
		wantError error
	}{
		{name: "canceled before start", canceled: true, wantError: context.Canceled},
		{name: "timeout kills child processes", wantError: context.DeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := bash.New()
			if err != nil {
				t.Fatal(err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()

			if tt.canceled {
				cancel()
			}

			dir := t.TempDir()
			started := time.Now()

			result, err := runner.Run(ctx, dir, "printf started; (sleep 0.3; printf leaked > leaked.txt) & wait")
			if !errors.Is(err, tt.wantError) || result.ExitCode != -1 {
				t.Fatalf("Run() = %+v, %v; want %v", result, err, tt.wantError)
			}

			if time.Since(started) > 2*time.Second {
				t.Fatal("cancellation did not promptly return")
			}

			if tt.canceled {
				return
			}

			if result.Output != "started" {
				t.Fatalf("partial output = %q", result.Output)
			}

			time.Sleep(400 * time.Millisecond)

			_, err = os.Stat(filepath.Join(dir, "leaked.txt"))
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("child process survived timeout: %v", err)
			}
		})
	}
}
