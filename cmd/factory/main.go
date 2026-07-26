// Package main is the entry point for the agent-factory CLI.
package main

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"strings"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
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
			err = process.Execute(root.Input{
				Args: os.Args, Env: os.Environ(), Stdin: os.Stdin, Stdout: os.Stdout,
				Stderr: os.Stderr, Context: ctx, WorkingDirectory: workingDirectory,
				StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
			})
		}
	}
	return processExitCode(err, os.Args)
}

func processExitCode(err error, args []string) int {
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
	commandName := selectedCommandName(args)
	if commandName == "" {
		return exitFailure
	}
	manifest, err := generated.RunSubmitFamilyManifest()
	if err != nil {
		return exitFailure
	}
	for _, command := range manifest.Commands {
		if command.Name != commandName || command.Path != manifest.RootPath+" "+commandName {
			continue
		}
		for _, exit := range command.Exits {
			if exit.Kind == "cancel" {
				return exit.Code
			}
		}
	}
	return exitFailure
}

func selectedCommandName(args []string) string {
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if strings.HasPrefix(arg, "--") {
			if !strings.Contains(arg, "=") && globalFlagConsumesValue(arg) {
				index++
			}
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg
	}
	return ""
}

func globalFlagConsumesValue(arg string) bool {
	switch arg {
	case "--server", "--default-worker-model-provider", "--default-worker-model":
		return true
	default:
		return false
	}
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
