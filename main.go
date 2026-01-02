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
  airlock [--config path] <command> [args]

Commands:
  init       Create airlock.yaml and .airlock/airlock.local.yaml (if missing) + ensure .airlock dirs + .gitignore entry
  up         Build (if needed) and create the airlock container (idempotent)
  enter [-e var] Enter the airlock container (interactive shell)
  exec [-e var]  Execute a command inside the airlock container
  down           Stop and remove the airlock container (keeps .airlock state dirs)
  info           Print detected engine, paths, and config
  help           Print this help message
  version        Print version

Examples:
  airlock init
  airlock up
  airlock enter -e ANTHROPIC_API_KEY
  airlock exec -e SOME_VAR -- git status
  airlock down

Flags:
`, version)
	flag.PrintDefaults()
}

type stringSlice []string

func (s *stringSlice) String() string {
	return fmt.Sprint(*s)
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	var configPath string
	flag.StringVar(&configPath, "config", "", "Path to airlock.yaml (default: ./airlock.yaml or ./airlock.yml)")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(2)
	}
	cmd := args[0]
	cmdArgs := args[1:]

	// Define command-specific flags
	enterFlags := flag.NewFlagSet("enter", flag.ExitOnError)
	var enterEnv stringSlice
	enterFlags.Var(&enterEnv, "e", "Forward ambient environment variable into the container")

	execFlags := flag.NewFlagSet("exec", flag.ExitOnError)
	var execEnv stringSlice
	execFlags.Var(&execEnv, "e", "Forward ambient environment variable into the container")

	ctx := context.Background()

	if cmd == "help" {
		usage()
		return
	}

	if cmd == "version" {
		fmt.Println(version)
		return
	}

	if cmd == "init" {
		if err := config.InitFiles("."); err != nil {
			fmt.Fprintf(os.Stderr, "init error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Created airlock.yaml and .airlock/airlock.local.yaml (if missing), ensured .airlock dirs, and updated .gitignore.")
		return
	}

	cfgFile := configPath
	if cfgFile == "" {
		for _, cand := range []string{"airlock.yaml", "airlock.yml"} {
			if _, err := os.Stat(cand); err == nil {
				cfgFile = cand
				break
			}
		}
	}
	if cfgFile == "" {
		fmt.Fprintln(os.Stderr, "No airlock.yaml found. Run: airlock init")
		os.Exit(1)
	}

	cfg, err := config.Load(cfgFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	absProj, _ := filepath.Abs(cfg.ProjectDir)

	eng, err := container.DetectEngine(cfg.Engine.Preferred)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to detect container engine: %v\n", err)
		os.Exit(1)
	}

	runner := container.NewRunner(eng)

	switch cmd {
	case "info":
		info, err := runner.Info(ctx, cfg, absProj)
		if err != nil {
			fmt.Fprintf(os.Stderr, "info error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(info)
		return

	case "up":
		if err := runner.Up(ctx, cfg, absProj); err != nil {
			fmt.Fprintf(os.Stderr, "up error: %v\n", err)
			os.Exit(1)
		}
		return

	case "enter":
		enterFlags.Parse(cmdArgs)
		if err := runner.Up(ctx, cfg, absProj); err != nil {
			fmt.Fprintf(os.Stderr, "up error: %v\n", err)
			os.Exit(1)
		}
		if err := runner.Enter(ctx, cfg, absProj, enterEnv); err != nil {
			fmt.Fprintf(os.Stderr, "enter error: %v\n", err)
			os.Exit(1)
		}
		return

	case "exec":
		execFlags.Parse(cmdArgs)
		cmdArgs = execFlags.Args()
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
		if err := runner.Exec(ctx, cfg, absProj, execEnv, cmdArgs); err != nil {
			fmt.Fprintf(os.Stderr, "exec error: %v\n", err)
			os.Exit(1)
		}
		return

	case "down":
		if err := runner.Down(ctx, cfg, absProj); err != nil {
			fmt.Fprintf(os.Stderr, "down error: %v\n", err)
			os.Exit(1)
		}
		return

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
