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
  VAR1: "value1"
  VAR2: "value2"
`
	err = os.WriteFile(cfgPath, []byte(mainYAML), 0644)
	if err != nil {
		t.Fatal(err)
	}

	localYAML := `env:
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

	if cfg.Env["VAR1"] != "value1" {
		t.Errorf("expected VAR1=value1, got %s", cfg.Env["VAR1"])
	}

	if cfg.Env["VAR2"] != "overridden" {
		t.Errorf("expected VAR2=overridden, got %s", cfg.Env["VAR2"])
	}

	if cfg.Env["VAR3"] != "local-only" {
		t.Errorf("expected VAR3=local-only, got %s", cfg.Env["VAR3"])
	}
}

func TestLoadWithBuild(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-build-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "airlock.yaml")
	yaml := `name: build-project
build:
  context: ./src
  containerfile: ./src/Containerfile
  tag: my-tag:latest
`
	err = os.WriteFile(cfgPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Build == nil {
		t.Fatal("expected build section to be populated")
	}
	if cfg.Build.Context != "./src" {
		t.Errorf("expected context ./src, got %s", cfg.Build.Context)
	}
	if cfg.Build.Containerfile != "./src/Containerfile" {
		t.Errorf("expected containerfile ./src/Containerfile, got %s", cfg.Build.Containerfile)
	}
	if cfg.Build.Tag != "my-tag:latest" {
		t.Errorf("expected tag my-tag:latest, got %s", cfg.Build.Tag)
	}
}

func TestLoadWithImage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-image-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "airlock.yaml")
	yaml := `name: image-project
image: my-image:latest
`
	err = os.WriteFile(cfgPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Image != "my-image:latest" {
		t.Errorf("expected image my-image:latest, got %s", cfg.Image)
	}
	if cfg.Build != nil {
		t.Error("expected build section to be nil")
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

func TestLoadWithMounts(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-mounts-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "airlock.yaml")
	yaml := `name: mounts-project
home: ./.airlock/myhome
cache: ./.airlock/mycache
workdir: /myworkspace
mounts:
  - source: ./data
    target: /mnt/data
    mode: ro
`
	err = os.WriteFile(cfgPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.HomeDir != "./.airlock/myhome" {
		t.Errorf("expected home ./.airlock/myhome, got %s", cfg.HomeDir)
	}
	if cfg.CacheDir != "./.airlock/mycache" {
		t.Errorf("expected cache ./.airlock/mycache, got %s", cfg.CacheDir)
	}
	if cfg.WorkDir != "/myworkspace" {
		t.Errorf("expected workdir /myworkspace, got %s", cfg.WorkDir)
	}
	if len(cfg.Mounts) != 1 {
		t.Fatalf("expected 1 mount, got %d", len(cfg.Mounts))
	}
	if cfg.Mounts[0].Source != "./data" {
		t.Errorf("expected mount source ./data, got %s", cfg.Mounts[0].Source)
	}
	if cfg.Mounts[0].Target != "/mnt/data" {
		t.Errorf("expected mount target /mnt/data, got %s", cfg.Mounts[0].Target)
	}
	if cfg.Mounts[0].Mode != "ro" {
		t.Errorf("expected mount mode ro, got %s", cfg.Mounts[0].Mode)
	}
}
