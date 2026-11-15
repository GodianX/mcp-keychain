package tools_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mcp-keychain/utils/logging"

	"mcp-keychain/internal/adapters/bash"
	"mcp-keychain/internal/config"
	"mcp-keychain/internal/handlers/requestlog"
	"mcp-keychain/internal/handlers/tools"
	"mcp-keychain/internal/services/commands"
)

type (
	httpCase struct {
		name          string
		method        string
		body          string
		origin        string
		wantStatus    int
		wantRPCError  bool
		wantToolError bool
		wantOutput    string
		wantCode      int
	}
	rpcResponse struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			IsError           bool            `json:"isError"`
			StructuredContent commands.Result `json:"structuredContent"`
			Content           []struct {
				Text string `json:"text"`
			} `json:"content"`
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema struct {
					AdditionalProperties bool `json:"additionalProperties"`
				} `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
)

func TestHTTP(t *testing.T) {
	t.Parallel()

	tests := []httpCase{
		{
			name: "initialize",
			body: `{"jsonrpc":"2.0","id":1,"method":"initialize","params":` +
				`{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		},
		{name: "list tools", body: `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`},
		{
			name:       "call success",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"one_test","arguments":{}}}`,
			wantOutput: "outerr",
		},
		{
			name:       "call without arguments",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"one_test"}}`,
			wantOutput: "outerr",
		},
		{
			name:          "call failure",
			body:          `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"two_test","arguments":{}}}`,
			wantToolError: true,
			wantOutput:    "failed",
			wantCode:      7,
		},
		{
			name:         "unknown tool",
			body:         `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"unknown","arguments":{}}}`,
			wantRPCError: true,
		},
		{
			name: "arbitrary command",
			body: `{"jsonrpc":"2.0","id":1,"method":"tools/call",` +
				`"params":{"name":"one_test","arguments":{"cmd":"touch injected"}}}`,
			wantRPCError: true,
		},
		{
			name:         "directory override",
			body:         `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"one_test","arguments":{"dir":"/tmp"}}}`,
			wantRPCError: true,
		},
		{
			name:         "null arguments",
			body:         `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"one_test","arguments":null}}`,
			wantRPCError: true,
		},
		{
			name:         "array arguments",
			body:         `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"one_test","arguments":[]}}`,
			wantRPCError: true,
		},
		{
			name:       "cross origin",
			body:       `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
			origin:     "https://untrusted.example",
			wantStatus: http.StatusForbidden,
		},
		{name: "GET is stateless", method: http.MethodGet, wantStatus: http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runner, err := bash.New()
			if err != nil {
				t.Fatal(err)
			}

			dir := t.TempDir()
			service := commands.New([]config.Project{
				{
					Name:  "one",
					Dir:   dir,
					Tools: []config.Tool{{Name: "test", Desc: "Run one", Cmd: "touch executed; printf out; printf err >&2"}},
				},
				{Name: "two", Dir: dir, Tools: []config.Tool{{Name: "test", Desc: "Run two", Cmd: "printf failed; exit 7"}}},
			}, runner, time.Minute, logging.New(io.Discard))
			handler := tools.New(service, logging.New(io.Discard), nil)

			method := tt.method
			if method == "" {
				method = http.MethodPost
			}

			request := httptest.NewRequestWithContext(t.Context(), method, "http://localhost/mcp", strings.NewReader(tt.body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("Mcp-Protocol-Version", "2025-06-18")

			if tt.origin != "" {
				request.Header.Set("Origin", tt.origin)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			status := tt.wantStatus
			if status == 0 {
				status = http.StatusOK
			}

			if response.Code != status {
				t.Fatalf("HTTP status = %d, want %d: %s", response.Code, status, response.Body)
			}

			if status != http.StatusOK {
				return
			}

			checkResponse(t, response, dir, tt)
		})
	}
}

func TestTraceLogging(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		command string
		code    int
		level   string
	}{
		{name: "success", command: "printf done", level: "INFO"},
		{name: "command error", command: "printf failed; exit 7", code: 7, level: "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buffer bytes.Buffer

			logger := logging.New(&buffer)

			runner, err := bash.New()
			if err != nil {
				t.Fatal(err)
			}

			service := commands.New([]config.Project{
				{Name: "demo", Dir: t.TempDir(), Tools: []config.Tool{{Name: "test", Cmd: tt.command}}},
			}, runner, time.Minute, logger)
			handler := requestlog.New(tools.New(service, logger, nil), logger)
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"demo_test","arguments":{}}}`
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp", strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("Mcp-Protocol-Version", "2025-06-18")
			request.Header.Set("X-Trace-Id", "integration-trace")

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			if response.Code != http.StatusOK || response.Header().Get("X-Trace-Id") != "integration-trace" {
				t.Fatalf("unexpected HTTP response: %+v", response)
			}

			messages := readTraceLogs(t, &buffer)

			if len(messages) != 4 || messages["HTTP request started"] == nil || messages["HTTP request completed"] == nil ||
				messages["command started"] == nil {
				t.Fatalf("missing correlated logs: %v", messages)
			}

			completed := messages["command completed"]
			if completed["exit_code"] != float64(tt.code) || completed["level"] != tt.level || completed["tool"] != "demo_test" ||
				completed["duration_ms"] == nil {
				t.Fatalf("invalid command log: %v", completed)
			}

			if tt.code != 0 && completed["error"] == nil {
				t.Fatal("command failure was not logged")
			}
		})
	}
}

func checkResponse(t *testing.T, response *httptest.ResponseRecorder, dir string, tt httpCase) {
	t.Helper()

	var rpc rpcResponse

	err := json.Unmarshal(response.Body.Bytes(), &rpc)
	if err != nil {
		t.Fatal(err)
	}

	if (len(rpc.Error) != 0) != tt.wantRPCError {
		t.Fatalf("unexpected RPC response: %s", response.Body)
	}

	if tt.wantRPCError {
		_, err = os.Stat(filepath.Join(dir, "executed"))
		if !os.IsNotExist(err) {
			t.Fatal("invalid request executed a command")
		}

		return
	}

	if tt.name == "list tools" {
		checkToolList(t, rpc, response)
	}

	if tt.wantOutput != "" {
		if rpc.Result.IsError != tt.wantToolError || rpc.Result.StructuredContent.Output != tt.wantOutput ||
			rpc.Result.StructuredContent.ExitCode != tt.wantCode ||
			len(rpc.Result.Content) == 0 ||
			rpc.Result.Content[0].Text != tt.wantOutput {
			t.Fatalf("unexpected tools/call: %s", response.Body)
		}
	}
}

func checkToolList(t *testing.T, rpc rpcResponse, response *httptest.ResponseRecorder) {
	t.Helper()

	if len(rpc.Result.Tools) != 2 || rpc.Result.Tools[0].Name != "one_test" || rpc.Result.Tools[1].Name != "two_test" ||
		rpc.Result.Tools[0].Description != "Project one: Run one" ||
		rpc.Result.Tools[1].Description != "Project two: Run two" ||
		rpc.Result.Tools[0].InputSchema.AdditionalProperties {
		t.Fatalf("unexpected tools/list: %s", response.Body)
	}
}

func readTraceLogs(t *testing.T, buffer *bytes.Buffer) map[string]map[string]any {
	t.Helper()

	decoder := json.NewDecoder(buffer)
	messages := make(map[string]map[string]any)

	for decoder.More() {
		var entry map[string]any

		err := decoder.Decode(&entry)
		if err != nil {
			t.Fatal(err)
		}

		if entry["trace_id"] != "integration-trace" {
			t.Fatalf("lost trace_id: %v", entry)
		}

		message, ok := entry["msg"].(string)
		if !ok {
			t.Fatalf("missing log message: %v", entry)
		}

		messages[message] = entry
	}

	return messages
}
