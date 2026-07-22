// Package main is the entry point for the agent-factory CLI.
package main

import (
	"context"
	"os"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
)

var runProcess = func() int {
	ctx := context.Background()
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
	if err != nil {
		return 1
	}
	return 0
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
