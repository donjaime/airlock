package container

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type UserConfig struct {
	Name    string
	Home    string
	WorkDir string
	Env     []string
	UID     int
	GID     int
}

type Runner struct {
	Engine  Engine
	Verbose bool
}

func NewRunner(e Engine) *Runner { return &Runner{Engine: e} }

func (r *Runner) Up(ctx context.Context, spec *ContainerSpec) error {
	if spec.Containerfile != "" {
		if err := r.buildImage(ctx, spec); err != nil {
			return err
		}
	}

	image := spec.ResolvedImage()
	userConfig, err := r.inspectImage(ctx, image)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(spec.HomeHost, 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(spec.CacheHost, 0700); err != nil {
		return err
	}

	name := containerName(spec)
	exists, err := r.containerExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		if r.Engine == EnginePodman {
			uid, gid, err := r.resolveContainerUID(ctx, image, userConfig.Name)
			if err != nil {
				return err
			}
			userConfig.UID = uid
			userConfig.GID = gid
		}
		if err := r.createContainer(ctx, spec, userConfig); err != nil {
			return err
		}
	}

	running, err := r.containerRunning(ctx, name)
	if err != nil {
		return err
	}
	if !running {
		if err := r.runCmd(ctx, r.engineBin(), "start", name); err != nil {
			return fmt.Errorf(
				"container %q exists but failed to start (volume mounts may be stale from a previous configuration).\n"+
					"Run `airlock down` then `airlock up` to recreate it: %w", name, err)
		}
	}

	return r.bootstrapHome(ctx, spec, userConfig)
}

// bootstrapHome seeds a fresh identity home dir with /etc/skel defaults and runs
// on-create.sh if present. Guarded by a sentinel file so it only runs once.
func (r *Runner) bootstrapHome(ctx context.Context, spec *ContainerSpec, u *UserConfig) error {
	sentinel := filepath.Join(spec.HomeHost, ".airlock-bootstrapped")
	if _, err := os.Stat(sentinel); err == nil {
		return nil // already bootstrapped
	}

	name := containerName(spec)
	fmt.Println("Bootstrapping home directory...")

	// Layer 1: copy /etc/skel into $HOME (distro-appropriate shell defaults)
	skelScript := fmt.Sprintf(`[ -d /etc/skel ] && cp -rn /etc/skel/. "%s/" 2>/dev/null || true`, u.Home)
	if err := r.runCmd(ctx, r.engineBin(), "exec", "--user", u.Name, name, "sh", "-c", skelScript); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not copy /etc/skel (continuing): %v\n", err)
	}

	// Layer 2: run on-create.sh inside the container if provided
	if spec.OnCreateScript != "" {
		fmt.Println("Running on-create.sh...")
		tmpPath := "/tmp/.airlock-on-create.sh"
		if err := r.runCmd(ctx, r.engineBin(), "cp", spec.OnCreateScript, name+":"+tmpPath); err != nil {
			return fmt.Errorf("failed to copy on-create.sh into container: %w", err)
		}
		runScript := fmt.Sprintf(`bash "%s"; ec=$?; rm -f "%s"; exit $ec`, tmpPath, tmpPath)
		if err := r.runCmdInteractive(ctx, r.engineBin(), "exec", "--user", u.Name, name, "sh", "-c", runScript); err != nil {
			return fmt.Errorf("on-create.sh failed: %w", err)
		}
	}

	// Write sentinel to prevent re-running
	return os.WriteFile(sentinel, []byte(""), 0600)
}

func (r *Runner) Enter(ctx context.Context, spec *ContainerSpec, extraEnv []string) error {
	userConfig, err := r.inspectImage(ctx, spec.ResolvedImage())
	if err != nil {
		return err
	}
	mergedEnv := r.getMergedEnv(spec, userConfig, extraEnv)
	args := []string{"exec", "-it", "--user", userConfig.Name}
	for _, e := range mergedEnv {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(spec), "bash")
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

func (r *Runner) Exec(ctx context.Context, spec *ContainerSpec, extraEnv []string, cmd []string) error {
	userConfig, err := r.inspectImage(ctx, spec.ResolvedImage())
	if err != nil {
		return err
	}
	mergedEnv := r.getMergedEnv(spec, userConfig, extraEnv)
	args := []string{"exec", "-it", "--user", userConfig.Name}
	for _, e := range mergedEnv {
		args = append(args, "-e", e)
	}
	args = append(args, containerName(spec))
	args = append(args, cmd...)
	return r.runCmdInteractive(ctx, r.engineBin(), args...)
}

// Down stops and removes the container with the given full container name.
func (r *Runner) Down(ctx context.Context, name string) error {
	_ = r.runCmdInteractive(ctx, r.engineBin(), "stop", name)
	_ = r.runCmdInteractive(ctx, r.engineBin(), "rm", "-f", name)
	return nil
}

// List returns the names of all running airlock-* containers.
func (r *Runner) List(ctx context.Context) ([]string, error) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s ps --filter name=^airlock- --format {{.Names}}\n", r.engineBin())
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

func (r *Runner) getMergedEnv(spec *ContainerSpec, u *UserConfig, extraEnv []string) []string {
	envMap := make(map[string]string)

	// 1. Image defaults
	for _, e := range u.Env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	// 2. CLI overrides: KEY=VALUE sets directly; KEY alone forwards from host env
	for _, e := range extraEnv {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		} else if val, ok := os.LookupEnv(e); ok {
			envMap[e] = val
		}
	}

	// 3. Airlock internals (always override)
	home := u.Home
	envMap["HOME"] = home
	envMap["XDG_CACHE_HOME"] = home + "/.cache"
	envMap["XDG_CONFIG_HOME"] = home + "/.config"
	envMap["XDG_DATA_HOME"] = home + "/.local/share"
	envMap["WORKDIR"] = u.WorkDir

	var env []string
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}

func (r *Runner) engineBin() string {
	if r.Engine == EngineDocker {
		return "docker"
	}
	return "podman"
}

func (r *Runner) buildImage(ctx context.Context, spec *ContainerSpec) error {
	context := filepath.Dir(spec.Containerfile)
	args := []string{"build", "-t", spec.ImageTag, "-f", spec.Containerfile, context}
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

	if userStr == "" {
		userStr = "root"
	}
	// Default workdir for images that don't set one
	if workdir == "" || workdir == "/" {
		workdir = "/workspace"
	}

	uc := &UserConfig{
		Name:    userStr,
		WorkDir: workdir,
		Env:     env,
	}
	if uc.Name == "root" {
		uc.Home = "/root"
	} else {
		uc.Home = "/home/" + uc.Name
	}
	return uc, nil
}

func (r *Runner) resolveContainerUID(ctx context.Context, image, user string) (int, int, error) {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s run --rm --user %s --entrypoint sh %s -c 'echo $(id -u):$(id -g)'\n", r.engineBin(), user, image)
	}
	cmd := exec.CommandContext(ctx, r.engineBin(), "run", "--rm", "--user", user, "--entrypoint", "sh", image, "-c", "echo $(id -u):$(id -g)")
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to resolve UID/GID for user %s in image %s: %w", user, image, err)
	}
	parts := strings.SplitN(strings.TrimSpace(string(out)), ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected id output: %s", string(out))
	}
	uid, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse UID %q: %w", parts[0], err)
	}
	gid, err := strconv.Atoi(parts[1])
	if err != nil {
		return uid, 0, fmt.Errorf("failed to parse GID %q: %w", parts[1], err)
	}
	return uid, gid, nil
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

func (r *Runner) createContainer(ctx context.Context, spec *ContainerSpec, u *UserConfig) error {
	name := containerName(spec)
	mergedEnv := r.getMergedEnv(spec, u, nil)

	var envArgs []string
	for _, e := range mergedEnv {
		envArgs = append(envArgs, "-e", e)
	}

	label := volumeLabel()
	mountArgs := []string{
		"-v", spec.WorkDirHost + ":" + u.WorkDir + label,
		"-v", spec.HomeHost + ":" + u.Home + label,
		"-v", spec.CacheHost + ":" + u.Home + "/.cache" + label,
	}

	args := []string{
		"run", "-d",
		"--init",
		"--name", name,
		"-w", u.WorkDir,
		"--user", u.Name,
	}
	if r.Engine == EnginePodman {
		args = append(args, fmt.Sprintf("--userns=keep-id:uid=%d,gid=%d", u.UID, u.GID))
	}
	args = append(args, envArgs...)
	args = append(args, mountArgs...)
	for _, p := range spec.Ports {
		args = append(args, "-p", p)
	}
	args = append(args, "--hostname", "airlock")
	args = append(args, spec.ResolvedImage())
	args = append(args, "sleep", "infinity")

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

// runCmd runs a non-interactive command, piping stdout/stderr to the terminal but
// not connecting stdin. Used for background operations like starting containers and
// copying files.
func (r *Runner) runCmd(ctx context.Context, bin string, args ...string) error {
	if r.Verbose {
		fmt.Fprintf(os.Stderr, "+ %s %s\n", bin, strings.Join(args, " "))
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func containerName(spec *ContainerSpec) string {
	return "airlock-" + spec.Name
}

func volumeLabel() string {
	if runtime.GOOS == "linux" {
		return ":Z"
	}
	return ""
}
