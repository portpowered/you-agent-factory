package application

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Input contains the invocation-local process values supplied to Execute.
// Args includes the executable name as its first element, matching os.Args.
type Input struct {
	Args    []string
	Env     []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Context context.Context
	// WorkingDirectory is the invocation-local project root observed by the
	// outer process edge. It is required; Initializer performs no host lookup.
	WorkingDirectory string
	// StdinIsTTY and StdoutIsTTY carry terminal classification observed by the
	// outer process edge. Nil means false and never triggers stream inspection.
	StdinIsTTY  *bool
	StdoutIsTTY *bool
}

// processInput is an immutable normalized snapshot of one process invocation.
type processInput struct {
	executable  string
	arguments   []string
	environment map[string]string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	context     context.Context
	workingDir  string
	stdinIsTTY  bool
	stdoutIsTTY bool
}

func normalize(input Input) (processInput, error) {
	if len(input.Args) == 0 {
		return processInput{}, fmt.Errorf("normalize process input: executable argument is required")
	}

	environment := make(map[string]string, len(input.Env))
	for _, entry := range input.Env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			return processInput{}, fmt.Errorf("normalize process input: invalid environment entry %q", entry)
		}
		environment[name] = value
	}

	stdin := input.Stdin
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	stdout := input.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := input.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	ctx := input.Context
	if ctx == nil {
		ctx = context.Background()
	}
	workingDir := strings.TrimSpace(input.WorkingDirectory)
	if workingDir == "" {
		return processInput{}, fmt.Errorf("normalize process input: working directory is required")
	}

	return processInput{
		executable:  input.Args[0],
		arguments:   append([]string(nil), input.Args[1:]...),
		environment: environment,
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
		context:     ctx,
		workingDir:  workingDir,
		stdinIsTTY:  terminalClassification(input.StdinIsTTY),
		stdoutIsTTY: terminalClassification(input.StdoutIsTTY),
	}, nil
}

func terminalClassification(classification *bool) bool {
	if classification != nil {
		return *classification
	}
	return false
}

// Executable returns the invocation's executable name.
func (input processInput) executableName() string { return input.executable }

// Arguments returns a copy of the command arguments, excluding the executable.
func (input processInput) argumentsCopy() []string {
	return append([]string(nil), input.arguments...)
}

// LookupEnv returns a supplied environment value and whether it was present.
func (input processInput) lookupEnv(name string) (string, bool) {
	value, ok := input.environment[name]
	return value, ok
}

func homeDir(input processInput) (string, error) {
	userProfile, _ := input.lookupEnv("USERPROFILE")
	homeDrive, _ := input.lookupEnv("HOMEDRIVE")
	homePath, _ := input.lookupEnv("HOMEPATH")
	unixHome, _ := input.lookupEnv("HOME")
	plan9Home, _ := input.lookupEnv("home")
	for _, home := range []string{userProfile, homeDrive + homePath, unixHome, plan9Home} {
		if strings.TrimSpace(home) != "" {
			return home, nil
		}
	}
	return "", fmt.Errorf("home directory is not defined in the supplied environment")
}
