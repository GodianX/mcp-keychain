package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"mcp-keychain/internal/services/commands"
	"mcp-keychain/utils/logging"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type (
	Commands interface {
		Tools() []commands.Tool
		Execute(ctx context.Context, name string) (commands.Result, error)
	}
)

func New(service Commands, logger *slog.Logger, allowedHosts []string) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{Name: "mcp-keychain", Version: "1.0.0"}, nil)

	for _, tool := range service.Tools() {
		server.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		}, newHandler(service, tool.Name, logger))
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                    true,
		JSONResponse:                 true,
		PropagateRequestCancellation: true,
		DisableLocalhostProtection:   len(allowedHosts) != 0,
	})

	return http.NewCrossOriginProtection().Handler(withAllowedHosts(handler, logger, allowedHosts))
}

func newHandler(service Commands, name string, logger *slog.Logger) mcp.ToolHandler {
	return func(ctx context.Context, request *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.Params.Arguments
		if len(args) != 0 {
			var object map[string]json.RawMessage

			err := json.Unmarshal(args, &object)
			if err != nil || object == nil || len(object) != 0 {
				logging.FromContext(ctx, logger).WarnContext(ctx, "tool arguments rejected", "tool", name)

				return nil, errors.New("tool arguments must be an empty object")
			}
		}

		result, err := service.Execute(ctx, name)

		return formatResult(result, err), nil
	}
}

func formatResult(result commands.Result, err error) *mcp.CallToolResult {
	content := []mcp.Content{&mcp.TextContent{Text: result.Output}}
	if err != nil {
		content = append(content, &mcp.TextContent{Text: err.Error()})
	}

	content = append(content, &mcp.TextContent{Text: fmt.Sprintf("exit_code: %d", result.ExitCode)})

	return &mcp.CallToolResult{
		Content:           content,
		StructuredContent: result,
		IsError:           err != nil || result.ExitCode != 0,
	}
}
