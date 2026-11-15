package tools_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"mcp-keychain/internal/config"
	"mcp-keychain/internal/handlers/tools"
	"mcp-keychain/internal/services/commands"
	"mcp-keychain/utils/logging"
)

func TestAllowedHosts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		host          string
		allowed       []string
		origin        string
		forwardedHost string
		localIP       string
		wantStatus    int
	}{
		{name: "default localhost", host: "localhost:8222", wantStatus: http.StatusOK},
		{name: "default blocks docker", host: "host.docker.internal:8222", wantStatus: http.StatusForbidden},
		{
			name:       "empty list preserves SDK protection",
			host:       "host.docker.internal:8222",
			allowed:    []string{},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "docker through loopback",
			host:       "host.docker.internal:8222",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusOK,
		},
		{name: "explicit localhost", host: "localhost:8222", allowed: []string{"localhost:8222"}, wantStatus: http.StatusOK},
		{name: "explicit IPv4", host: "127.0.0.1:8222", allowed: []string{"127.0.0.1:8222"}, wantStatus: http.StatusOK},
		{name: "explicit IPv6", host: "[::1]:8222", allowed: []string{"[::1]:8222"}, wantStatus: http.StatusOK},
		{
			name:       "case insensitive request",
			host:       "HOST.DOCKER.INTERNAL:8222",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "case insensitive configuration",
			host:       "host.docker.internal:8222",
			allowed:    []string{"HOST.DOCKER.INTERNAL:8222"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unlisted localhost blocked",
			host:       "localhost:8222",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "wrong port blocked",
			host:       "host.docker.internal:8080",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "hostname suffix blocked",
			host:       "host.docker.internal.evil:8222",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusForbidden,
		},
		{name: "empty host blocked", allowed: []string{"host.docker.internal:8222"}, wantStatus: http.StatusForbidden},
		{
			name:          "forwarded host cannot bypass",
			host:          "evil.example:8222",
			forwardedHost: "host.docker.internal:8222",
			allowed:       []string{"host.docker.internal:8222"},
			wantStatus:    http.StatusForbidden,
		},
		{
			name:       "non-loopback also checked",
			host:       "evil.example:8222",
			localIP:    "192.168.1.10",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cross origin remains blocked",
			host:       "host.docker.internal:8222",
			origin:     "https://evil.example",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "same origin accepted",
			host:       "host.docker.internal:8222",
			origin:     "http://host.docker.internal:8222",
			allowed:    []string{"host.docker.internal:8222"},
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			logger := logging.New(io.Discard)
			service := commands.New([]config.Project{
				{Name: "demo", Tools: []config.Tool{{Name: "test", Desc: "Run tests"}}},
			}, nil, time.Minute, logger)
			handler := tools.New(service, logger, tt.allowed)
			body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost:8222/mcp", strings.NewReader(body))
			request.Host = tt.host
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Accept", "application/json, text/event-stream")
			request.Header.Set("Mcp-Protocol-Version", "2025-06-18")
			request.Header.Set("Origin", tt.origin)
			request.Header.Set("X-Forwarded-Host", tt.forwardedHost)

			localIP := tt.localIP
			if localIP == "" {
				localIP = "127.0.0.1"
			}

			ctx := context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.ParseIP(localIP), Port: 8222})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request.WithContext(ctx))

			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", response.Code, tt.wantStatus, response.Body)
			}

			if tt.wantStatus == http.StatusOK {
				var rpc rpcResponse

				err := json.Unmarshal(response.Body.Bytes(), &rpc)
				if err != nil {
					t.Fatal(err)
				}

				if len(rpc.Error) != 0 || len(rpc.Result.Tools) != 1 || rpc.Result.Tools[0].Name != "demo_test" {
					t.Fatalf("tools/list failed: %s", response.Body)
				}
			}
		})
	}
}
