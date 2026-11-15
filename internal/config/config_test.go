package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"mcp-keychain/internal/config"
)

func TestLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	otherDir := t.TempDir()
	server := "server:\n  address: '127.0.0.1:8080'\n  command_timeout: 1m\n  shutdown_timeout: 1s\n"
	projects := fmt.Sprintf(
		"projects:\n  - name: demo\n    dir: %q\n    tools:\n      - name: test\n        desc: Run tests\n        cmd: task test\n",
		dir,
	)
	filePath := filepath.Join(dir, "regular-file")

	err := os.WriteFile(filePath, nil, 0o600)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		server    string
		projects  string
		wantError string
	}{
		{name: "valid", server: server, projects: projects},
		{name: "console logging", server: server + "logging:\n  mode: console\n", projects: projects},
		{name: "file logging", server: server + "logging:\n  mode: file\n", projects: projects},
		{name: "invalid logging mode", server: server + "logging:\n  mode: other\n", projects: projects, wantError: "oneof"},
		{name: "custom log path rejected", server: server + "logging:\n  file: other.log\n", projects: projects, wantError: "file"},
		{name: "missing server file", projects: projects, wantError: "open config.yaml"},
		{name: "missing projects file", server: server, wantError: "open projects.yaml"},
		{name: "empty server", server: "{}", projects: projects, wantError: "decode config.yaml"},
		{name: "null config", server: "null", projects: projects, wantError: "decode config.yaml"},
		{name: "null server", server: "server: null", projects: projects, wantError: "decode config.yaml"},
		{name: "null projects document", server: server, projects: "null", wantError: "decode projects.yaml"},
		{name: "invalid yaml", server: "server: [", projects: projects, wantError: "decode config.yaml"},
		{name: "unknown field", server: server + "unexpected: true\n", projects: projects, wantError: "unexpected"},
		{name: "duplicate field", server: server + server, projects: projects, wantError: "decode config.yaml"},
		{name: "second document", server: server + "---\n{}\n", projects: projects, wantError: "exactly one"},
		{
			name:      "required address",
			server:    strings.ReplaceAll(server, "  address: '127.0.0.1:8080'\n", ""),
			projects:  projects,
			wantError: "required",
		},
		{
			name:      "invalid address",
			server:    strings.ReplaceAll(server, "127.0.0.1:8080", "wrong"),
			projects:  projects,
			wantError: "decode config.yaml",
		},
		{name: "zero port", server: strings.ReplaceAll(server, ":8080", ":0"), projects: projects, wantError: "decode config.yaml"},
		{name: "invalid duration", server: strings.ReplaceAll(server, "1m", "bad"), projects: projects, wantError: "decode config.yaml"},
		{name: "zero duration", server: strings.ReplaceAll(server, "1m", "0s"), projects: projects, wantError: "decode config.yaml"},
		{name: "negative duration", server: strings.ReplaceAll(server, "1m", "-1s"), projects: projects, wantError: "decode config.yaml"},
		{name: "empty projects", server: server, projects: "projects: []", wantError: "decode projects.yaml"},
		{
			name:      "empty tools",
			server:    server,
			projects:  fmt.Sprintf("projects: [{name: demo, dir: %q, tools: []}]", dir),
			wantError: "decode projects.yaml",
		},
		{
			name:      "required command",
			server:    server,
			projects:  strings.ReplaceAll(projects, "        cmd: task test\n", ""),
			wantError: "required",
		},
		{
			name:      "blank command",
			server:    server,
			projects:  strings.ReplaceAll(projects, "cmd: task test", "cmd: '  '"),
			wantError: "nonblank",
		},
		{
			name:      "blank description",
			server:    server,
			projects:  strings.ReplaceAll(projects, "desc: Run tests", "desc: '  '"),
			wantError: "nonblank",
		},
		{
			name:      "invalid project",
			server:    server,
			projects:  strings.ReplaceAll(projects, "name: demo", "name: 'bad name'"),
			wantError: "project name",
		},
		{
			name:      "invalid tool",
			server:    server,
			projects:  strings.ReplaceAll(projects, "name: test", "name: 'bad.tool'"),
			wantError: "tool name",
		},
		{
			name:      "long tool",
			server:    server,
			projects:  strings.ReplaceAll(projects, "name: test", "name: "+strings.Repeat("a", 128)),
			wantError: "tool name",
		},
		{
			name:      "relative directory",
			server:    server,
			projects:  strings.ReplaceAll(projects, fmt.Sprintf("%q", dir), "relative"),
			wantError: "absolute path",
		},
		{
			name:      "missing directory",
			server:    server,
			projects:  strings.ReplaceAll(projects, fmt.Sprintf("%q", dir), fmt.Sprintf("%q", filepath.Join(dir, "missing"))),
			wantError: "access directory",
		},
		{
			name:      "file as directory",
			server:    server,
			projects:  strings.ReplaceAll(projects, fmt.Sprintf("%q", dir), fmt.Sprintf("%q", filePath)),
			wantError: "refer to a directory",
		},
		{
			name:      "duplicate project",
			server:    server,
			projects:  projects + strings.TrimPrefix(projects, "projects:\n"),
			wantError: "project name",
		},
		{
			name:      "duplicate tool",
			server:    server,
			projects:  projects + "      - name: test\n        desc: Duplicate\n        cmd: task test\n",
			wantError: "tool name",
		},
		{
			name:   "composed name collision",
			server: server,
			projects: strings.ReplaceAll(
				projects,
				"name: test",
				"name: part_test",
			) + strings.ReplaceAll(
				strings.ReplaceAll(strings.TrimPrefix(projects, "projects:\n"), "name: demo", "name: demo_part"),
				fmt.Sprintf("%q", dir), fmt.Sprintf("%q", otherDir)),
			wantError: "tool name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := fstest.MapFS{}
			if tt.server != "" {
				files["config.yaml"] = &fstest.MapFile{Data: []byte(tt.server)}
			}

			if tt.projects != "" {
				files["projects.yaml"] = &fstest.MapFile{Data: []byte(tt.projects)}
			}

			cfg, loadErr := config.Load(files)
			if tt.wantError != "" {
				if loadErr == nil || !strings.Contains(loadErr.Error(), tt.wantError) {
					t.Fatalf("Load() error = %v, want containing %q", loadErr, tt.wantError)
				}

				return
			}

			if loadErr != nil {
				t.Fatal(loadErr)
			}

			if cfg.Server.CommandTimeout != time.Minute || cfg.Server.ShutdownTimeout != time.Second || cfg.Projects[0].Dir != dir {
				t.Fatalf("unexpected config: %+v", cfg)
			}
		})
	}
}

func TestProjectUniqueness(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	dir := filepath.Join(root, "project")
	nested := filepath.Join(dir, "nested")
	otherDir := filepath.Join(root, "other")

	err := os.MkdirAll(nested, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	err = os.Mkdir(otherDir, 0o700)
	if err != nil {
		t.Fatal(err)
	}

	alias := filepath.Join(root, "alias")

	err = os.Symlink(dir, alias)
	if err != nil {
		t.Fatal(err)
	}

	nestedAlias := filepath.Join(root, "nested-alias")

	err = os.Symlink(nested, nestedAlias)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		secondName string
		secondDir  string
		wantError  string
	}{
		{name: "distinct projects", secondName: "two", secondDir: otherDir},
		{name: "nested directory allowed", secondName: "two", secondDir: nested},
		{name: "duplicate name", secondName: "one", secondDir: otherDir, wantError: "duplicate project name"},
		{name: "duplicate directory", secondName: "two", secondDir: dir, wantError: "duplicate directory"},
		{name: "trailing slash", secondName: "two", secondDir: dir + "/", wantError: "duplicate directory"},
		{name: "dot path", secondName: "two", secondDir: dir + "/.", wantError: "duplicate directory"},
		{name: "parent path", secondName: "two", secondDir: nested + "/..", wantError: "duplicate directory"},
		{name: "symlink", secondName: "two", secondDir: alias, wantError: "duplicate directory"},
		{name: "symlink parent", secondName: "two", secondDir: nestedAlias + "/..", wantError: "duplicate directory"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			files := fstest.MapFS{
				"config.yaml": {Data: []byte("server:\n  address: '127.0.0.1:8080'\n  command_timeout: 1m\n  shutdown_timeout: 1s\n")},
				"projects.yaml": {Data: []byte(fmt.Sprintf(
					"projects:\n"+
						"  - name: one\n    dir: %q\n    tools: [{name: test, desc: Run tests, cmd: task test}]\n"+
						"  - name: %s\n    dir: %q\n    tools: [{name: test, desc: Run tests, cmd: task test}]\n",
					dir, tt.secondName, tt.secondDir))},
			}

			_, loadErr := config.Load(files)
			if tt.wantError == "" {
				if loadErr != nil {
					t.Fatal(loadErr)
				}

				return
			}

			if loadErr == nil || !strings.Contains(loadErr.Error(), tt.wantError) {
				t.Fatalf("Load() error = %v, want %q", loadErr, tt.wantError)
			}

			if tt.wantError == "duplicate directory" &&
				(!strings.Contains(loadErr.Error(), `project "one"`) || !strings.Contains(loadErr.Error(), `project "two"`)) {
				t.Fatalf("duplicate directory error must identify both projects: %v", loadErr)
			}
		})
	}
}
