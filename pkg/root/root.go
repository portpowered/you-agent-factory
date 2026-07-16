package root

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli"
	startupcli "github.com/portpowered/infinite-you/pkg/transports/cli/startup"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/spf13/cobra"
)

// NewCommand constructs a fresh compatible command tree for one normalized
// process execution.
func NewCommand(input ProcessInput) *cobra.Command {
	return NewCommandWithDependencies(input, Dependencies{})
}

// NewCommandWithDependencies constructs a command using explicit process
// construction and lifecycle boundaries.
func NewCommandWithDependencies(input ProcessInput, dependencies Dependencies) *cobra.Command {
	core := wire.InjectWireCore()
	resolved := dependenciesFromWireCore(core, dependencies)
	command := resolved.core.BuildCLICommand(cli.RootCommandOptions{
		HomeDir:   func() (string, error) { return homeDir(input) },
		LookupEnv: input.LookupEnv,
		Startup: func(ctx context.Context, request startupcli.Request) error {
			return executeStartup(ctx, request, resolved)
		},
		BuildSessionExecution: resolved.core.BuildSessionExecution,
		BuildModelInvocation:  resolved.core.BuildModelInvocation,
	})
	command.SetArgs(input.Arguments())
	command.SetIn(input.stdin)
	command.SetOut(input.stdout)
	command.SetErr(input.stderr)
	command.SetContext(input.context)
	return command
}

// Execute normalizes input and executes a fresh command instance.
func Execute(input Input) error {
	normalized, err := Normalize(input)
	if err != nil {
		return err
	}
	return NewCommand(normalized).Execute()
}

// ExecuteWithDependencies executes one process with explicit graph and
// initializer boundaries. It is the deterministic process-level test seam.
func ExecuteWithDependencies(input Input, dependencies Dependencies) error {
	normalized, err := Normalize(input)
	if err != nil {
		return err
	}
	return NewCommandWithDependencies(normalized, dependencies).Execute()
}

func homeDir(input ProcessInput) (string, error) {
	var home string
	switch runtime.GOOS {
	case "windows":
		home, _ = input.LookupEnv("USERPROFILE")
		if home == "" {
			drive, _ := input.LookupEnv("HOMEDRIVE")
			path, _ := input.LookupEnv("HOMEPATH")
			home = drive + path
		}
	case "plan9":
		home, _ = input.LookupEnv("home")
	default:
		home, _ = input.LookupEnv("HOME")
	}
	if strings.TrimSpace(home) != "" {
		return home, nil
	}
	return "", fmt.Errorf("home directory is not defined in the supplied environment")
}
