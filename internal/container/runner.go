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
	homeHost := resolveHostPath(absProjectDir, cfg.HomeDir)
	cacheHost := resolveHostPath(absProjectDir, cfg.CacheDir)

	image := cfg.Image
	if cfg.Build != nil {
		image = cfg.Build.Tag
	}

	lines := []string{
		"engine: " + string(r.Engine),
		"config.name: " + cfg.Name,
		"projectDir: " + absProjectDir,
		"containerName: " + containerName(cfg),
		"image: " + image,
		"workdir: " + cfg.Workdir,
		"homeHostDir: " + homeHost,
		"cacheHostDir: " + cacheHost,
	}
	return strings.Join(lines, "\n"), nil
}

func (r *Runner) Up(ctx context.Context, cfg *config.Config, absProjectDir string) error {
	if cfg.Build != nil {
		if err := r.buildImage(ctx, cfg, absProjectDir); err != nil {
			return err
		}
	}

	homeHost := resolveHostPath(absProjectDir, cfg.HomeDir)
	cacheHost := resolveHostPath(absProjectDir, cfg.CacheDir)
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
	args := []string{"exec", "-it", "--user", fmt.Sprintf("%d:%d", cfg.User.UID, cfg.User.GID)}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(cfg), "bash", "-l")
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) Exec(ctx context.Context, cfg *config.Config, absProjectDir string, env []string, cmd []string) error {
	args := []string{"exec", "-it", "--user", fmt.Sprintf("%d:%d", cfg.User.UID, cfg.User.GID)}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(cfg))
	args = append(args, cmd...)
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) Down(ctx context.Context, cfg *config.Config, name string) error {
	target := name
	if target == "" {
		target = containerName(cfg)
	} else if !strings.HasPrefix(target, "airlock-") {
		target = "airlock-" + target
	}
	_ = r.runCmdInteractive(ctx, r.engineBin(), "stop", target)
	_ = r.runCmdInteractive(ctx, r.engineBin(), "rm", "-f", target)
	return nil
}

func (r *Runner) List(ctx context.Context) ([]string, error) {
	// We use --filter name=^airlock- to match containers starting with airlock-
	// Both podman and docker support this.
	// We don't use -a because the requirement is to show "running" containers.
	cmd := exec.CommandContext(ctx, r.engineBin(), "ps", "--filter", "name=^airlock-", "--format", "{{.Names}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	var names []string
	for _, line := range lines {
		name := strings.TrimSpace(line)
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func (r *Runner) engineBin() string {
	if r.Engine == EngineDocker {
		return "docker"
	}
	return "podman"
}

func (r *Runner) buildImage(ctx context.Context, cfg *config.Config, absProjectDir string) error {
	df := cfg.Build.Containerfile
	if !filepath.IsAbs(df) {
		df = filepath.Join(absProjectDir, df)
	}
	args := []string{"build", "-t", cfg.Build.Tag, "-f", df, cfg.Build.Context}
	if !filepath.IsAbs(cfg.Build.Context) {
		args[len(args)-1] = filepath.Join(absProjectDir, cfg.Build.Context)
	}
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
	workdir := cfg.Workdir

	home := cfg.User.Home
	envArgs := []string{
		"-e", "HOME=" + home,
		"-e", "XDG_CACHE_HOME=" + home + "/.cache",
		"-e", "XDG_CONFIG_HOME=" + home + "/.config",
		"-e", "XDG_DATA_HOME=" + home + "/.local/share",
		"-e", "WORKDIR=" + workdir,
	}
	for k, v := range cfg.Env.Vars {
		envArgs = append(envArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	mountArgs := []string{
		"-v", homeHost + ":" + home + ":Z",
		"-v", cacheHost + ":" + home + "/.cache:Z",
	}

	workdirMounted := false
	for _, m := range cfg.Mounts {
		src := resolveHostPath(absProjectDir, m.Source)
		if m.Target == workdir {
			workdirMounted = true
		}
		mode := m.Mode
		if mode == "" {
			mode = "rw"
		}
		// We add :Z for podman relabeling, similar to other mounts
		mountArgs = append(mountArgs, "-v", fmt.Sprintf("%s:%s:%s,Z", src, m.Target, mode))
	}

	if !workdirMounted {
		mountArgs = append([]string{"-v", absProjectDir + ":" + workdir + ":Z"}, mountArgs...)
	}

	args := []string{
		"run", "-d",
		"--name", name,
		"-w", workdir,
		"--user", fmt.Sprintf("%d:%d", cfg.User.UID, cfg.User.GID),
	}
	if r.Engine == EnginePodman {
		args = append(args, "--userns=keep-id")
	}
	args = append(args, envArgs...)
	args = append(args, mountArgs...)
	args = append(args, "--hostname", "airlock")
	image := cfg.Image
	if cfg.Build != nil {
		image = cfg.Build.Tag
	}
	args = append(args, image)
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
