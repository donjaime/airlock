package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donjaime/airlock/internal/config"
)

type UserConfig struct {
	Name    string
	Home    string
	WorkDir string
	Env     []string
}

type Runner struct {
	Engine  Engine
	Verbose bool
}

func NewRunner(e Engine) *Runner { return &Runner{Engine: e} }

func (r *Runner) Info(ctx context.Context, cfg *config.Config, absProjectDir string) (string, error) {
	homeHost := resolveHostPath(absProjectDir, cfg.HomeDir)
	cacheHost := resolveHostPath(absProjectDir, cfg.CacheDir)
	workDirHost := resolveHostPath(absProjectDir, cfg.WorkDir)

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
		"workHostDir: " + workDirHost,
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

	image := cfg.Image
	if cfg.Build != nil {
		image = cfg.Build.Tag
	}

	userConfig, err := r.inspectImage(ctx, image)
	if err != nil {
		return err
	}

	homeHost := resolveHostPath(absProjectDir, cfg.HomeDir)
	cacheHost := resolveHostPath(absProjectDir, cfg.CacheDir)
	workDirHost := resolveHostPath(absProjectDir, cfg.WorkDir)
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
		if err := r.createContainer(ctx, cfg, userConfig, absProjectDir, homeHost, cacheHost, workDirHost); err != nil {
			return err
		}
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
	image := cfg.Image
	if cfg.Build != nil {
		image = cfg.Build.Tag
	}
	userConfig, err := r.inspectImage(ctx, image)
	if err != nil {
		return err
	}
	args := []string{"exec", "-it", "--user", fmt.Sprintf("%s", userConfig.Name)}
	for _, e := range env {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(cfg), "bash", "-l")
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) Exec(ctx context.Context, cfg *config.Config, absProjectDir string, env []string, cmd []string) error {
	image := cfg.Image
	if cfg.Build != nil {
		image = cfg.Build.Tag
	}
	userConfig, err := r.inspectImage(ctx, image)
	if err != nil {
		return err
	}
	args := []string{"exec", "-it", "--user", fmt.Sprintf("%s", userConfig.Name)}
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
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s %s\n", r.engineBin(), strings.Join([]string{"ps", "--filter", "name=^airlock-", "--format", "{{.Names}}"}, " "))
	}
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

func (r *Runner) inspectImage(ctx context.Context, image string) (*UserConfig, error) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s image inspect %s\n", r.engineBin(), image)
	}
	cmd := exec.CommandContext(ctx, r.engineBin(), "image", "inspect", "--format", "json", image)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image %s: %w", image, err)
	}

	var data []struct {
		Config struct {
			User       string   `json:"User"`
			WorkingDir string   `json:"WorkingDir"`
			Env        []string `json:"Env"`
		} `json:"Config"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, fmt.Errorf("failed to parse image inspect output: %w", err)
	}

	if len(data) == 0 {
		return nil, fmt.Errorf("no data returned from image inspect %s", image)
	}

	userStr := data[0].Config.User
	workdir := data[0].Config.WorkingDir
	env := data[0].Config.Env

	// Default to inheriting host uid if not specified
	if userStr == "" {
		userStr = "1000"
	}

	userConfig := &UserConfig{
		Name:    userStr,
		WorkDir: workdir,
		Env:     env,
	}

	// Now we need to find the home directory. This is tricky because it depends on the user inside the container.
	// Common convention is /home/username or /root.
	// If we have a username but not UID/GID, we might need to look it up in the image (e.g. /etc/passwd).
	// For now, let's make some assumptions or try to find it.
	if userConfig.Name == "root" {
		userConfig.Home = "/root"
	} else if userConfig.Name != "" {
		userConfig.Home = "/home/" + userConfig.Name
	}
	return userConfig, nil
}

func (r *Runner) containerExists(ctx context.Context, name string) (bool, error) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s container inspect %s\n", r.engineBin(), name)
	}
	cmd := exec.CommandContext(ctx, r.engineBin(), "container", "inspect", name)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func (r *Runner) containerRunning(ctx context.Context, name string) (bool, error) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s inspect -f {{.State.Running}} %s\n", r.engineBin(), name)
	}
	out, err := exec.CommandContext(ctx, r.engineBin(), "inspect", "-f", "{{.State.Running}}", name).CombinedOutput()
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(string(out)) == "true", nil
}

func (r *Runner) createContainer(ctx context.Context, cfg *config.Config, u *UserConfig, absProjectDir, homeHost, cacheHost, workDirHost string) error {
	name := containerName(cfg)

	// Build the environment map, starting with image defaults, then airlock.yaml, then airlock overrides.
	envMap := make(map[string]string)
	for _, e := range u.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	for k, v := range cfg.Env {
		envMap[k] = v
	}

	home := u.Home
	// Airlock specific overrides
	envMap["HOME"] = home
	envMap["XDG_CACHE_HOME"] = home + "/.cache"
	envMap["XDG_CONFIG_HOME"] = home + "/.config"
	envMap["XDG_DATA_HOME"] = home + "/.local/share"
	envMap["WORKDIR"] = u.WorkDir

	var envArgs []string
	for k, v := range envMap {
		envArgs = append(envArgs, "-e", fmt.Sprintf("%s=%s", k, v))
	}

	mountArgs := []string{
		"-v", homeHost + ":" + home + ":Z",
		"-v", cacheHost + ":" + home + "/.cache:Z",
	}

	workdirMounted := false
	for _, m := range cfg.Mounts {
		src := resolveHostPath(absProjectDir, m.Source)
		if m.Target == u.WorkDir {
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
		mountArgs = append([]string{"-v", workDirHost + ":" + u.WorkDir + ":Z"}, mountArgs...)
	}

	// Always hide .airlock folder from the working directory mount
	mountArgs = append(mountArgs, "-v", u.WorkDir+"/.airlock")

	args := []string{
		"run", "-d",
		"--name", name,
		"-w", u.WorkDir,
		"--user", fmt.Sprintf("%s", u.Name),
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
	// args = append(args, "sleep", "infinity")

	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) runCmdInteractive(ctx context.Context, bin string, args ...string) error {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s %s\n", bin, strings.Join(args, " "))
	}
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
