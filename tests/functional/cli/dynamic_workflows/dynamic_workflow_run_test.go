package session

import (
	"bytes"
	"context"
	"os"
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

type functionalEdgeGraphBuilder struct {
	edges wire.FunctionalEdges
}

func (builder functionalEdgeGraphBuilder) Build(
	ctx context.Context,
	request root.GraphRequest,
) (*root.ApplicationGraph, error) {
	return wire.BuildProcessGraphWithFunctionalEdges(ctx, request.Startup, request.Policy, builder.edges)
}

func TestSessionEnumeration(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))

	support.SetWorkingDirectory(t, dir)

	// Act

	dependencies := root.Dependencies{GraphBuilder: functionalEdgeGraphBuilder{edges: wire.FunctionalEdges{
		ProviderCommandRunner: support.NewStaticSuccessCommandRunner("<SUCCESS>"),
	}}}

	// Enumerate the server configs
	output := bytes.Buffer{}
	stderr := bytes.Buffer{}
	fakeEnv := BasicCliInputWithArgs(t, []string{"you", "run", "--factory", "./basic.js"})
	fakeEnv.Stdout = &output
	fakeEnv.Stderr = &stderr
	exitCode := root.Run(fakeEnv, dependencies)

	// Assert

	if !bytes.Contains(output.Bytes(), []byte(dir)) {
		t.Errorf("expected output to contain copied fixture directory %q, got: %s", dir, output.String())
	}

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
}
