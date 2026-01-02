package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Name       string `yaml:"name"`
	ProjectDir string `yaml:"projectDir"` // defaults to "."
	Image      Image  `yaml:"image"`
	Engine     Engine `yaml:"engine"`
	Mounts     Mounts `yaml:"mounts"`
	Env        Env    `yaml:"env"`
	Agent      Agent  `yaml:"agent"`
}

type Image struct {
	BaseImage     string `yaml:"baseImage"`     // optional if building locally from Containerfile
	Containerfile string `yaml:"containerfile"` // default: "./Containerfile"
	Build         bool   `yaml:"build"`         // default true if Containerfile exists (unless explicitly set)
	Tag           string `yaml:"tag"`           // default: airlock:<name>
}

type Engine struct {
	Preferred string `yaml:"preferred"` // "podman" or "docker" or empty
}

type Mounts struct {
	Workdir  string `yaml:"workdir"`  // default: /workspace
	HomeDir  string `yaml:"homeDir"`  // default: ./.airlock/home (host path)
	CacheDir string `yaml:"cacheDir"` // default: ./.airlock/cache (host path)
}

type Env struct {
	Vars map[string]string `yaml:"vars"`
}

type Agent struct {
	InstallClaudeCode bool `yaml:"installClaudeCode"` // default true
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
	// The README says ./.airlock/airlock.local.yaml
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
	if c.Name == "" {
		dir := filepath.Dir(path)
		c.Name = filepath.Base(dir)
	}
	if c.ProjectDir == "" {
		c.ProjectDir = "."
	}
	if c.Image.Containerfile == "" {
		c.Image.Containerfile = "Containerfile"
	}
	if c.Image.Tag == "" {
		c.Image.Tag = "airlock:" + sanitizeTag(c.Name)
	}
	// If user didn't specify build and Containerfile exists, assume build=true.
	if !fieldMentioned(b, "build") {
		if _, statErr := os.Stat(c.Image.Containerfile); statErr == nil {
			c.Image.Build = true
		}
	}

	if c.Mounts.Workdir == "" {
		c.Mounts.Workdir = "/workspace"
	}
	if c.Mounts.HomeDir == "" {
		c.Mounts.HomeDir = "./.airlock/home"
	}
	if c.Mounts.CacheDir == "" {
		c.Mounts.CacheDir = "./.airlock/cache"
	}

	if c.Env.Vars == nil {
		c.Env.Vars = map[string]string{}
	}
	if !fieldWasExplicitlyFalse(b, "installClaudeCode") {
		c.Agent.InstallClaudeCode = true
	}
	if c.Name == "" {
		return nil, errors.New("name is required (or inferable)")
	}
	return &c, nil
}

func InitFiles(dir string) error {
	cfgPath := filepath.Join(dir, "airlock.yaml")
	localCfgPath := filepath.Join(dir, ".airlock", "airlock.local.yaml")
	gitignorePath := filepath.Join(dir, ".gitignore")

	// config only if missing
	if _, err := os.Stat(cfgPath); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(cfgPath, []byte(defaultYAML()), 0644); err != nil {
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

func defaultYAML() string {
	return `name: my-project
projectDir: .

engine:
  preferred: podman # or docker, or omit

image:
  containerfile: Containerfile
  build: true
  tag: airlock:my-project

mounts:
  workdir: /workspace

  # Host directories that back the sandbox HOME and cache.
  # Defaults are inside the repo for simplicity.
  homeDir: ./.airlock/home
  cacheDir: ./.airlock/cache

  # To reuse across projects, point these at shared host paths, e.g.:
  # homeDir: ~/.local/share/airlock/home
  # cacheDir: ~/.local/share/airlock/cache

env:
  vars:
    EXAMPLE_VAR: "hello"

agent:
  installClaudeCode: true
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
