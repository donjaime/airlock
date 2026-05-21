package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/donjaime/airlock/internal/container"
	"github.com/donjaime/airlock/internal/global"
)

const version = "0.6.0"

var (
	verboseFlag bool
	envVarsFlag stringSlice
)

func init() {
	flag.BoolVar(&verboseFlag, "v", false, "Enable verbose output (print underlying podman/docker commands)")
	flag.Var(&envVarsFlag, "e", "Forward environment variable into the container (KEY or KEY=VALUE); repeatable")
	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, `airlock v%s

Usage:
  airlock [-e var] [-v] <command> [args]

Commands:
  init [-p host:container] Set up airlock for the current directory
  forget [name]            Remove a project from the airlock index
  identity list            List all identities
  identity add <name>      Create a new identity (with setup.sh and on-create.sh templates)
  identity setup <name>    Run setup.sh on the host (wire up dotfiles/credentials)
  identity remove <name>   Remove an identity (requires --force to delete files)
  up                       Build image (if needed) and start the container
  enter                    Open an interactive shell in the container
  exec -- <cmd>            Run a command inside the container
  down [name]              Stop and remove a container
  list                     List all known projects with status
  info                     Show resolved config for current directory
  version                  Print version
  help                     Print this help

Examples:
  airlock init
  airlock up
  airlock -e ANTHROPIC_API_KEY enter
  airlock exec -- go test ./...
  airlock down
  airlock forget my-old-project
  airlock list

Flags:
`, version)
	flag.PrintDefaults()
}

func main() {
	flag.Parse()
	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}

	cmd := args[0]
	cmdArgs := args[1:]
	ctx := context.Background()

	switch cmd {
	case "help":
		usage()

	case "version":
		fmt.Println(version)

	case "init":
		initFlags := flag.NewFlagSet("init", flag.ExitOnError)
		var portFlags stringSlice
		initFlags.Var(&portFlags, "p", "Forward port from host to container (host:container or port); repeatable")
		_ = initFlags.Parse(cmdArgs)
		runInit([]string(portFlags))

	case "forget":
		name := ""
		if len(cmdArgs) > 0 {
			name = cmdArgs[0]
		}
		runForget(ctx, name)

	case "identity":
		runIdentity(cmdArgs)

	case "list":
		runList(ctx)

	case "down":
		gcfg := mustLoadGlobalConfig()
		eng := mustDetectEngine(gcfg.Engine)
		r := newRunner(eng)
		name := ""
		if len(cmdArgs) > 0 {
			name = cmdArgs[0]
		}
		runDown(ctx, r, name)

	case "info":
		gcfg := mustLoadGlobalConfig()
		eng := mustDetectEngine(gcfg.Engine)
		spec := mustLoadSpec(gcfg, eng)
		fmt.Printf("engine:    %s\n", string(eng))
		fmt.Printf("name:      %s\n", spec.Name)
		fmt.Printf("container: airlock-%s\n", spec.Name)
		fmt.Printf("image:     %s\n", spec.ResolvedImage())
		fmt.Printf("identity:  %s\n", spec.Identity)
		fmt.Printf("homeDir:   %s\n", spec.HomeHost)
		fmt.Printf("cacheDir:  %s\n", spec.CacheHost)
		fmt.Printf("workDir:   %s\n", spec.WorkDirHost)
		if len(spec.Ports) > 0 {
			fmt.Printf("ports:     %s\n", strings.Join(spec.Ports, ", "))
		}

	case "up":
		gcfg := mustLoadGlobalConfig()
		eng := mustDetectEngine(gcfg.Engine)
		r := newRunner(eng)
		spec := mustLoadSpec(gcfg, eng)
		if err := r.Up(ctx, spec); err != nil {
			fatalf("up error: %v\n", err)
		}

	case "enter":
		gcfg := mustLoadGlobalConfig()
		eng := mustDetectEngine(gcfg.Engine)
		r := newRunner(eng)
		spec := mustLoadSpec(gcfg, eng)
		if err := r.Up(ctx, spec); err != nil {
			fatalf("up error: %v\n", err)
		}
		if err := r.Enter(ctx, spec, []string(envVarsFlag)); err != nil {
			fatalf("enter error: %v\n", err)
		}

	case "exec":
		if len(cmdArgs) == 0 {
			fatalf("exec requires a command, e.g. airlock exec -- go test ./...\n")
		}
		if cmdArgs[0] == "--" {
			cmdArgs = cmdArgs[1:]
		}
		gcfg := mustLoadGlobalConfig()
		eng := mustDetectEngine(gcfg.Engine)
		r := newRunner(eng)
		spec := mustLoadSpec(gcfg, eng)
		if err := r.Up(ctx, spec); err != nil {
			fatalf("up error: %v\n", err)
		}
		if err := r.Exec(ctx, spec, []string(envVarsFlag), cmdArgs); err != nil {
			fatalf("exec error: %v\n", err)
		}

	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

// --- init ---

func runInit(portFlags []string) {
	cwd, err := os.Getwd()
	if err != nil {
		fatalf("Failed to get current directory: %v\n", err)
	}

	idx, err := global.LoadProjectIndex()
	if err != nil {
		fatalf("Failed to load project index: %v\n", err)
	}

	fmt.Printf("Setting up airlock for %s\n\n", cwd)

	// Normalize bare port numbers to "port:port"
	ports := normalizePorts(portFlags)

	// Warn if already initialized
	if existing, ok := idx.Find(cwd); ok {
		fmt.Printf("This directory is already initialized:\n")
		fmt.Printf("  image:    %s\n", existing.ResolvedImage())
		fmt.Printf("  identity: %s\n", existing.Identity)
		if len(existing.Ports) > 0 {
			fmt.Printf("  ports:    %s\n", strings.Join(existing.Ports, ", "))
		}
		fmt.Println()
		yes, err := global.PromptConfirm("Re-initialize?")
		if err != nil || !yes {
			return
		}
		// Preserve existing ports if none specified on the command line
		if len(ports) == 0 {
			ports = existing.Ports
		}
		fmt.Println()
	}

	// Step 1: pick image
	image, containerfile, imageTag := promptImage(idx, filepath.Base(cwd))

	// Step 2: pick identity
	identityName := promptIdentity()

	// Step 3: project name (with collision warning)
	defaultName := sanitizeName(filepath.Base(cwd))
	warnNameCollisions(idx, defaultName, cwd)
	projectName, err := global.PromptText("Project name", defaultName)
	if err != nil {
		fatalf("Failed to read project name: %v\n", err)
	}
	fmt.Println()

	entry := &global.ProjectEntry{
		Name:          projectName,
		Image:         image,
		Containerfile: containerfile,
		ImageTag:      imageTag,
		Identity:      identityName,
		Ports:         ports,
	}
	idx.Bind(cwd, entry)
	if err := idx.Save(); err != nil {
		fatalf("Failed to save project index: %v\n", err)
	}

	fmt.Printf("Saved to %s\n", filepath.Join(global.Dir(), "projects.yaml"))
	if len(ports) > 0 {
		fmt.Printf("Ports:    %s\n", strings.Join(ports, ", "))
	}
	fmt.Printf("Run `airlock up` to start the container.\n")
}

func promptImage(idx *global.ProjectIndex, baseName string) (image, containerfile, imageTag string) {
	usedImages := idx.UsedImages()

	choices := make([]string, 0, len(usedImages)+2)
	choices = append(choices, usedImages...)
	choices = append(choices, "Enter a different image ref...")
	choices = append(choices, "Build from a Containerfile...")

	if len(usedImages) == 0 {
		fmt.Println("No previously used images on this system.")
	} else {
		fmt.Println("Previously used images:")
	}
	picked, err := global.PromptSelect("Select image", choices)
	if err != nil {
		fatalf("Selection failed: %v\n", err)
	}
	fmt.Println()

	switch {
	case picked < len(usedImages):
		image = usedImages[picked]
	case choices[picked] == "Enter a different image ref...":
		image, err = global.PromptText("Image ref (e.g. ubuntu:24.04, docker.io/golang:1.22)", "")
		if err != nil || image == "" {
			fatalf("No image specified.\n")
		}
		fmt.Println()
	default: // Build from Containerfile
		containerfile, err = global.PromptText("Containerfile path", "./Containerfile")
		if err != nil || containerfile == "" {
			fatalf("No Containerfile path specified.\n")
		}
		if !filepath.IsAbs(containerfile) {
			cwd, _ := os.Getwd()
			containerfile = filepath.Join(cwd, containerfile)
		}
		defaultTag := "airlock-" + sanitizeName(baseName) + ":latest"
		imageTag, err = global.PromptText("Image tag", defaultTag)
		if err != nil || imageTag == "" {
			fatalf("No image tag specified.\n")
		}
		fmt.Println()
	}
	return
}

func promptIdentity() string {
	identities, err := global.ListIdentities()
	if err != nil {
		fatalf("Failed to list identities: %v\n", err)
	}

	if len(identities) == 0 {
		fmt.Println("No identities found. Creating a default identity automatically.")
		if _, err := global.CreateIdentity("default"); err != nil {
			fatalf("Failed to create default identity: %v\n", err)
		}
		fmt.Printf("Created %s\n\n", filepath.Join(global.Dir(), "identities", "default"))
		return "default"
	}

	choices := make([]string, 0, len(identities)+1)
	for _, id := range identities {
		choices = append(choices, id.Name)
	}
	choices = append(choices, "Create a new identity...")

	fmt.Println("Available identities:")
	picked, err := global.PromptSelect("Select identity", choices)
	if err != nil {
		fatalf("Selection failed: %v\n", err)
	}
	fmt.Println()

	if picked < len(identities) {
		return identities[picked].Name
	}

	// Create new identity
	name, err := global.PromptText("Identity name", "")
	if err != nil || name == "" {
		fatalf("No identity name specified.\n")
	}
	if _, err := global.CreateIdentity(name); err != nil {
		fatalf("Failed to create identity %q: %v\n", name, err)
	}
	fmt.Printf("Created %s\n\n", filepath.Join(global.Dir(), "identities", name))
	return name
}

func warnNameCollisions(idx *global.ProjectIndex, name, cwd string) {
	var collisions []string
	for _, p := range idx.FindByName(name) {
		if p != cwd {
			collisions = append(collisions, p)
		}
	}
	if len(collisions) > 0 {
		fmt.Printf("Note: name %q is already used by %s.\n", name, strings.Join(collisions, ", "))
		fmt.Printf("Both projects would share the container airlock-%s.\n\n", name)
	}
}

// --- forget ---

func runForget(ctx context.Context, nameArg string) {
	idx, err := global.LoadProjectIndex()
	if err != nil {
		fatalf("Failed to load project index: %v\n", err)
	}
	if len(idx.Projects) == 0 {
		fmt.Println("No projects in the airlock index.")
		return
	}

	var targetPath string

	if nameArg != "" {
		paths := idx.FindByName(nameArg)
		if len(paths) == 0 {
			fatalf("No project named %q found.\n", nameArg)
		}
		if len(paths) == 1 {
			targetPath = paths[0]
		} else {
			fmt.Printf("Multiple projects named %q:\n", nameArg)
			i, err := global.PromptSelect("Which one?", paths)
			if err != nil {
				fatalf("Selection failed: %v\n", err)
			}
			targetPath = paths[i]
		}
	} else {
		cwd, _ := os.Getwd()
		if _, ok := idx.Find(cwd); ok {
			targetPath = cwd
		} else {
			paths := idx.SortedPaths()
			choices := make([]string, len(paths))
			for i, p := range paths {
				e := idx.Projects[p]
				choices[i] = fmt.Sprintf("%-25s %s", e.Name, p)
			}
			i, err := global.PromptSelect("Forget which project?", choices)
			if err != nil {
				fatalf("Selection failed: %v\n", err)
			}
			targetPath = paths[i]
		}
	}

	entry, _ := idx.Find(targetPath)
	yes, err := global.PromptConfirm(fmt.Sprintf("Forget %s (%s)?", entry.Name, targetPath))
	if err != nil || !yes {
		return
	}

	idx.Forget(targetPath)
	if err := idx.Save(); err != nil {
		fatalf("Failed to save project index: %v\n", err)
	}
	fmt.Println("Removed.")
}

// --- identity ---

func runIdentity(args []string) {
	if len(args) < 1 {
		fatalf("Usage: airlock identity <list|add|setup|remove>\n")
	}
	switch args[0] {
	case "setup":
		if len(args) < 2 {
			fatalf("Usage: airlock identity setup <name>\n")
		}
		runIdentitySetup(args[1])

	case "list":
		ids, err := global.ListIdentities()
		if err != nil {
			fatalf("Failed to list identities: %v\n", err)
		}
		if len(ids) == 0 {
			fmt.Println("No identities. Use `airlock identity add <name>` to create one.")
			return
		}
		for _, id := range ids {
			fmt.Printf("%-20s %s\n", id.Name, filepath.Dir(id.HomeDir))
		}

	case "add":
		if len(args) < 2 {
			fatalf("Usage: airlock identity add <name>\n")
		}
		name := args[1]
		id, err := global.CreateIdentity(name)
		if err != nil {
			fatalf("Failed to create identity %q: %v\n", name, err)
		}
		fmt.Printf("Created identity %q at %s\n", id.Name, filepath.Dir(id.HomeDir))

	case "remove":
		force := false
		name := ""
		for _, a := range args[1:] {
			if a == "--force" {
				force = true
			} else {
				name = a
			}
		}
		if name == "" {
			fatalf("Usage: airlock identity remove <name> [--force]\n")
		}
		if err := global.RemoveIdentity(name, force); err != nil {
			fatalf("Failed to remove identity: %v\n", err)
		}
		fmt.Printf("Removed identity %q.\n", name)

	default:
		fatalf("Unknown identity subcommand: %s\nUsage: airlock identity <list|add|setup|remove>\n", args[0])
	}
}

func runIdentitySetup(name string) {
	identity, err := global.GetIdentity(name)
	if err != nil {
		fatalf("Identity %q not found: %v\n", name, err)
	}
	setupScript := filepath.Join(identity.Dir(), "setup.sh")
	if isEffectivelyEmpty(setupScript) {
		fmt.Printf("setup.sh for identity %q contains only the template (no active commands).\n", name)
		fmt.Printf("Edit %s to wire up your dotfiles, then re-run.\n", setupScript)
		return
	}
	fmt.Printf("Running setup.sh for identity %q...\n", name)
	cmd := exec.Command("bash", setupScript)
	cmd.Env = append(os.Environ(), "IDENTITY_HOME="+identity.HomeDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		fatalf("setup.sh failed: %v\n", err)
	}
	fmt.Println("Done.")
}

// --- list ---

func runList(ctx context.Context) {
	gcfg := mustLoadGlobalConfig()
	eng := mustDetectEngine(gcfg.Engine)
	r := newRunner(eng)

	runningNames, err := r.List(ctx)
	if err != nil {
		fatalf("Failed to list containers: %v\n", err)
	}
	runningSet := make(map[string]bool, len(runningNames))
	for _, n := range runningNames {
		runningSet[n] = true
	}

	idx, err := global.LoadProjectIndex()
	if err != nil {
		fatalf("Failed to load project index: %v\n", err)
	}

	if len(idx.Projects) == 0 {
		fmt.Println("No projects. Run `airlock init` in a project directory.")
		return
	}

	fmt.Printf("%-25s %-35s %-15s %s\n", "NAME", "IMAGE", "IDENTITY", "STATUS")
	for _, p := range idx.SortedPaths() {
		e := idx.Projects[p]
		status := "stopped"
		if runningSet["airlock-"+e.Name] {
			status = "running"
		}
		fmt.Printf("%-25s %-35s %-15s %s\n", e.Name, e.ResolvedImage(), e.Identity, status)
	}
}

// --- down ---

func runDown(ctx context.Context, r *container.Runner, nameArg string) {
	var target string
	if nameArg != "" {
		if strings.HasPrefix(nameArg, "airlock-") {
			target = nameArg
		} else {
			target = "airlock-" + nameArg
		}
	} else {
		cwd, _ := os.Getwd()
		idx, err := global.LoadProjectIndex()
		if err != nil {
			fatalf("Failed to load project index: %v\n", err)
		}
		entry, ok := idx.Find(cwd)
		if !ok {
			fatalf("This directory is not initialized. Run: airlock init\n")
		}
		target = "airlock-" + entry.Name
	}
	if err := r.Down(ctx, target); err != nil {
		fatalf("down error: %v\n", err)
	}
}

// --- helpers ---

func mustLoadGlobalConfig() *global.GlobalConfig {
	gcfg, err := global.LoadConfig()
	if err != nil {
		fatalf("Failed to load global config: %v\n", err)
	}
	return gcfg
}

func mustDetectEngine(preferred string) container.Engine {
	eng, err := container.DetectEngine(preferred)
	if err != nil {
		fatalf("Failed to detect container engine: %v\n", err)
	}
	return eng
}

func mustLoadSpec(gcfg *global.GlobalConfig, eng container.Engine) *container.ContainerSpec {
	cwd, err := os.Getwd()
	if err != nil {
		fatalf("Failed to get current directory: %v\n", err)
	}
	idx, err := global.LoadProjectIndex()
	if err != nil {
		fatalf("Failed to load project index: %v\n", err)
	}
	entry, ok := idx.Find(cwd)
	if !ok {
		fatalf("This directory is not initialized. Run: airlock init\n")
	}
	identity, err := global.GetIdentity(entry.Identity)
	if err != nil {
		fatalf("Identity %q not found: %v\nRe-run `airlock init` to fix the configuration.\n", entry.Identity, err)
	}

	// Resolve on-create.sh — only set if the file actually exists and is non-empty
	onCreateScript := filepath.Join(identity.Dir(), "on-create.sh")
	if isEffectivelyEmpty(onCreateScript) {
		onCreateScript = ""
	}

	return &container.ContainerSpec{
		Name:           entry.Name,
		Image:          entry.Image,
		Containerfile:  entry.Containerfile,
		ImageTag:       entry.ImageTag,
		HomeHost:       identity.HomeDir,
		CacheHost:      identity.CacheDir,
		WorkDirHost:    cwd,
		Identity:       entry.Identity,
		OnCreateScript: onCreateScript,
		Ports:          entry.Ports,
		Engine:         eng,
	}
}

// isEffectivelyEmpty reports whether a file is absent or contains only
// comments and whitespace (i.e. the template has not been edited).
func isEffectivelyEmpty(path string) bool {
	b, err := os.ReadFile(path)
	if err != nil {
		return true
	}
	for _, line := range strings.Split(string(b), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return false
		}
	}
	return true
}

func newRunner(eng container.Engine) *container.Runner {
	r := container.NewRunner(eng)
	r.Verbose = verboseFlag
	return r
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format, args...)
	os.Exit(1)
}

func sanitizeName(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
		case r >= 'A' && r <= 'Z':
			out = append(out, r+('a'-'A'))
		case r >= '0' && r <= '9':
			out = append(out, r)
		default:
			out = append(out, '-')
		}
	}
	return string(out)
}

type stringSlice []string

func (s *stringSlice) String() string         { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error     { *s = append(*s, v); return nil }

// normalizePorts converts bare port numbers like "3000" to "3000:3000".
func normalizePorts(ports []string) []string {
	if len(ports) == 0 {
		return nil
	}
	out := make([]string, len(ports))
	for i, p := range ports {
		if !strings.Contains(p, ":") {
			p = p + ":" + p
		}
		out[i] = p
	}
	return out
}
