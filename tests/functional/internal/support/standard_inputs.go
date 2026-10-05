package support

import (
	"bytes"
	"context"
	"os"
	"path/filepath"

	"github.com/portpowered/infinite-you/pkg/root"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
)

// CapturedInputs owns the streams for one Process.Execute invocation.
type CapturedInputs struct {
	root.Input

	stdout bytes.Buffer
	stderr bytes.Buffer
}

// IsolatedHomeEnvironment keeps customer commands inside a fixture-owned home,
// including model discovery when the operator has configured an external cache.
func IsolatedHomeEnvironment(home string) []string {
	environment := withFunctionalEnvironment(os.Environ(), "HOME", home)
	environment = withFunctionalEnvironment(environment, "USERPROFILE", home)
	return withFunctionalEnvironment(environment, runcli.ModelCacheDirEnvironment, filepath.Join(home, ".agent-factory", "models"))
}

func FakeInputs(ctx context.Context, args []string) *CapturedInputs {
	inputs := &CapturedInputs{}
	workingDirectory, _ := os.Getwd()
	stdinIsTTY := testStreamIsTerminal(os.Stdin)
	stdoutIsTTY := false
	inputs.Input = root.Input{
		Args: args, Env: os.Environ(), Stdin: os.Stdin, Stdout: &inputs.stdout,
		Stderr: &inputs.stderr, Context: ctx, WorkingDirectory: workingDirectory,
		StdinIsTTY: &stdinIsTTY, StdoutIsTTY: &stdoutIsTTY,
	}
	return inputs
}

func testStreamIsTerminal(file *os.File) bool {
	if file == nil {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// Stdout returns the text written to the process standard-output stream.
func (inputs *CapturedInputs) Stdout() string {
	if inputs == nil {
		return ""
	}
	return inputs.stdout.String()
}

// Stderr returns the text written to the process diagnostic stream.
func (inputs *CapturedInputs) Stderr() string {
	if inputs == nil {
		return ""
	}
	return inputs.stderr.String()
}
