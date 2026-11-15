# mcp-keychain

This is project is vibecoded.

An MCP server for running predefined Bash commands in project directories. Tools are registered at startup from `projects.yaml` and exposed over Streamable HTTP at `/mcp`.

## Getting started

Requires the Go version specified in `go.mod`, [Task](https://taskfile.dev/), and Bash. Supports macOS and Linux. Lint checks require golangci-lint v2 built with a compatible Go version.

Run all commands from the project root:

```bash
cp config.example.yaml config.yaml
cp projects.example.yaml projects.yaml
```

In `projects.yaml`, replace `/Users/username/Projects/go/project_one` and the other paths with existing absolute paths to your project directories. In `config.yaml`, set the server address, command timeout, and HTTP server shutdown timeout.

```bash
task install
task run
```

To build the binary, run `task build`, then run `./bin/mcp-keychain` from the project root. Configuration files are always loaded from `./config.yaml` and `./projects.yaml`; their paths cannot be changed through command-line arguments or environment variables. Restart the server after changing the configuration.

## Configuration

```yaml
server:
  address: "127.0.0.1:8080"
  command_timeout: "10m"
  shutdown_timeout: "10s"
```

```yaml
projects:
  - name: "project_one"
    dir: "/Users/username/Projects/go/project_one"
    tools:
      - name: "test"
        desc: "Run project_one tests"
        cmd: "task test"
```

All fields shown above are required. Unknown YAML fields, empty lists, nonexistent directories, duplicate project names or directories, and conflicting tool names cause startup to fail. Directory uniqueness is checked by filesystem identity, so paths containing `.`, `..`, trailing slashes, or symlinks cannot register the same directory twice. Names may contain ASCII letters, digits, `_`, and `-`, and must start with a letter or digit. The combined tool name is limited to 128 characters.

### Allowed Host headers and Docker

The optional `server.allowed_hosts` setting accepts a list of exact `host:port` values. Matching is case-insensitive and includes the port; wildcards, URLs, and paths are not accepted. With a nonempty list, every MCP request must match an entry. The list replaces the SDK's localhost Host check, while cross-origin protection remains enabled. Omitting the setting or leaving the list empty preserves the SDK's default behavior.

For a client running inside Docker and an MCP server running on the host at port `8222`, use:

```yaml
server:
  address: "0.0.0.0:8222"
  allowed_hosts:
    - "localhost:8222"
    - "127.0.0.1:8222"
    - "[::1]:8222"
    - "host.docker.internal:8222"
  command_timeout: "10m"
  shutdown_timeout: "10s"
```

Connect the container's MCP client to `http://host.docker.internal:8222/mcp`. On Linux, configure `host.docker.internal:host-gateway` in the container's `extra_hosts` if needed. Restart the server after editing `config.yaml`. A client-side `Host` override is unnecessary.

Docker or OrbStack may forward connections through the host's loopback interface. The SDK then rejects `Host: host.docker.internal:8222` by default even when the server listens on `0.0.0.0`. Adding that exact value to `allowed_hosts` permits the intended request. This setting controls Host validation; it does not provide authentication or restrict which network interfaces the server listens on.

## Calling MCP tools

For model-facing usage instructions, see the [mcp-keychain skill](docs/skills/mcp-keychain/SKILL.md).

Set your MCP client's HTTP URL to `http://127.0.0.1:8080/mcp`. The server supports `initialize`, `tools/list`, and `tools/call`; HTTP sessions are stateless. Tool descriptions use the format `Project <project.name>: <tool.desc>`. With the configuration above, the client will see a `project_one_test` tool with the description `Project project_one: Run project_one tests`.

Example HTTP call after MCP initialization:

```bash
curl http://127.0.0.1:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'MCP-Protocol-Version: 2025-06-18' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"project_one_test","arguments":{}}}'
```

Tool arguments must be an empty object `{}`. The server runs `bash --noprofile --norc -c` with the command from `cmd` and the working directory from `dir`, using the server process's environment. stdout and stderr are combined and returned in full in the first text block and in `structuredContent.output`; the exit code is returned in `structuredContent.exit_code`. On failure, cancellation, or timeout, the result includes `isError: true` and a diagnostic message while preserving any output already captured. Cancellation and timeout terminate the command's process group.

Commands run with the permissions of the user who started the server. The configuration defines which commands are allowed but does not isolate them from the filesystem. The example configuration listens on localhost only; there is no built-in authentication.

## Logging

By default, structured JSON logs from `slog` are written to the console via stderr. To write logs to a file, set the following in `config.yaml`:

```yaml
logging:
  mode: "file"
```

The server creates a `logs/` directory in the project root and appends entries to `logs/mcp-keychain.log`. This path is fixed. To switch back to console logging, use `mode: "console"` or remove the `logging` section. Changes take effect after a restart. If the log file cannot be opened, startup fails.

Each HTTP request receives a `trace_id` that links its request logs to the command's start and completion logs. The server accepts an `X-Trace-ID` of up to 128 characters containing ASCII letters, digits, `.`, `_`, and `-`, starting with a letter or digit. If the header is missing or invalid, a new identifier is generated. The identifier is returned in the response's `X-Trace-ID` header.

Logs include the HTTP method, path, status, tool name, duration, exit code, and errors. Server startup and shutdown events have no `trace_id` because they are not associated with a request.

## Checks

```bash
task lcf
task test
```

`task lc` runs the linter without applying fixes. `task test` enables the race detector and creates `coverage.out`. Docker and a database are not required.
