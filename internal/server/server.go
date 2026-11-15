package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"mcp-keychain/internal/config"
	"mcp-keychain/internal/handlers/requestlog"
)

const readHeaderTimeout = 10 * time.Second

type (
	Server struct {
		logger          *slog.Logger
		http            *http.Server
		shutdownTimeout time.Duration
	}
)

func New(cfg config.Server, handler http.Handler, logger *slog.Logger) *Server {
	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	return &Server{
		logger: logger,
		http: &http.Server{
			Addr:              cfg.Address,
			Handler:           requestlog.New(mux, logger),
			ReadHeaderTimeout: readHeaderTimeout,
		},
		shutdownTimeout: cfg.ShutdownTimeout,
	}
}

func (s *Server) Run(ctx context.Context) error {
	listener, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.http.Addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	s.logger.InfoContext(ctx, "MCP server listening", "address", listener.Addr(), "path", "/mcp")

	return s.Serve(ctx, listener)
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	s.http.BaseContext = func(net.Listener) context.Context {
		return ctx
	}

	finished := make(chan error, 1)

	go func() {
		finished <- s.http.Serve(listener)
	}()

	select {
	case err := <-finished:
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		s.logger.InfoContext(ctx, "HTTP server stopping")

		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdownTimeout)
		defer cancel()

		err := s.http.Shutdown(shutdownCtx)
		if err != nil {
			closeErr := s.http.Close()

			return fmt.Errorf("shutdown HTTP: %w", errors.Join(err, closeErr))
		}

		<-finished
		s.logger.InfoContext(ctx, "HTTP server stopped")

		return nil
	}
}
