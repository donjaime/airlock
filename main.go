package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/donjaime/airlock/internal/config"
	"github.com/donjaime/airlock/internal/container"
)

const version = "0.5.0"

func usage() {
	fmt.Fprintf(os.Stderr, `airlock v%s

Usage:
  airlock [--config path] [-e var] [-v] <command> [args]

Commands:
  init [name]  Create airlock.yaml, Containerfile, and .airlock/airlock.local.yaml (if missing) + ensure .airlock dirs + .gitignore entry
  up         Build (if needed) and create the airlock container (idempotent)
  enter      Enter the airlock container (interactive shell)
  exec       Execute a command inside the airlock container
  down [name]    Stop and remove the airlock container (keeps .airlock state dirs)
  list           List all running airlock containers
  info           Print detected engine, paths, and config
  help           Print this help message
  version        Print version

Examples:
  airlock init
  airlock up
  airlock -e ANTHROPIC_API_KEY enter
  airlock -e SOME_VAR exec -- git status
  airlock down [container-name]
  airlock list

Flags:
`, version)
	flag.PrintDefaults()
}

var (
	configPath = flag.String("config", "", "Path to airlock.yaml (default: ./airlock.yaml or ./airlock.yml)")
	verbose    = flag.Bool("v", false, "Enable verbose output (print underlying podman/docker commands)")
	envVars    = stringSliceFlag("e", "Forward ambient environment variable into the container")
)

func init() {
	flag.Usage = usage
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
		name := ""
		if len(cmdArgs) > 0 {
			name = cmdArgs[0]
		}
		if err := config.InitFiles(".", name); err != nil {
			fmt.Fprintf(os.Stderr, "init error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created airlock.yaml, Containerfile, and .airlock/airlock.local.yaml (if missing), ensured .airlock dirs, and updated .gitignore.")

	case "list", "down", "info", "up", "enter", "exec":
		cfg, _, err := loadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v. Run: airlock init\n", err)
			os.Exit(1)
		}

		absProj, _ := filepath.Abs(cfg.ProjectDir)
		eng, err := container.DetectEngine(cfg.Engine)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Failed to detect container engine: %v\n", err)
			os.Exit(1)
		}

		runner := container.NewRunner(eng)
		runner.Verbose = *verbose

		switch cmd {
		case "list":
			names, err := runner.List(ctx)
			if err != nil {
				fmt.Fprintf(os.Stderr, "list error: %v\n", err)
				os.Exit(1)
			}
			for _, name := range names {
				fmt.Println(name)
			}

		case "down":
			var target string
			if len(cmdArgs) > 0 {
				target = cmdArgs[0]
			}
			if err := runner.Down(ctx, cfg, target); err != nil {
				fmt.Fprintf(os.Stderr, "down error: %v\n", err)
				os.Exit(1)
			}

		case "info":
			info, err := runner.Info(ctx, cfg, absProj)
			if err != nil {
				fmt.Fprintf(os.Stderr, "info error: %v\n", err)
				os.Exit(1)
			}
			fmt.Println(info)

		case "up":
			if err := runner.Up(ctx, cfg, absProj); err != nil {
				fmt.Fprintf(os.Stderr, "up error: %v\n", err)
				os.Exit(1)
			}

		case "enter":
			if err := runner.Enter(ctx, cfg, absProj, envVars); err != nil {
				fmt.Fprintf(os.Stderr, "enter error: %v\n", err)
				os.Exit(1)
			}

		case "exec":
			if len(cmdArgs) == 0 {
				fmt.Fprintln(os.Stderr, "exec requires a command, e.g. airlock exec -- ls -la")
				os.Exit(2)
			}
			if cmdArgs[0] == "--" {
				cmdArgs = cmdArgs[1:]
			}
			if err := runner.Up(ctx, cfg, absProj); err != nil {
				fmt.Fprintf(os.Stderr, "up error: %v\n", err)
				os.Exit(1)
			}
			if err := runner.Exec(ctx, cfg, absProj, envVars, cmdArgs); err != nil {
				fmt.Fprintf(os.Stderr, "exec error: %v\n", err)
				os.Exit(1)
			}
		}

	default:
		if strings.HasPrefix(cmd, "-") {
			usage()
			os.Exit(2)
		}
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", cmd)
		usage()
		os.Exit(2)
	}
}

type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprint(*s)
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

// Helper function to allow one-line assignment
func stringSliceFlag(name string, usage string) []string {
	var s stringSlice
	flag.Var(&s, name, usage)
	return s
}

func loadConfig(path string) (*config.Config, string, error) {
	cfgFile := path
	if cfgFile == "" {
		for _, cand := range []string{"airlock.yaml", "airlock.yml"} {
			if _, err := os.Stat(cand); err == nil {
				cfgFile = cand
				break
			}
		}
	}
	if cfgFile == "" {
		return nil, "", fmt.Errorf("no airlock.yaml found")
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		return nil, "", err
	}
	return cfg, cfgFile, nil
}
