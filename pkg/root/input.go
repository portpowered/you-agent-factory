// Package root owns process-level input normalization and top-level command
// execution for the you binary.
package root

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// Input contains the process values supplied by an entrypoint. Args includes
// the executable name as its first element, matching os.Args.
type Input struct {
	Args    []string
	Env     []string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Context context.Context
}

// ProcessInput is an immutable normalized snapshot of one process execution.
// Slice and map state is kept private; accessors return copies or scalar values.
type ProcessInput struct {
	executable  string
	arguments   []string
	environment map[string]string
	stdin       io.Reader
	stdout      io.Writer
	stderr      io.Writer
	context     context.Context
}

// Normalize validates and snapshots explicit process input.
func Normalize(input Input) (ProcessInput, error) {
	if len(input.Args) == 0 {
		return ProcessInput{}, fmt.Errorf("normalize process input: executable argument is required")
	}

	environment := make(map[string]string, len(input.Env))
	for _, entry := range input.Env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			return ProcessInput{}, fmt.Errorf("normalize process input: invalid environment entry %q", entry)
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

	return ProcessInput{
		executable:  input.Args[0],
		arguments:   append([]string(nil), input.Args[1:]...),
		environment: environment,
		stdin:       stdin,
		stdout:      stdout,
		stderr:      stderr,
		context:     ctx,
	}, nil
}

// Executable returns the invocation's executable name.
func (input ProcessInput) Executable() string { return input.executable }

// Arguments returns a copy of the command arguments, excluding the executable.
func (input ProcessInput) Arguments() []string {
	return append([]string(nil), input.arguments...)
}

// LookupEnv returns a supplied environment value and whether it was present.
// An explicitly empty value therefore remains distinct from an absent value.
func (input ProcessInput) LookupEnv(name string) (string, bool) {
	value, ok := input.environment[name]
	return value, ok
}
