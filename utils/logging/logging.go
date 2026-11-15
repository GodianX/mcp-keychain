package logging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
)

const (
	logDirectoryPermissions = 0o700
	logFilePermissions      = 0o600
)

type (
	Output struct {
		Logger *slog.Logger
		file   *os.File
	}

	traceKey struct{}
)

func New(writer io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(writer, nil))
}

func Open(rootDir, mode string) (*Output, error) {
	if mode == "" || mode == "console" {
		return &Output{Logger: New(os.Stderr)}, nil
	}

	if mode != "file" {
		return nil, fmt.Errorf("invalid logging mode %q", mode)
	}

	root, err := os.OpenRoot(rootDir)
	if err != nil {
		return nil, fmt.Errorf("open project root for logging: %w", err)
	}

	file, err := openLogFile(root)

	closeErr := root.Close()
	if err != nil {
		return nil, errors.Join(err, closeErr)
	}

	if closeErr != nil {
		return nil, errors.Join(closeErr, file.Close())
	}

	return &Output{Logger: New(file), file: file}, nil
}

func (o *Output) Close() error {
	if o.file == nil {
		return nil
	}

	return o.file.Close()
}

func WithTrace(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceKey{}, id)
}

func TraceID(ctx context.Context) string {
	id, ok := ctx.Value(traceKey{}).(string)
	if !ok {
		return ""
	}

	return id
}

func FromContext(ctx context.Context, logger *slog.Logger) *slog.Logger {
	id := TraceID(ctx)
	if id == "" {
		return logger
	}

	return logger.With("trace_id", id)
}

func openLogFile(root *os.Root) (*os.File, error) {
	err := root.MkdirAll("logs", logDirectoryPermissions)
	if err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}

	file, err := root.OpenFile("logs/mcp-keychain.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, logFilePermissions)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	return file, nil
}
