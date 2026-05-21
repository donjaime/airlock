package global

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// overrideDir is set in tests to avoid touching ~/.airlock.
var overrideDir string

// Dir returns the root airlock directory (~/.airlock or the test override).
func Dir() string {
	if overrideDir != "" {
		return overrideDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".airlock")
}

type GlobalConfig struct {
	Version         int    `yaml:"version"`
	Engine          string `yaml:"engine"`
	DefaultIdentity string `yaml:"defaultIdentity"`
}

func configPath() string {
	return filepath.Join(Dir(), "config.yaml")
}

func LoadConfig() (*GlobalConfig, error) {
	b, err := os.ReadFile(configPath())
	if os.IsNotExist(err) {
		return &GlobalConfig{Version: 1}, nil
	}
	if err != nil {
		return nil, err
	}
	var c GlobalConfig
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return &c, nil
}

func SaveConfig(c *GlobalConfig) error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(), b, 0600)
}
