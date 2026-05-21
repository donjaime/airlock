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

func identitiesDir() string {
	return filepath.Join(Dir(), "identities")
}

func identityDir(name string) string {
	return filepath.Join(identitiesDir(), name)
}

// CreateIdentity creates home/ and cache/ subdirs for a named identity.
func CreateIdentity(name string) (*Identity, error) {
	home := filepath.Join(identityDir(name), "home")
	cache := filepath.Join(identityDir(name), "cache")
	if err := os.MkdirAll(home, 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cache, 0700); err != nil {
		return nil, err
	}
	return &Identity{Name: name, HomeDir: home, CacheDir: cache}, nil
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
