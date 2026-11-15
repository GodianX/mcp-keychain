package requestlog_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mcp-keychain/internal/handlers/requestlog"
	"mcp-keychain/utils/logging"
)

func TestMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		headers   []string
		wantSame  bool
		status    int
		wantLevel string
		flush     bool
	}{
		{name: "generated", wantLevel: "INFO"},
		{name: "accepted", headers: []string{"trace-123_ab.C"}, wantSame: true, wantLevel: "INFO"},
		{name: "maximum length", headers: []string{strings.Repeat("a", 128)}, wantSame: true, wantLevel: "INFO"},
		{name: "empty", headers: []string{""}, wantLevel: "INFO"},
		{name: "too long", headers: []string{strings.Repeat("a", 129)}, wantLevel: "INFO"},
		{name: "invalid characters", headers: []string{"bad value"}, wantLevel: "INFO"},
		{name: "multiple headers", headers: []string{"first", "second"}, wantLevel: "INFO"},
		{name: "no content", status: http.StatusNoContent, wantLevel: "INFO"},
		{name: "client error", status: http.StatusBadRequest, wantLevel: "WARN"},
		{name: "server error", status: http.StatusInternalServerError, wantLevel: "ERROR"},
		{name: "stream flush", flush: true, wantLevel: "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var (
				buffer       bytes.Buffer
				contextTrace string
			)

			handler := requestlog.New(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextTrace = logging.TraceID(r.Context())

				if tt.flush {
					err := http.NewResponseController(w).Flush()
					if err != nil {
						t.Error(err)
					}
				}

				if tt.status != 0 {
					w.WriteHeader(tt.status)
					w.WriteHeader(http.StatusTeapot)
				} else {
					_, err := fmt.Fprint(w, "ok")
					if err != nil {
						t.Error(err)
					}
				}
			}), logging.New(&buffer))

			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost/mcp", http.NoBody)
			for _, value := range tt.headers {
				request.Header.Add("X-Trace-Id", value)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)

			id := response.Header().Get("X-Trace-Id")
			if id == "" || contextTrace != id {
				t.Fatalf("response trace %q, context trace %q", id, contextTrace)
			}

			if tt.wantSame && id != tt.headers[0] {
				t.Fatalf("incoming trace not reused: %q", id)
			}

			if !tt.wantSame && len(tt.headers) != 0 && id == tt.headers[0] {
				t.Fatal("invalid trace reused")
			}

			checkRequestLogs(t, &buffer, id, tt.wantLevel, response.Code)
		})
	}
}

func checkRequestLogs(t *testing.T, buffer *bytes.Buffer, id, wantLevel string, status int) {
	t.Helper()

	var started, completed map[string]any

	decoder := json.NewDecoder(buffer)

	err := decoder.Decode(&started)
	if err != nil {
		t.Fatal(err)
	}

	err = decoder.Decode(&completed)
	if err != nil {
		t.Fatal(err)
	}

	if started["trace_id"] != id || completed["trace_id"] != id || completed["level"] != wantLevel {
		t.Fatalf("unexpected request logs: %v, %v", started, completed)
	}

	if completed["status"] != float64(status) || completed["method"] != "POST" || completed["path"] != "/mcp" ||
		completed["duration_ms"] == nil {
		t.Fatalf("missing request fields: %v", completed)
	}
}
