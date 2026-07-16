package root

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	processinitializer "github.com/portpowered/infinite-you/pkg/initializer"
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

func TestProductionRootPreservesModelListSuccessAndFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		statusCode int
		body       string
		wantCode   int
		wantOutput string
	}{
		{name: "success", statusCode: http.StatusOK, body: `{"results":[]}`, wantCode: ExitSuccess, wantOutput: `"results":[]`},
		{name: "dependency failure", statusCode: http.StatusServiceUnavailable, body: `{"message":"model catalog unavailable"}`, wantCode: ExitFailure, wantOutput: "model catalog unavailable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/models" {
					t.Fatalf("request path = %q, want /models", r.URL.Path)
				}
				w.WriteHeader(test.statusCode)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()

			var stdout bytes.Buffer
			var stderr bytes.Buffer
			code := Run(Input{
				Args: []string{"you", "--server", server.URL, "--json", "models", "list"},
				Env:  rootTestEnvironment(), Stdout: &stdout, Stderr: &stderr,
			}, Dependencies{})
			if code != test.wantCode {
				t.Fatalf("exit code = %d, want %d; stdout=%q stderr=%q", code, test.wantCode, stdout.String(), stderr.String())
			}
			output := stdout.String() + stderr.String()
			if !strings.Contains(output, test.wantOutput) {
				t.Fatalf("output = %q, want %q", output, test.wantOutput)
			}
		})
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
	err := ExecuteWithDependencies(Input{
		Args: []string{"you", "run", "--dir", dir, "--quiet", "--no-record"},
		Env:  homeEnvironment(t.TempDir()),
	}, Dependencies{Initializer: initializer})

	if !errors.Is(err, initializerErr) {
		t.Fatalf("ExecuteWithDependencies() error = %v, want initializer failure", err)
	}
	if initializer.calls != 1 {
		t.Fatalf("initializer calls = %d, want 1 after completed production construction", initializer.calls)
	}
	if initializer.input.Graph == nil {
		t.Fatal("initializer did not receive the constructed production graph")
	}
	closeUnstartedProductionGraph(t, initializer.input.Graph)
}

func closeUnstartedProductionGraph(t *testing.T, graph *ApplicationGraph) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := processinitializer.RunProcess(ctx, graph); err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("close unstarted production graph: %v", err)
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
				Initializer: initializer,
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
	fixturePath := testutil.MustRepoPath(t, "pkg/transports/http/testdata/durable-session-contract-fixtures.json")
	input := strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"root-test","version":"test"}}}` + "\n" +
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"you.factory_session.list","arguments":{"scope":"persisted"}}}` + "\n",
	)
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
	if !strings.Contains(output.String(), `"id":2`) ||
		!strings.Contains(output.String(), `"isError":false`) ||
		!strings.Contains(output.String(), `dur-sess-js-failed-partial-001`) {
		t.Fatalf("MCP stdout = %q, want successful graph-backed Factory Session list response", output.String())
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
