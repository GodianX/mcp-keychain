package commands

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"mcp-keychain/internal/config"
	"mcp-keychain/utils/logging"
)

type (
	Service struct {
		logger   *slog.Logger
		runner   Runner
		timeout  time.Duration
		tools    []Tool
		commands map[string]command
	}

	Runner interface {
		Run(ctx context.Context, dir, command string) (Result, error)
	}

	Result struct {
		Output   string `json:"output"`
		ExitCode int    `json:"exit_code"`
	}

	Tool struct {
		Name        string
		Description string
	}

	command struct {
		dir string
		cmd string
	}
)

func New(projects []config.Project, runner Runner, timeout time.Duration, logger *slog.Logger) *Service {
	service := &Service{
		logger:   logger,
		runner:   runner,
		timeout:  timeout,
		commands: make(map[string]command),
	}

	for _, project := range projects {
		for _, tool := range project.Tools {
			name := project.Name + "_" + tool.Name
			service.tools = append(service.tools, Tool{
				Name:        name,
				Description: "Project " + project.Name + ": " + tool.Desc,
			})
			service.commands[name] = command{dir: project.Dir, cmd: tool.Cmd}
		}
	}

	return service
}

func (s *Service) Tools() []Tool {
	return append([]Tool(nil), s.tools...)
}

func (s *Service) Execute(ctx context.Context, name string) (Result, error) {
	logger := logging.FromContext(ctx, s.logger).With("tool", name)

	cmd, exists := s.commands[name]
	if !exists {
		logger.WarnContext(ctx, "unknown tool rejected")

		return Result{ExitCode: -1}, fmt.Errorf("unknown tool %q", name)
	}

	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	started := time.Now()

	logger.InfoContext(ctx, "command started", "dir", cmd.dir)
	result, err := s.runner.Run(ctx, cmd.dir, cmd.cmd)

	attributes := []any{"exit_code", result.ExitCode, "duration_ms", time.Since(started).Milliseconds()}
	if err != nil {
		attributes = append(attributes, "error", err)
	}

	level := slog.LevelInfo
	if err != nil || result.ExitCode != 0 {
		level = slog.LevelError
	}

	logger.Log(ctx, level, "command completed", attributes...)

	return result, err
}
