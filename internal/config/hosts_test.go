package config_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"testing/fstest"

	"mcp-keychain/internal/config"
)

func TestAllowedHosts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tests := []struct {
		name      string
		hosts     []string
		wantError bool
	}{
		{name: "null list"},
		{name: "empty list", hosts: []string{}},
		{name: "docker", hosts: []string{"host.docker.internal:8222"}},
		{name: "IPv6", hosts: []string{"[::1]:8222"}},
		{name: "multiple hosts", hosts: []string{"localhost:8222", "127.0.0.1:8222", "[::1]:8222", "host.docker.internal:8222"}},
		{name: "empty entry", hosts: []string{""}, wantError: true},
		{name: "missing port", hosts: []string{"host.docker.internal"}, wantError: true},
		{name: "invalid port", hosts: []string{"localhost:invalid"}, wantError: true},
		{name: "zero port", hosts: []string{"localhost:0"}, wantError: true},
		{name: "empty hostname", hosts: []string{":8222"}, wantError: true},
		{name: "unbracketed IPv6", hosts: []string{"::1:8222"}, wantError: true},
		{name: "port out of range", hosts: []string{"localhost:65536"}, wantError: true},
		{name: "URL rejected", hosts: []string{"http://host.docker.internal:8222"}, wantError: true},
		{name: "wildcard rejected", hosts: []string{"*:8222"}, wantError: true},
		{name: "path rejected", hosts: []string{"host.docker.internal:8222/mcp"}, wantError: true},
		{name: "spaces rejected", hosts: []string{" host.docker.internal:8222"}, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hosts, err := json.Marshal(tt.hosts)
			if err != nil {
				t.Fatal(err)
			}

			files := fstest.MapFS{
				"config.yaml": {Data: []byte(fmt.Sprintf(
					"server:\n  address: '127.0.0.1:8222'\n  command_timeout: 1m\n  shutdown_timeout: 1s\n  allowed_hosts: %s\n", hosts))},
				"projects.yaml": {Data: []byte(fmt.Sprintf(
					"projects: [{name: demo, dir: %q, tools: [{name: test, desc: Run tests, cmd: task test}]}]\n", dir))},
			}

			cfg, err := config.Load(files)
			if (err != nil) != tt.wantError {
				t.Fatalf("Load() error = %v, want error %v", err, tt.wantError)
			}

			if !tt.wantError && !slices.Equal(cfg.Server.AllowedHosts, tt.hosts) {
				t.Fatalf("allowed_hosts = %v, want %v", cfg.Server.AllowedHosts, tt.hosts)
			}
		})
	}
}
