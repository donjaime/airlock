package container

import (
	"errors"
	"os/exec"
)

type Engine string

const (
	EnginePodman Engine = "podman"
	EngineDocker Engine = "docker"
)

func DetectEngine(preferred string) (Engine, error) {
	if preferred != "" {
		if preferred == string(EnginePodman) && commandExists("podman") {
			return EnginePodman, nil
		}
		if preferred == string(EngineDocker) && commandExists("docker") {
			return EngineDocker, nil
		}
		return "", errors.New("preferred engine not found on PATH: " + preferred)
	}

	if commandExists("podman") {
		return EnginePodman, nil
	}
	if commandExists("docker") {
		return EngineDocker, nil
	}
	return "", errors.New("neither podman nor docker found on PATH")
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
