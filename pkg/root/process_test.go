package root

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
)

func TestRunTranslatesSuccessfulAndFailingProcessOutcomes(t *testing.T) {
	t.Parallel()
	environment := rootTestEnvironment()

	var help bytes.Buffer
	if code := Run(Input{
		Args: []string{"you", "--help"}, Env: environment, Stdout: &help,
	}, Dependencies{}); code != ExitSuccess {
		t.Fatalf("help exit code = %d, want %d", code, ExitSuccess)
	}
	if !strings.Contains(help.String(), "Usage:") {
		t.Fatalf("help output = %q, want usage", help.String())
	}

	var diagnostics bytes.Buffer
	if code := Run(Input{
		Args: []string{"you", "unknown-command"}, Env: environment, Stderr: &diagnostics,
	}, Dependencies{}); code != ExitFailure {
		t.Fatalf("invalid command exit code = %d, want %d", code, ExitFailure)
	}
	if count := strings.Count(diagnostics.String(), `unknown command "unknown-command"`); count != 1 {
		t.Fatalf("invalid command diagnostic count = %d, want 1; stderr = %q", count, diagnostics.String())
	}
}

func TestRunPreservesConstructionInitializerAndCancellationFailures(t *testing.T) {
	t.Parallel()
	environment := rootTestEnvironment()

	constructionErr := errors.New("construction failed")
	if code := Run(Input{
		Args: []string{"you", "run", "--dir", "."}, Env: environment,
	}, Dependencies{GraphBuilder: &recordingGraphBuilder{err: constructionErr}}); code != ExitFailure {
		t.Fatalf("construction failure exit code = %d, want %d", code, ExitFailure)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	initializer := &recordingInitializer{err: ctx.Err()}
	builder := &recordingGraphBuilder{graph: &ApplicationGraph{}}
	if code := Run(Input{
		Args: []string{"you", "run", "--dir", "."}, Env: environment, Context: ctx,
	}, Dependencies{GraphBuilder: builder, Initializer: initializer}); code != ExitFailure {
		t.Fatalf("cancellation exit code = %d, want %d", code, ExitFailure)
	}
	if builder.calls != 1 || initializer.calls != 1 {
		t.Fatalf("builder/initializer calls = %d/%d, want 1/1", builder.calls, initializer.calls)
	}
}

func TestProductionRunGraphCompletesConstructionBeforeInitializerFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())

	initializerErr := errors.New("initializer failed after construction")
	initializer := &recordingInitializer{err: initializerErr}
	code := Run(Input{
		Args: []string{"you", "run", "--dir", dir, "--quiet", "--no-record"},
		Env:  rootTestEnvironment(),
	}, Dependencies{GraphBuilder: productionGraphBuilder{}, Initializer: initializer})

	if code != ExitFailure {
		t.Fatalf("exit code = %d, want %d", code, ExitFailure)
	}
	if initializer.calls != 1 {
		t.Fatalf("initializer calls = %d, want 1 after completed production construction", initializer.calls)
	}
	if initializer.input.Graph == nil {
		t.Fatal("initializer did not receive the constructed production graph")
	}
}

func TestProductionGraphConstructionFailuresPreventInitializerStartup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{name: "run", args: []string{"you", "run", "--dir", t.TempDir(), "--quiet", "--no-record"}},
		{name: "MCP", args: []string{"you", "mcp", "serve", "--fixture-catalog", filepath.Join(t.TempDir(), "missing.json")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initializer := &recordingInitializer{}
			code := Run(Input{Args: test.args, Env: rootTestEnvironment()}, Dependencies{
				GraphBuilder: productionGraphBuilder{}, Initializer: initializer,
			})
			if code != ExitFailure {
				t.Fatalf("exit code = %d, want %d", code, ExitFailure)
			}
			if initializer.calls != 0 {
				t.Fatalf("initializer calls = %d, want 0 after production construction failure", initializer.calls)
			}
		})
	}
}

func TestProductionMCPGraphUsesSuppliedProcessStreams(t *testing.T) {
	t.Parallel()
	fixturePath := testutil.MustRepoPath(t, "pkg/api/testdata/durable-session-contract-fixtures.json")
	input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"root-test","version":"test"}}}` + "\n")
	var output bytes.Buffer
	code := Run(Input{
		Args: []string{"you", "mcp", "serve", "--fixture-catalog", fixturePath},
		Env:  rootTestEnvironment(), Stdin: input, Stdout: &output,
	}, Dependencies{})
	if code != ExitSuccess {
		t.Fatalf("MCP exit code = %d, want %d", code, ExitSuccess)
	}
	if !strings.Contains(output.String(), `"protocolVersion":"2024-11-05"`) {
		t.Fatalf("MCP stdout = %q, want initialize response", output.String())
	}
}

func rootTestEnvironment() []string {
	switch runtime.GOOS {
	case "windows":
		return []string{"USERPROFILE=C:\\tmp"}
	case "plan9":
		return []string{"home=/tmp"}
	default:
		return []string{"HOME=/tmp"}
	}
}
