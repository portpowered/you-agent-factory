package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

// fakeACPServer records every Serve invocation without performing any real
// protocol I/O, so tests can assert exact stream/context forwarding without
// standing up the production ACP stack.
type fakeACPServer struct {
	mu       sync.Mutex
	calls    int
	gotCtx   context.Context
	gotIn    io.Reader
	gotOut   io.Writer
	serveErr error
	onServe  func(ctx context.Context, in io.Reader, out io.Writer) error
}

func (f *fakeACPServer) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	f.mu.Lock()
	f.calls++
	f.gotCtx, f.gotIn, f.gotOut = ctx, in, out
	f.mu.Unlock()
	if f.onServe != nil {
		return f.onServe(ctx, in, out)
	}
	return f.serveErr
}

func TestServeFamily_VisibleInRootHelpAndDistinctFromWorkersAcpAndMcpServe(t *testing.T) {
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)

	if _, _, err := root.Find([]string{"serve"}); err != nil {
		t.Fatalf("find %q: %v", "you serve", err)
	}
	acpCmd, _, err := root.Find([]string{"serve", "acp"})
	if err != nil {
		t.Fatalf("find %q: %v", "you serve acp", err)
	}
	if !acpCmd.Runnable() {
		t.Fatal("you serve acp must be runnable")
	}

	if found, _, _ := root.Find([]string{"acp", "serve"}); found == acpCmd || (found != root && found.Name() == "acp") {
		t.Fatal("you acp serve must not be a recognized alias")
	}
	for _, sub := range root.Commands() {
		if sub.Name() == "acp" {
			t.Fatal("you acp must not be a top-level command; ACP serving is only reachable as you serve acp")
		}
	}

	workersACP, _, err := root.Find([]string{"workers", "acp"})
	if err != nil {
		t.Fatalf("find %q: %v", "you workers acp", err)
	}
	if workersACP == acpCmd {
		t.Fatal("you workers acp must remain a distinct command from you serve acp")
	}

	mcpServe, _, err := root.Find([]string{"mcp", "serve"})
	if err != nil {
		t.Fatalf("find %q: %v", "you mcp serve", err)
	}
	if mcpServe == acpCmd {
		t.Fatal("you mcp serve must remain a distinct command from you serve acp")
	}
}

func TestServeACPCommand_HelpDescribesStdioProtocolAndStderrDiagnostics(t *testing.T) {
	manifest, err := generated.ServeFamilyManifest()
	if err != nil {
		t.Fatalf("ServeFamilyManifest() error = %v", err)
	}
	record, err := manifest.CommandByID("you.serve.acp")
	if err != nil {
		t.Fatalf("CommandByID(you.serve.acp) error = %v", err)
	}
	if !record.Runnable {
		t.Fatal("you.serve.acp manifest record must be runnable")
	}
	if record.Visibility != "visible" {
		t.Fatalf("visibility = %q, want visible", record.Visibility)
	}

	long := record.Documentation.Documentation.Description.CanonicalEnglish
	for _, want := range []string{"stdin", "stdout", "stderr", "JSON-RPC"} {
		if !strings.Contains(long, want) {
			t.Fatalf("you serve acp help text missing %q:\n%s", want, long)
		}
	}
}

func TestServeACPCommand_DispatchesToInjectedACPServerWithExactStreamsAndContext(t *testing.T) {
	fake := &fakeACPServer{}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	root := factory.NewCommand(nil, nil, nil)
	stdin := strings.NewReader("acp protocol input")
	var stdout bytes.Buffer
	root.SetIn(stdin)
	root.SetOut(&stdout)
	root.SetErr(io.Discard)

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	root.SetContext(ctx)
	root.SetArgs([]string{"serve", "acp"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if fake.calls != 1 {
		t.Fatalf("Serve call count = %d, want 1", fake.calls)
	}
	if fake.gotIn != stdin {
		t.Fatal("Serve did not receive the exact invocation stdin reader")
	}
	if fake.gotOut != &stdout {
		t.Fatal("Serve did not receive the exact invocation stdout writer")
	}
	if fake.gotCtx == nil || fake.gotCtx.Value(ctxKey{}) != "marker" {
		t.Fatal("Serve did not receive the invocation's process context")
	}
}

func TestServeACPCommand_CleanEOFSucceeds(t *testing.T) {
	fake := &fakeACPServer{serveErr: nil}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	root := factory.NewCommand(nil, nil, nil)
	root.SetIn(strings.NewReader(""))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"serve", "acp"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v, want nil on clean EOF", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must contain only ACP protocol frames, got %q", stdout.String())
	}
}

func TestServeACPCommand_CancellationPropagatesFromProcessContext(t *testing.T) {
	fake := &fakeACPServer{}
	fake.onServe = func(ctx context.Context, _ io.Reader, _ io.Writer) error {
		<-ctx.Done()
		return ctx.Err()
	}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	root := factory.NewCommand(nil, nil, nil)
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	root.SetContext(ctx)
	root.SetArgs([]string{"serve", "acp"})
	cancel()

	if err := root.Execute(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestServeACPCommand_StartupFailureReportsSanitizedDiagnosticOnStderr(t *testing.T) {
	fake := &fakeACPServer{serveErr: errors.New("acp: stdio input ended with a partial trailing frame")}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	root := factory.NewCommand(nil, nil, nil)
	root.SetIn(strings.NewReader(""))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"serve", "acp"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want a non-nil serve failure")
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure must not contaminate stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "partial trailing frame") {
		t.Fatalf("stderr = %q, want it to carry the serve failure diagnostic", stderr.String())
	}
}

func TestServeACPCommand_MissingACPServerFailsWithoutPanicking(t *testing.T) {
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})

	root := factory.NewCommand(nil, nil, nil)
	root.SetIn(strings.NewReader(""))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"serve", "acp"})

	if err := root.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want a failure when no ACP server is injected")
	}
	if stdout.Len() != 0 {
		t.Fatalf("failure must not contaminate stdout, got %q", stdout.String())
	}
}
