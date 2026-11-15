package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"mcp-keychain/internal/root"
	"mcp-keychain/utils/logging"
)

func main() {
	slog.SetDefault(logging.New(os.Stderr))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx)

	stop()

	if err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	app, err := root.New(os.DirFS("."))
	if err != nil {
		return err
	}

	return app.Run(ctx)
}
