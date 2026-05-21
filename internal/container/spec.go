package container

// ContainerSpec holds all resolved parameters needed to create and run a container.
// Exactly one of Image or (Containerfile + ImageTag) should be set.
type ContainerSpec struct {
	Name           string
	Image          string // direct image ref (registry or local)
	Containerfile  string // absolute path; if set, airlock builds before running
	ImageTag       string // tag for the built image when Containerfile is set
	HomeHost       string // absolute host path mounted as $HOME in the container
	CacheHost      string // absolute host path mounted as $HOME/.cache
	WorkDirHost    string // absolute host path (project dir) mounted as the container workdir
	Identity       string // identity name, for display only
	OnCreateScript string // absolute host path to on-create.sh; empty if absent
	Ports          []string // "host:container" pairs forwarded with -p
	Engine         Engine
}

// ResolvedImage returns the image name to pass to the container runtime.
func (s *ContainerSpec) ResolvedImage() string {
	if s.Image != "" {
		return s.Image
	}
	return s.ImageTag
}
