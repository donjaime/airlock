package global

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// ProjectEntry records how a project directory is bound to an image and identity.
// Exactly one of Image or (Containerfile + ImageTag) must be set.
type ProjectEntry struct {
	Name          string   `yaml:"name"`
	Image         string   `yaml:"image,omitempty"`
	Containerfile string   `yaml:"containerfile,omitempty"`
	ImageTag      string   `yaml:"imageTag,omitempty"`
	Identity      string   `yaml:"identity"`
	Ports         []string `yaml:"ports,omitempty"` // "host:container" pairs
}

// ResolvedImage returns the image name to pass to the container runtime.
func (e *ProjectEntry) ResolvedImage() string {
	if e.Image != "" {
		return e.Image
	}
	return e.ImageTag
}

type ProjectIndex struct {
	Version  int                        `yaml:"version"`
	Projects map[string]*ProjectEntry   `yaml:"projects"`
}

func projectsPath() string {
	return filepath.Join(Dir(), "projects.yaml")
}

func LoadProjectIndex() (*ProjectIndex, error) {
	b, err := os.ReadFile(projectsPath())
	if os.IsNotExist(err) {
		return &ProjectIndex{Version: 1, Projects: map[string]*ProjectEntry{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var idx ProjectIndex
	if err := yaml.Unmarshal(b, &idx); err != nil {
		return nil, err
	}
	if idx.Projects == nil {
		idx.Projects = map[string]*ProjectEntry{}
	}
	return &idx, nil
}

func (idx *ProjectIndex) Save() error {
	if err := os.MkdirAll(Dir(), 0700); err != nil {
		return err
	}
	b, err := yaml.Marshal(idx)
	if err != nil {
		return err
	}
	return os.WriteFile(projectsPath(), b, 0600)
}

// Find returns the entry for the given absolute path, if any.
func (idx *ProjectIndex) Find(absPath string) (*ProjectEntry, bool) {
	e, ok := idx.Projects[absPath]
	return e, ok
}

// FindByName returns all project paths whose entry name matches.
func (idx *ProjectIndex) FindByName(name string) []string {
	var paths []string
	for path, e := range idx.Projects {
		if e.Name == name {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	return paths
}

// Bind adds or replaces the entry for absPath.
func (idx *ProjectIndex) Bind(absPath string, entry *ProjectEntry) {
	idx.Projects[absPath] = entry
}

// Forget removes the entry for absPath.
func (idx *ProjectIndex) Forget(absPath string) {
	delete(idx.Projects, absPath)
}

// UsedImages returns the distinct resolved image names across all project entries.
func (idx *ProjectIndex) UsedImages() []string {
	seen := map[string]bool{}
	var images []string
	for _, e := range idx.Projects {
		img := e.ResolvedImage()
		if img != "" && !seen[img] {
			seen[img] = true
			images = append(images, img)
		}
	}
	sort.Strings(images)
	return images
}

// SortedPaths returns project paths in sorted order.
func (idx *ProjectIndex) SortedPaths() []string {
	paths := make([]string, 0, len(idx.Projects))
	for p := range idx.Projects {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
