package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
)

const (
	golangciTool = "github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8"
	configPath   = ".golangci.pkg.yml"
	packageScope = "./pkg/..."
)

var (
	commandMain                 = run
	runPkgLintCommand           = runPkgLint
	execCommand                 = exec.Command
	exitFunc                    = os.Exit
	stdout            io.Writer = os.Stdout
	stderr            io.Writer = os.Stderr
)

func main() {
	exitFunc(commandMain(os.Args[1:], stdout, stderr))
}

func run(_ []string, stdout io.Writer, stderr io.Writer) int {
	if err := runPkgLintCommand(); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}

	fmt.Fprintln(stdout, "[agent-factory:pkg-lint] pkg lint passed")
	return 0
}

func runPkgLint() error {
	cmd := execCommand("go", "run", golangciTool, "run", "--config", configPath, packageScope)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run pkg lint: %w\n%s%s", err, stdout.String(), stderr.String())
	}
	if stdout.Len() > 0 {
		_, _ = os.Stdout.Write(stdout.Bytes())
	}
	if stderr.Len() > 0 {
		_, _ = os.Stderr.Write(stderr.Bytes())
	}
	return nil
}
