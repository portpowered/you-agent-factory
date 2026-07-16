package session

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/wire"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func BasicCliInputWithArgs(t *testing.T, args []string) root.Input {
	return root.Input{
		Args:    args,
		Env:     os.Environ(),
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
		Context: t.Context(),
	}
}

func TestRunJavaScriptFactoryDispatchesThroughInjectedProviderCommandRunner(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))

	support.SetWorkingDirectory(t, dir)

	// Act

	runner := support.NewRecordingCommandRunner("<SUCCESS>")
	dependencies := root.Dependencies{FunctionalEdges: wire.FunctionalEdges{
		ProviderCommandRunner: runner,
	}}

	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := BasicCliInputWithArgs(t, []string{"you", "run", "--factory", "./basic.js"})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, dependencies)

	// Assert

	if !strings.Contains(output.String(), "Factory session ") || !strings.Contains(output.String(), " completed (SUCCEEDED).") {
		t.Errorf("expected successful Factory Session output, got: %s", output.String())
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr=%s", exitCode, stderr.String())
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner call count = %d, want 1", runner.CallCount())
	}
}

func TestRunJavaScriptFactoryWithMockWorkersUsesFakeChildExecutor(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	support.SetWorkingDirectory(t, dir)

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	dependencies := root.Dependencies{FunctionalEdges: wire.FunctionalEdges{
		ProviderCommandRunner: runner,
	}}
	var output bytes.Buffer
	var stderr bytes.Buffer
	input := BasicCliInputWithArgs(t, []string{
		"you", "run", "--factory", "./basic.js", "--with-mock-workers",
	})
	input.Stdout = &output
	input.Stderr = &stderr

	exitCode := root.Run(input, dependencies)
	if exitCode != root.ExitSuccess {
		t.Fatalf("exit code = %d, want success; stdout=%q stderr=%q", exitCode, output.String(), stderr.String())
	}
	if !strings.Contains(output.String(), " completed (SUCCEEDED).") {
		t.Fatalf("stdout = %q, want successful Factory Session", output.String())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}
}
