// Package main is the entry point for the agent-factory CLI.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

const (
	exitSuccess = 0
	exitFailure = 1
)

var runProcess = func() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	workingDirectory, err := os.Getwd()
	if err == nil {
		process, buildErr := root.BuildProcess(ctx, edges.Edges{})
		err = buildErr
		if err == nil {
			stdinIsTTY := streamIsTerminal(os.Stdin)
			stdoutIsTTY := streamIsTerminal(os.Stdout)
			stderrIsTTY := streamIsTerminal(os.Stderr)
			err = process.Execute(root.Input{
				Args: os.Args, Env: os.Environ(), Stdin: os.Stdin, Stdout: os.Stdout,
				Stderr: os.Stderr, Context: ctx, WorkingDirectory: workingDirectory,
				StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY, StderrIsTTY: &stderrIsTTY,
			})
			closeCtx, cancelClose := context.WithTimeout(context.Background(), 5*time.Second)
			err = errors.Join(err, process.Close(closeCtx))
			cancelClose()
		}
	}
	return processExitCode(err, ctx.Err(), os.Args)
}

func processExitCode(err, contextErr error, args []string) int {
	if err == nil {
		err = contextErr
	}
	switch {
	case err == nil:
		return exitSuccess
	case errors.Is(err, context.Canceled):
		return declaredCancellationExitCode(args)
	default:
		return exitFailure
	}
}

func declaredCancellationExitCode(args []string) int {
	commandPath := selectedCommandPath(args)
	if commandPath == "" {
		return exitFailure
	}
	manifests := []func() (climanifest.Manifest, error){
		generated.RunSubmitFamilyManifest,
		generated.WorkerSessionsFamilyManifest,
	}
	for _, loadManifest := range manifests {
		manifest, err := loadManifest()
		if err != nil {
			continue
		}
		for _, command := range manifest.Commands {
			if command.Path != commandPath {
				continue
			}
			for _, exit := range command.Exits {
				if exit.Kind == "cancel" {
					return exit.Code
				}
			}
		}
	}
	return exitFailure
}

func selectedCommandPath(args []string) string {
	commandPaths, flagByName, rootPaths := cancellationCommandMetadata()
	if len(commandPaths) == 0 || len(rootPaths) == 0 {
		return ""
	}

	parts := make([]string, 0, 2)
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			return ""
		}
		if strings.HasPrefix(arg, "-") {
			if !strings.Contains(arg, "=") {
				flag, knownFlag := flagByName[arg]
				if knownFlag && manifestFlagConsumesValue(flag) {
					if index+1 >= len(args) {
						return ""
					}
					index++
				}
			}
			continue
		}

		parts = append(parts, arg)
		matchedPrefix := false
		for rootPath := range rootPaths {
			candidate := strings.TrimSpace(rootPath + " " + strings.Join(parts, " "))
			if canonicalPath, exact := commandPaths[candidate]; exact {
				// Once Cobra has selected a runnable command, later non-flag
				// tokens are command arguments rather than command path parts.
				return canonicalPath
			}
			if cancellationPathHasPrefix(commandPaths, candidate) {
				matchedPrefix = true
				break
			}
		}
		if !matchedPrefix {
			return ""
		}
	}
	return ""
}

func cancellationCommandMetadata() (map[string]string, map[string]climanifest.Flag, map[string]struct{}) {
	commandPaths := make(map[string]string)
	flagByName := make(map[string]climanifest.Flag)
	rootPaths := make(map[string]struct{})
	manifests := []func() (climanifest.Manifest, error){
		generated.RunSubmitFamilyManifest,
		generated.WorkerSessionsFamilyManifest,
	}
	for _, loadManifest := range manifests {
		manifest, err := loadManifest()
		if err != nil {
			continue
		}
		if manifest.RootPath != "" {
			rootPaths[manifest.RootPath] = struct{}{}
		}
		for _, command := range manifest.Commands {
			for _, flag := range command.Flags {
				registerManifestFlag(flagByName, flag)
			}
			if !commandDeclaresCancellation(command) {
				continue
			}
			commandPaths[command.Path] = command.Path
			for _, alias := range command.Aliases {
				parentPath := strings.TrimSuffix(command.Path, " "+command.Name)
				aliasPath := strings.TrimSpace(parentPath + " " + alias)
				if aliasPath != "" {
					commandPaths[aliasPath] = command.Path
				}
			}
		}
	}
	return commandPaths, flagByName, rootPaths
}

func commandDeclaresCancellation(command climanifest.Command) bool {
	for _, exit := range command.Exits {
		if exit.Kind == "cancel" {
			return true
		}
	}
	return false
}

func registerManifestFlag(flags map[string]climanifest.Flag, flag climanifest.Flag) {
	if flag.Long == "" {
		return
	}
	register := func(name string) {
		if current, exists := flags[name]; !exists || (current.ValueType == "bool" && flag.ValueType != "bool") {
			flags[name] = flag
		}
	}
	register("--" + flag.Long)
	if flag.Shorthand != "" {
		register("-" + flag.Shorthand)
	}
	for _, alias := range flag.Aliases {
		register("--" + alias)
	}
}

func manifestFlagConsumesValue(flag climanifest.Flag) bool {
	return flag.ValueType != "bool" && flag.NoOptionDefault == ""
}

func cancellationPathHasPrefix(commandPaths map[string]string, candidate string) bool {
	for commandPath := range commandPaths {
		if strings.HasPrefix(commandPath, candidate+" ") {
			return true
		}
	}
	return false
}

func streamIsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

var exitProcess = os.Exit

func main() {
	exitProcess(runProcess())
}
