package container

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donjaime/airlock/internal/config"
)

type Runner struct {
	Engine Engine
}

func NewRunner(e Engine) *Runner { return &Runner{Engine: e} }

func (r *Runner) Info(ctx context.Context, cfg *config.Config, absProjectDir string) (string, error) {
	homeHost := resolveHostPath(absProjectDir, cfg.Mounts.HomeDir)
	cacheHost := resolveHostPath(absProjectDir, cfg.Mounts.CacheDir)

	lines := []string{
		"engine: " + string(r.Engine),
		"config.name: " + cfg.Name,
		"projectDir: " + absProjectDir,
		"containerName: " + containerName(cfg),
		"imageTag: " + cfg.Image.Tag,
		"workdir: " + cfg.Mounts.Workdir,
		"homeHostDir: " + homeHost,
		"cacheHostDir: " + cacheHost,
	}
	return strings.Join(lines, "\n"), nil
}

func (r *Runner) Up(ctx context.Context, cfg *config.Config, absProjectDir string) error {
	if cfg.Image.Build {
		if err := r.buildImage(ctx, cfg, absProjectDir); err != nil {
			return err
		}
	}

	homeHost := resolveHostPath(absProjectDir, cfg.Mounts.HomeDir)
	cacheHost := resolveHostPath(absProjectDir, cfg.Mounts.CacheDir)
	if err := os.MkdirAll(homeHost, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(cacheHost, 0700); err != nil {
		return err
	}

	exists, err := r.containerExists(ctx, containerName(cfg))
	if err != nil {
		return err
	}
	if !exists {
		return r.createContainer(ctx, cfg, absProjectDir, homeHost, cacheHost)
	}

	running, err := r.containerRunning(ctx, containerName(cfg))
	if err != nil {
		return err
	}
	if !running {
		return r.runCmdInteractive(ctx, r.engineBin(), "start", containerName(cfg))
	}
	return nil
}

func (r *Runner) Enter(ctx context.Context, cfg *config.Config, absProjectDir string, env []string) error {
	args := []string{"exec", "-it"}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(cfg), "bash", "-l")
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) Exec(ctx context.Context, cfg *config.Config, absProjectDir string, env []string, cmd []string) error {
	args := []string{"exec", "-it"}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(cfg))
	args = append(args, cmd...)
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) Down(ctx context.Context, cfg *config.Config, absProjectDir string) error {
	_ = r.runCmdInteractive(ctx, r.engineBin(), "stop", containerName(cfg))
	_ = r.runCmdInteractive(ctx, r.engineBin(), "rm", "-f", containerName(cfg))
	return nil
}

func (r *Runner) engineBin() string {
	if r.Engine == EngineDocker {
		return "docker"
	}
	return "podman"
}

func (r *Runner) buildImage(ctx context.Context, cfg *config.Config, absProjectDir string) error {
	df := cfg.Image.Containerfile
	if !filepath.IsAbs(df) {
		df = filepath.Join(absProjectDir, df)
	}
	args := []string{"build", "-t", cfg.Image.Tag, "-f", df, absProjectDir}
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) containerExists(ctx context.Context, name string) (bool, error) {
	cmd := exec.CommandContext(ctx, r.engineBin(), "container", "inspect", name)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func (r *Runner) containerRunning(ctx context.Context, name string) (bool, error) {
	out, err := exec.CommandContext(ctx, r.engineBin(), "inspect", "-f", "{{.State.Running}}", name).CombinedOutput()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (r *Runner) createContainer(ctx context.Context, cfg *config.Config, absProjectDir, homeHost, cacheHost string) error {
	name := containerName(cfg)
	workdir := cfg.Mounts.Workdir

	envArgs := []string{
		"-e", "HOME=/home/agent",
		"-e", "XDG_CACHE_HOME=/home/agent/.cache",
		"-e", "XDG_CONFIG_HOME=/home/agent/.config",
		"-e", "XDG_DATA_HOME=/home/agent/.local/share",
		"-e", "WORKDIR=" + workdir,
	}
	for k, v := range cfg.Env.Vars {
		envArgs = append(envArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	mountArgs := []string{
		"-v", absProjectDir + ":" + workdir + ":Z",
		"-v", homeHost + ":/home/agent:Z",
		"-v", cacheHost + ":/home/agent/.cache:Z",
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"-w", workdir,
	}
	args = append(args, envArgs...)
	args = append(args, mountArgs...)
	args = append(args, "--hostname", "airlock")
	args = append(args, cfg.Image.Tag)
	args = append(args, "bash", "-lc", "tail -f /dev/null")

	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) runCmdInteractive(ctx context.Context, bin string, args ...string) error {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func containerName(cfg *config.Config) string {
	return "airlock-" + cfg.Name
}

func resolveHostPath(projectAbs, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Clean(filepath.Join(projectAbs, p))
}
