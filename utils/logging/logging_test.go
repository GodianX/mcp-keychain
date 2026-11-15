package logging_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mcp-keychain/utils/logging"
)

func TestContextLogger(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		trace string
	}{
		{name: "no trace"},
		{name: "traced", trace: "test-trace-123"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer

			ctx := t.Context()
			if tt.trace != "" {
				ctx = logging.WithTrace(ctx, tt.trace)
			}

			child, cancel := context.WithCancel(ctx)
			defer cancel()

			logger := logging.FromContext(child, logging.New(&buffer))
			logger.InfoContext(child, "test message", "tool", "demo_test")

			var entry map[string]any

			err := json.Unmarshal(buffer.Bytes(), &entry)
			if err != nil {
				t.Fatal(err)
			}

			if entry["msg"] != "test message" || entry["level"] != "INFO" || entry["tool"] != "demo_test" {
				t.Fatalf("unexpected log: %s", &buffer)
			}

			if logging.TraceID(child) != tt.trace {
				t.Fatal("trace not propagated to child context")
			}

			if tt.trace == "" {
				if _, exists := entry["trace_id"]; exists {
					t.Fatal("trace_id present outside a request")
				}
			} else if entry["trace_id"] != tt.trace {
				t.Fatalf("trace_id = %v", entry["trace_id"])
			}

			if logging.TraceID(t.Context()) != "" {
				t.Fatal("trace leaked into parent context")
			}
		})
	}
}

func TestOpen(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		mode        string
		blockedDir  bool
		blockedFile bool
		wantError   bool
	}{
		{name: "console default"},
		{name: "console explicit", mode: "console"},
		{name: "file append", mode: "file"},
		{name: "invalid mode", mode: "unknown", wantError: true},
		{name: "directory failure", mode: "file", blockedDir: true, wantError: true},
		{name: "file failure", mode: "file", blockedFile: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()

			path := filepath.Join(dir, "logs", "mcp-keychain.log")
			if tt.blockedDir {
				err := os.WriteFile(filepath.Join(dir, "logs"), nil, 0o600)
				if err != nil {
					t.Fatal(err)
				}
			}

			if tt.blockedFile {
				err := os.MkdirAll(path, 0o700)
				if err != nil {
					t.Fatal(err)
				}
			}

			output, err := logging.Open(dir, tt.mode)
			if (err != nil) != tt.wantError {
				t.Fatalf("Open() error = %v", err)
			}

			if tt.wantError {
				return
			}

			if tt.mode != "file" {
				err = output.Close()
				if err != nil {
					t.Fatal(err)
				}

				_, err = os.Stat(filepath.Join(dir, "logs"))
				if !os.IsNotExist(err) {
					t.Fatal("console mode created a log directory")
				}

				return
			}

			output.Logger.Info("first message")

			err = output.Close()
			if err != nil {
				t.Fatal(err)
			}

			output, err = logging.Open(dir, tt.mode)
			if err != nil {
				t.Fatal(err)
			}

			output.Logger.Info("second message")

			err = output.Close()
			if err != nil {
				t.Fatal(err)
			}

			checkAppendedLogs(t, path)
		})
	}
}

func checkAppendedLogs(t *testing.T, path string) {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(data, []byte("first message")) || !bytes.Contains(data, []byte("second message")) ||
		strings.Count(string(data), "\n") != 2 {
		t.Fatalf("file not appended: %s", data)
	}
}
