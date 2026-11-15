package root

import (
	"context"
	"errors"
	"io/fs"
	"net"

	"mcp-keychain/internal/adapters/bash"
	"mcp-keychain/internal/config"
	"mcp-keychain/internal/handlers/tools"
	"mcp-keychain/internal/server"
	"mcp-keychain/internal/services/commands"
	"mcp-keychain/utils/logging"
)

type (
	App struct {
		server *server.Server
		output *logging.Output
	}
)

func New(files fs.FS) (*App, error) {
	cfg, err := config.Load(files)
	if err != nil {
		return nil, err
	}

	runner, err := bash.New()
	if err != nil {
		return nil, err
	}

	output, err := logging.Open(".", cfg.Logging.Mode)
	if err != nil {
		return nil, err
	}

	service := commands.New(cfg.Projects, runner, cfg.Server.CommandTimeout, output.Logger)
	handler := tools.New(service, output.Logger, cfg.Server.AllowedHosts)

	return &App{server: server.New(cfg.Server, handler, output.Logger), output: output}, nil
}

func (a *App) Run(ctx context.Context) error {
	err := a.server.Run(ctx)

	return a.finish(ctx, err)
}

func (a *App) Serve(ctx context.Context, listener net.Listener) error {
	err := a.server.Serve(ctx, listener)

	return a.finish(ctx, err)
}

func (a *App) finish(ctx context.Context, err error) error {
	if err != nil {
		a.output.Logger.ErrorContext(ctx, "server failed", "error", err)
	}

	return errors.Join(err, a.output.Close())
}
