package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/goccy/go-yaml"
)

var namePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

const maxToolNameLength = 128

type (
	Config struct {
		Server   Server    `yaml:"server"  validate:"required"`
		Logging  Logging   `yaml:"logging"`
		Projects []Project `yaml:"-"`
	}

	Logging struct {
		Mode string `yaml:"mode" validate:"omitempty,oneof=console file"`
	}

	Server struct {
		Address         string        `yaml:"address"          validate:"required,hostname_port"`
		AllowedHosts    []string      `yaml:"allowed_hosts"    validate:"omitempty,dive,required"`
		CommandTimeout  time.Duration `yaml:"command_timeout"  validate:"required,gt=0"`
		ShutdownTimeout time.Duration `yaml:"shutdown_timeout" validate:"required,gt=0"`
	}

	Project struct {
		Name  string `yaml:"name"  validate:"required"`
		Dir   string `yaml:"dir"   validate:"required"`
		Tools []Tool `yaml:"tools" validate:"required,min=1,dive"`
	}

	Tool struct {
		Name string `yaml:"name" validate:"required"`
		Desc string `yaml:"desc" validate:"required"`
		Cmd  string `yaml:"cmd"  validate:"required"`
	}

	projectsFile struct {
		Projects []Project `yaml:"projects" validate:"required,min=1,dive"`
	}
)

func Load(files fs.FS) (*Config, error) {
	cfg := new(Config)

	err := decode(files, "config.yaml", cfg)
	if err != nil {
		return nil, err
	}

	projects := new(projectsFile)

	err = decode(files, "projects.yaml", projects)
	if err != nil {
		return nil, err
	}

	cfg.Projects = projects.Projects

	err = cfg.validate()
	if err != nil {
		return nil, err
	}

	return cfg, nil
}

func (cfg *Config) validate() error {
	err := validateAllowedHosts(cfg.Server.AllowedHosts)
	if err != nil {
		return err
	}

	projects := make(map[string]bool)
	directories := make(map[string]os.FileInfo)
	tools := make(map[string]bool)

	for _, project := range cfg.Projects {
		if !namePattern.MatchString(project.Name) || projects[project.Name] {
			return fmt.Errorf("invalid or duplicate project name %q", project.Name)
		}

		projects[project.Name] = true

		directory, err := validateDirectory(project.Dir)
		if err != nil {
			return fmt.Errorf("project %q: %w", project.Name, err)
		}

		for name, existing := range directories {
			if os.SameFile(directory, existing) {
				return fmt.Errorf("project %q: duplicate directory %q already used by project %q", project.Name, project.Dir, name)
			}
		}

		directories[project.Name] = directory

		err = validateTools(project, tools)
		if err != nil {
			return err
		}
	}

	return nil
}

func validateAllowedHosts(hosts []string) error {
	validate := validator.New()

	for _, value := range hosts {
		host, port, err := net.SplitHostPort(value)
		if err != nil || host == "" {
			return fmt.Errorf("server.allowed_hosts: invalid host:port %q", value)
		}

		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return fmt.Errorf("server.allowed_hosts: invalid port in %q", value)
		}

		if net.ParseIP(host) == nil {
			err = validate.Var(host, "hostname_rfc1123")
			if err != nil {
				return fmt.Errorf("server.allowed_hosts: invalid hostname in %q", value)
			}
		}
	}

	return nil
}

func decode(files fs.FS, name string, target any) error {
	data, err := fs.ReadFile(files, name)
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data), yaml.Strict(), yaml.Validator(validator.New(validator.WithRequiredStructEnabled())))

	err = decoder.Decode(target)
	if err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}

	var extra any

	err = decoder.Decode(&extra)
	if !errors.Is(err, io.EOF) {
		return fmt.Errorf("%s must contain exactly one YAML document", name)
	}

	return nil
}

func validateDirectory(dir string) (os.FileInfo, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("dir must be an absolute path")
	}

	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("access directory: %w", err)
	}

	if !info.IsDir() {
		return nil, errors.New("dir must refer to a directory")
	}

	return info, nil
}

func validateTools(project Project, names map[string]bool) error {
	for _, tool := range project.Tools {
		name := project.Name + "_" + tool.Name
		if !namePattern.MatchString(tool.Name) || len(name) > maxToolNameLength || names[name] {
			return fmt.Errorf("invalid or duplicate tool name %q", name)
		}

		if strings.TrimSpace(tool.Desc) == "" || strings.TrimSpace(tool.Cmd) == "" {
			return fmt.Errorf("tool %q requires nonblank desc and cmd", name)
		}

		names[name] = true
	}

	return nil
}
