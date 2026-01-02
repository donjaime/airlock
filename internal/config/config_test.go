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
	if cfg.Workdir != "/myworkspace" {
		t.Errorf("expected workdir /myworkspace, got %s", cfg.Workdir)
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

func TestLoadWithUser(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-user-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "airlock.yaml")
	yaml := `name: user-project
user:
  name: testuser
  uid: 2000
  gid: 2000
  home: /home/testuser
`
	err = os.WriteFile(cfgPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.User.Name != "testuser" {
		t.Errorf("expected user testuser, got %s", cfg.User.Name)
	}
	if cfg.User.UID != 2000 {
		t.Errorf("expected uid 2000, got %d", cfg.User.UID)
	}
	if cfg.User.GID != 2000 {
		t.Errorf("expected gid 2000, got %d", cfg.User.GID)
	}
	if cfg.User.Home != "/home/testuser" {
		t.Errorf("expected home /home/testuser, got %s", cfg.User.Home)
	}
}

func TestLoadWithUserDefaults(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "airlock-user-defaults-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cfgPath := filepath.Join(tmpDir, "airlock.yaml")
	yaml := `name: user-defaults-project`
	err = os.WriteFile(cfgPath, []byte(yaml), 0644)
	if err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.User.Name != "agent" {
		t.Errorf("expected default user agent, got %s", cfg.User.Name)
	}
	if cfg.User.UID != 1000 {
		t.Errorf("expected default uid 1000, got %d", cfg.User.UID)
	}
	if cfg.User.GID != 1000 {
		t.Errorf("expected default gid 1000, got %d", cfg.User.GID)
	}
	if cfg.User.Home != "/home/agent" {
		t.Errorf("expected default home /home/agent, got %s", cfg.User.Home)
	}
}
