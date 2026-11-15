package root_test

import (
	"context"
	"fmt"
	"testing"
	"testing/fstest"
	"time"

	"mcp-keychain/internal/root"
	"mcp-keychain/testhelpers"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		files fstest.MapFS
	}{
		{name: "missing config", files: fstest.MapFS{}},
		{name: "invalid config", files: fstest.MapFS{"config.yaml": {Data: []byte("unexpected: true")}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := root.New(tt.files)
			if err == nil {
				t.Fatal("New() accepted invalid configuration")
			}
		})
	}
}

func TestMCPClient(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := fstest.MapFS{
		"config.yaml": {Data: []byte("server:\n  address: '127.0.0.1:8080'\n  command_timeout: 1m\n  shutdown_timeout: 1s\n" +
			"  allowed_hosts: ['host.docker.internal:8222']\n")},
		"projects.yaml": {
			Data: []byte(
				fmt.Sprintf(
					"projects:\n  - name: demo\n    dir: %q\n    tools:\n"+
						"      - name: test\n        desc: Read project file\n        cmd: \"printf success; printf stderr >&2\"\n",
					dir,
				),
			),
		},
	}

	app, err := root.New(files)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	listener, httpClient := testhelpers.NewHTTPListener(t)

	finished := make(chan error, 1)
	go func() { finished <- app.Serve(ctx, listener) }()

	t.Cleanup(func() {
		cancel()

		select {
		case serveErr := <-finished:
			if serveErr != nil {
				t.Error(serveErr)
			}
		case <-time.After(3 * time.Second):
			t.Error("server did not stop")
		}
	})

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)

	clientCtx, clientCancel := context.WithTimeout(ctx, 5*time.Second)
	defer clientCancel()

	session, err := client.Connect(clientCtx, &mcp.StreamableClientTransport{
		Endpoint: "http://host.docker.internal:8222/mcp", HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	list, err := session.ListTools(clientCtx, nil)
	if err != nil {
		t.Fatal(err)
	}

	if len(list.Tools) != 1 || list.Tools[0].Name != "demo_test" || list.Tools[0].Description != "Project demo: Read project file" {
		t.Fatalf("ListTools() = %+v", list)
	}

	result, err := session.CallTool(clientCtx, &mcp.CallToolParams{Name: "demo_test", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}

	if result.IsError || len(result.Content) == 0 {
		t.Fatalf("CallTool() = %+v", result)
	}

	content, ok := result.Content[0].(*mcp.TextContent)
	if !ok || content.Text != "successstderr" {
		t.Fatalf("unexpected content: %+v", result.Content)
	}
}
