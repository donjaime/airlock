package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadWithLocal(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "airlock.yaml")
	localDir := filepath.Join(tmpDir, ".airlock")
	localPath := filepath.Join(localDir, "airlock.local.yaml")

	err = os.MkdirAll(localDir, 0755)
	if err != nil {
		t.Fatal(err)
	}

	mainYAML := `name: test-project
env:
  vars:
    VAR1: "value1"
    VAR2: "value2"
`
	err = os.WriteFile(cfgPath, []byte(mainYAML), 0644)
	if err != nil {
		t.Fatal(err)
	}

	localYAML := `env:
  vars:
    VAR2: "overridden"
    VAR3: "local-only"
`
	err = os.WriteFile(localPath, []byte(localYAML), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Name != "test-project" {
		t.Errorf("expected name test-project, got %s", cfg.Name)
	}

	if cfg.Env.Vars["VAR1"] != "value1" {
		t.Errorf("expected VAR1=value1, got %s", cfg.Env.Vars["VAR1"])
	}

	if cfg.Env.Vars["VAR2"] != "overridden" {
		t.Errorf("expected VAR2=overridden, got %s", cfg.Env.Vars["VAR2"])
	}

	if cfg.Env.Vars["VAR3"] != "local-only" {
		t.Errorf("expected VAR3=local-only, got %s", cfg.Env.Vars["VAR3"])
	}
}

func TestInitFiles(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-init-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	err = InitFiles(tmpDir)
	if err != nil {
		t.Fatalf("InitFiles failed: %v", err)
	}

	// Check airlock.yaml
	if _, err := os.Stat(filepath.Join(tmpDir, "airlock.yaml")); err != nil {
		t.Errorf("airlock.yaml not created")
	}

	// Check .airlock/airlock.local.yaml
	if _, err := os.Stat(filepath.Join(tmpDir, ".airlock", "airlock.local.yaml")); err != nil {
		t.Errorf(".airlock/airlock.local.yaml not created")
	}

	// Check .gitignore
	if _, err := os.Stat(filepath.Join(tmpDir, ".gitignore")); err != nil {
		t.Errorf(".gitignore not created")
	}
}
