package global

import (
	"fmt"
	"os"
	"path/filepath"
)

type Identity struct {
	Name     string
	HomeDir  string
	CacheDir string
}

// Dir returns the identity's root directory (parent of home/ and cache/).
func (id *Identity) Dir() string {
	return filepath.Dir(id.HomeDir)
}

func identitiesDir() string {
	return filepath.Join(Dir(), "identities")
}

func identityDir(name string) string {
	return filepath.Join(identitiesDir(), name)
}

// CreateIdentity creates home/, cache/, and template hook scripts for a named identity.
func CreateIdentity(name string) (*Identity, error) {
	home := filepath.Join(identityDir(name), "home")
	cache := filepath.Join(identityDir(name), "cache")
	if err := os.MkdirAll(home, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	id := &Identity{Name: name, HomeDir: home, CacheDir: cache}
	if err := writeTemplateScripts(id); err != nil {
		return nil, err
	}
	return id, nil
}

// GetIdentity returns the Identity for name, or an error if it doesn't exist.
func GetIdentity(name string) (*Identity, error) {
	dir := identityDir(name)
	if _, err := os.Stat(dir); err != nil {
		return nil, fmt.Errorf("identity %q not found in %s", name, identitiesDir())
	}
	return &Identity{
		Name:     name,
		HomeDir:  filepath.Join(dir, "home"),
		CacheDir: filepath.Join(dir, "cache"),
	}, nil
}

// ListIdentities returns all identities found in the identities directory.
func ListIdentities() ([]Identity, error) {
	entries, err := os.ReadDir(identitiesDir())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var ids []Identity
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		ids = append(ids, Identity{
			Name:     name,
			HomeDir:  filepath.Join(identityDir(name), "home"),
			CacheDir: filepath.Join(identityDir(name), "cache"),
		})
	}
	return ids, nil
}

// IdentityExists reports whether a named identity directory exists.
func IdentityExists(name string) bool {
	_, err := os.Stat(identityDir(name))
	return err == nil
}

// RemoveIdentity deletes the identity directory. Requires force=true.
func RemoveIdentity(name string, force bool) error {
	dir := identityDir(name)
	if _, err := os.Stat(dir); err != nil {
		return fmt.Errorf("identity %q not found", name)
	}
	if !force {
		return fmt.Errorf("pass --force to remove identity %q and delete its home/cache directories", name)
	}
	return os.RemoveAll(dir)
}

func writeTemplateScripts(id *Identity) error {
	setupPath := filepath.Join(id.Dir(), "setup.sh")
	if _, err := os.Stat(setupPath); os.IsNotExist(err) {
		if err := os.WriteFile(setupPath, []byte(setupShTemplate), 0755); err != nil {
			return err
		}
	}
	onCreatePath := filepath.Join(id.Dir(), "on-create.sh")
	if _, err := os.Stat(onCreatePath); os.IsNotExist(err) {
		if err := os.WriteFile(onCreatePath, []byte(onCreateShTemplate), 0755); err != nil {
			return err
		}
	}
	return nil
}

const setupShTemplate = `#!/usr/bin/env bash
# setup.sh — runs on the HOST to prepare this identity's home directory.
#
# Run with:   airlock identity setup <name>
# Safe to re-run (use ln -sf for idempotent symlinks).
#
# IDENTITY_HOME is set to the absolute path of this identity's home/ directory.
# Use it to wire up files from your dotfiles repo or credential store.
# Always use $IDENTITY_HOME (not hardcoded paths) so this works on any machine.

# Examples:
#
# Link your git config:
#   ln -sf ~/dotfiles/.gitconfig "$IDENTITY_HOME/.gitconfig"
#
# Link your shell config:
#   ln -sf ~/dotfiles/.bashrc "$IDENTITY_HOME/.bashrc"
#
# Link an SSH key (one key at a time — never the whole ~/.ssh dir):
#   mkdir -p "$IDENTITY_HOME/.ssh"
#   chmod 700 "$IDENTITY_HOME/.ssh"
#   ln -sf ~/.config/airlock/identities/myid/.ssh/id_ed25519 "$IDENTITY_HOME/.ssh/id_ed25519"
`

const onCreateShTemplate = `#!/usr/bin/env bash
# on-create.sh — runs INSIDE the container on first creation.
#
# Triggered automatically by: airlock up (once per container lifetime).
# $HOME and $USER are set to the container image's user.
#
# Always use $HOME (not hardcoded paths like /home/ubuntu) so this script
# works correctly with any container image, regardless of the username.

# Examples:
#
# Set git identity:
#   git config --global user.name  "Your Name"
#   git config --global user.email "you@example.com"
#
# Install a shell prompt:
#   curl -fsSL https://starship.rs/install.sh | sh -s -- --yes
#
# Install a Node version manager:
#   curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.0/install.sh | bash
`
