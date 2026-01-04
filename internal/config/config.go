package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name       string       `yaml:"name"`
	ProjectDir string       `yaml:"projectDir"` // (Override only) Defaults to the dir containing the config file. Usually unset.
	WorkDir    string       `yaml:"workdir"`    // defaults to "."
	Image      string       `yaml:"image"`
	Build      *BuildConfig `yaml:"build"`
	Engine     string       `yaml:"engine"` // "podman" or "docker" or empty
	HomeDir    string       `yaml:"home"`
	CacheDir   string       `yaml:"cache"`
	Mounts     []Mount      `yaml:"mounts"`
	Env        EnvVars      `yaml:"env"`
}

type EnvVars map[string]string

func (e *EnvVars) UnmarshalYAML(value *yaml.Node) error {
	if *e == nil {
		*e = make(EnvVars)
	}
	switch value.Kind {
	case yaml.MappingNode:
		var m map[string]string
		if err := value.Decode(&m); err != nil {
			return err
		}
		for k, v := range m {
			(*e)[k] = v
		}
	case yaml.SequenceNode:
		var s []string
		if err := value.Decode(&s); err != nil {
			return err
		}
		for _, item := range s {
			parts := strings.SplitN(item, "=", 2)
			if len(parts) == 2 {
				(*e)[parts[0]] = parts[1]
			} else {
				// Handle KEY only format if needed, for now just skip or set to empty
				(*e)[parts[0]] = ""
			}
		}
	default:
		return fmt.Errorf("env must be a map or a list of strings")
	}
	return nil
}

type BuildConfig struct {
	Context       string `yaml:"context"`
	Containerfile string `yaml:"containerfile"`
	Tag           string `yaml:"tag"`
}

type Mount struct {
	Source string `yaml:"source"`
	Target string `yaml:"target"`
	Mode   string `yaml:"mode"` // "rw" or "ro"
}

func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}

	// Try to load .airlock/airlock.local.yaml relative to the config file or project root
	localPath := filepath.Join(filepath.Dir(path), ".airlock", "airlock.local.yaml")
	if _, err := os.Stat(localPath); err == nil {
		lb, err := os.ReadFile(localPath)
		if err == nil {
			// Unmarshal into the same struct to "merge" basic fields.
			// Note: yaml.Unmarshal into an existing struct merges maps and replaces scalars.
			if err := yaml.Unmarshal(lb, &c); err != nil {
				return nil, fmt.Errorf("failed to parse local config: %w", err)
			}
			// Re-read both to handle fieldMentioned/fieldWasExplicitlyFalse if we want to be very precise,
			// but for now let's focus on the primary merge.
			// We combine the bytes for the fieldMentioned checks later.
			b = append(b, []byte("\n")...)
			b = append(b, lb...)
		}
	}

	// defaults
	dir := filepath.Dir(path)
	if c.Name == "" {
		c.Name = filepath.Base(dir)
	}
	if c.ProjectDir == "" {
		c.ProjectDir = dir
	}
	if c.WorkDir == "" {
		c.WorkDir = "."
	}

	if c.Image != "" && c.Build != nil {
		return nil, errors.New("Only one of either Image or Build can be configured")
	}

	// If neither image nor build is set, try to default to build if Containerfile exists
	if c.Image == "" && c.Build == nil {
		if _, err := os.Stat("Containerfile"); err == nil {
			c.Build = &BuildConfig{
				Context:       ".",
				Containerfile: "Containerfile",
			}
		} else if _, err := os.Stat("env/Containerfile"); err == nil {
			c.Build = &BuildConfig{
				Context:       "./env",
				Containerfile: "./env/Containerfile",
			}
		}
	}

	if c.Build != nil {
		if c.Build.Context == "" {
			c.Build.Context = "."
		}
		if c.Build.Containerfile == "" {
			c.Build.Containerfile = "Containerfile"
		}
		if c.Build.Tag == "" {
			c.Build.Tag = "airlock:" + sanitizeTag(c.Name)
		}
	}

	if c.HomeDir == "" {
		c.HomeDir = "./.airlock/home"
	}
	if c.CacheDir == "" {
		c.CacheDir = "./.airlock/cache"
	}

	if c.Env == nil {
		c.Env = EnvVars{}
	}

	if c.Name == "" {
		return nil, errors.New("name is required")
	}
	return &c, nil
}

func InitFiles(dir string, name string) error {
	cfgPath := filepath.Join(dir, "airlock.yaml")
	localCfgPath := filepath.Join(dir, ".airlock", "airlock.local.yaml")
	gitignorePath := filepath.Join(dir, ".gitignore")
	containerfilePath := filepath.Join(dir, "Containerfile")

	if name == "" {
		name = "my-project"
	}

	// config only if missing
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(cfgPath, []byte(defaultYAML(name)), 0644); err != nil {
			return err
		}
	}

	// Containerfile only if missing
	if _, err := os.Stat(containerfilePath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(containerfilePath, []byte(defaultContainerfile()), 0644); err != nil {
			return err
		}
	}

	// ensure default .airlock dirs exist (safe defaults)
	if err := os.MkdirAll(filepath.Join(dir, ".airlock", "home"), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, ".airlock", "cache"), 0700); err != nil {
		return err
	}

	// local config only if missing
	if _, err := os.Stat(localCfgPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(localCfgPath, []byte(defaultLocalYAML()), 0644); err != nil {
			return err
		}
	}

	// ensure .gitignore ignores .airlock/
	ensureLineInFile(gitignorePath, ".airlock/")

	return nil
}

func defaultLocalYAML() string {
	return `# This file is for local-only configuration that should not be checked into version control.
# Properties here will merge with and override airlock.yaml.
# This is a good place for personal API tokens or local environment overrides.

env:
  vars:
    # GITHUB_TOKEN: "your-token-here"
    # AWS_PROFILE: "local-dev"
`
}

func defaultYAML(name string) string {
	return fmt.Sprintf(`name: %s

engine: podman # or docker, or omit

# The sandbox container image to run.
# You can either point at a prebuilt image OR provide a build section in place of image.
# image: ghcr.io/your-org/airlock-dev:latest

# If build is set, Airlock will build and tag an image for this project.
build:
  context: .
  containerfile: ./Containerfile
  tag: airlock:%s

# Host directories that back the sandbox HOME and cache.
# Defaults are inside the repo for simplicity.
home: ./.airlock/home
cache: ./.airlock/cache

# To reuse across projects, point these at shared host paths, e.g.:
# home: ~/.local/share/airlock/home
# cache: ~/.local/share/airlock/cache

workdir: .

mounts:
  - source: .
    target: /workspace
    mode: rw

env:
  - EXAMPLE_VAR: "hello"
`, name, name)
}

func defaultContainerfile() string {
	return `FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# Install a bunch of useful fullstack dev deps
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    git \
    gnupg \
    jq \
    less \
    openssh-client \
    ripgrep \
    build-essential \
    python3 \
    python3-pip \
    nodejs \
    npm \
    bash \
    tzdata \
    golang-go \
  && rm -rf /var/lib/apt/lists/*

# Base image uses ubuntu user
ARG USERNAME=ubuntu

USER root
RUN mkdir -p /workspace && chown $USERNAME:$USERNAME /workspace

# Install claude code as root
RUN npm install -g @anthropic-ai/claude-code || echo "WARNING: Failed to install @anthropic-ai/claude-code via npm."

# Switch back to ubuntu and set HOME and GOCACHE
USER $USERNAME
ENV HOME=/home/$USERNAME
ENV GOCACHE=$HOME/.cache/go-build
WORKDIR /workspace

# Keep the container running so you can 'exec' into it
CMD ["sleep", "infinity"]
`
}

func ensureLineInFile(path string, line string) {
	// Best-effort helper: create file if missing; append line if not present.
	b, err := os.ReadFile(path)
	if err != nil {
		_ = os.WriteFile(path, []byte(line+"\n"), 0644)
		return
	}
	txt := string(b)
	if indexOf(txt, line) >= 0 {
		return
	}
	if len(txt) > 0 && txt[len(txt)-1] != '\n' {
		txt += "\n"
	}
	txt += line + "\n"
	_ = os.WriteFile(path, []byte(txt), 0644)
}

func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}

func sanitizeTag(s string) string { return sanitizeName(s) }

func fieldWasExplicitlyFalse(yamlBytes []byte, field string) bool {
	text := string(yamlBytes)
	needle := field + ":"
	i := indexOf(text, needle)
	if i < 0 {
		return false
	}
	rest := text[i+len(needle):]
	line := rest
	if j := indexOf(rest, "\n"); j >= 0 {
		line = rest[:j]
	}
	return indexOf(line, "false") >= 0
}

func fieldMentioned(yamlBytes []byte, field string) bool {
	return indexOf(string(yamlBytes), field+":") >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
