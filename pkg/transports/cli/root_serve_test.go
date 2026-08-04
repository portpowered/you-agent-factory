package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	acpwire "github.com/portpowered/infinite-you/pkg/transports/acp/wire"
	"github.com/spf13/pflag"
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

// TestServeACPCommand_HelpRendersManifestExamplesAndNoLocalFlags executes the
// real --help path (not a manifest-text read) so a drift between the
// authoritative manifest and the projected runtime command tree -- for
// example a missing Examples section, the way a hand-built cobra.Command
// that only copies Use/Short/Long would silently produce -- fails this test.
//
// you.serve.acp declares no local manifest inputs of its own, so its only
// local flag is Cobra's built-in --help. It does inherit the root's four
// persistent you.flag.{debug,json,server,verbose} records -- exactly like
// every other command family (see you.mcp/you.mcp.serve in
// contracts/cli/commands.json) -- so those, and only those, may appear as
// Global Flags in the real rendered output; this asserts the complete
// rendered flag surface (local and inherited) instead of only local flags,
// so an undeclared or unexpected inherited flag would fail this test too.
func TestServeACPCommand_HelpRendersManifestExamplesAndNoLocalFlags(t *testing.T) {
	var stdout bytes.Buffer
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"serve", "acp", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute serve acp --help: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"stdin", "stdout", "stderr", "JSON-RPC",
		"Examples:",
		"you serve acp",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("you serve acp --help missing %q:\n%s", want, got)
		}
	}

	acpCmd, _, err := root.Find([]string{"serve", "acp"})
	if err != nil {
		t.Fatalf("find %q: %v", "you serve acp", err)
	}
	acpCmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name != "help" {
			t.Fatalf("you serve acp declares no manifest inputs but has local flag %q", flag.Name)
		}
	})

	wantInherited := map[string]bool{"debug": false, "json": false, "server": false, "verbose": false}
	acpCmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if _, ok := wantInherited[flag.Name]; !ok {
			t.Fatalf("you serve acp advertises unrelated inherited flag %q", flag.Name)
		}
		wantInherited[flag.Name] = true
	})
	for name, seen := range wantInherited {
		if !seen {
			t.Fatalf("you serve acp is missing the standard inherited flag %q", name)
		}
	}
	if !strings.Contains(got, "Global Flags:") {
		t.Fatalf("you serve acp --help missing rendered Global Flags section:\n%s", got)
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

// readStartSignal wraps an io.ReadCloser and closes started the instant its
// first Read call begins, before delegating to the real, potentially
// blocking Read. bufio.Scanner.Scan calls Read exactly this way to fill its
// buffer, so a caller that waits on started observes -- deterministically,
// without guessing a duration -- that the scanner has actually reached its
// blocking read rather than merely assuming enough wall-clock time has
// passed.
type readStartSignal struct {
	rc      io.ReadCloser
	once    sync.Once
	started chan struct{}
}

func newReadStartSignal(rc io.ReadCloser) *readStartSignal {
	return &readStartSignal{rc: rc, started: make(chan struct{})}
}

func (r *readStartSignal) Read(p []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	return r.rc.Read(p)
}

func (r *readStartSignal) Close() error {
	return r.rc.Close()
}

// TestServeACPCommand_CancellationClosesStdinToUnblockRealServerMidRead
// proves the production regression this story's review caught: the real
// stdio.Server's Serve only checks ctx.Err() between reads (see
// pkg/transports/acp/internal/stdio/server_test.go's
// TestServeReturnsContextErrorOnMidReadCancellation, which requires its own
// caller-side goroutine to close the pipe on cancellation), so a command
// that merely forwards a cancellable context without also closing stdin
// would hang forever once the read is already blocked. This test exercises
// the real production wire.NewServer implementation (not a fake) with stdin
// left open and never written to, and asserts cancellation unblocks it
// within a small bounded time instead of hanging.
func TestServeACPCommand_CancellationClosesStdinToUnblockRealServerMidRead(t *testing.T) {
	server := acpwire.NewServer(nil, nil, nil, nil, nil)
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = server

	stdinRead, stdinWrite, err := os.Pipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	t.Cleanup(func() {
		_ = stdinRead.Close()
		_ = stdinWrite.Close()
	})
	signalingStdin := newReadStartSignal(stdinRead)

	root := factory.NewCommand(nil, nil, nil)
	root.SetIn(signalingStdin)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	root.SetContext(ctx)
	root.SetArgs([]string{"serve", "acp"})

	done := make(chan error, 1)
	go func() { done <- root.Execute() }()

	// Wait for the deterministic read-start signal instead of guessing how
	// long Execute needs to reach the blocking read, so this test actually
	// exercises a mid-read cancellation rather than a pre-cancelled context
	// short-circuit. The surrounding timeout is only a hang guard against a
	// genuine regression (the scanner never even attempting a read), not a
	// substitute for the deterministic signal itself.
	select {
	case <-signalingStdin.started:
	case <-time.After(2 * time.Second):
		t.Fatal("real server did not begin reading stdin before timeout")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute() did not return after cancellation while stdin remained open and idle")
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

// TestServeACPCommand_ServeFailuresAreSanitizedOnStderr proves every
// non-cancellation Serve failure category is replaced with a fixed,
// payload-free stderr diagnostic: since a Serve error can originate from
// arbitrary decoded request content, nothing about its own message text is
// safe to print verbatim. Each case injects a distinct sensitive sentinel a
// real failure of that category could plausibly carry and asserts it never
// reaches stdout or stderr.
func TestServeACPCommand_ServeFailuresAreSanitizedOnStderr(t *testing.T) {
	cases := []struct {
		name     string
		sentinel string
	}{
		{"startup", "startup failure referencing provider command --api-key sk-live-SENTINEL-1"},
		{"framing", "framing failure decoding prompt content: SENTINEL-PROMPT-please ignore prior instructions"},
		{"reader", "reader failure at unsafe path /home/operator/.ssh/SENTINEL-id_rsa"},
		{"writer", "writer failure writing private topology host SENTINEL-internal-dispatch.local"},
		{"unknown", "unclassified failure SENTINEL-credential=hunter2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeACPServer{serveErr: errors.New(tc.sentinel)}
			factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
			factory.acpServer = fake

			root := factory.NewCommand(nil, nil, nil)
			root.SetIn(strings.NewReader(""))
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"serve", "acp"})

			err := root.Execute()
			if err == nil {
				t.Fatal("Execute() error = nil, want a non-nil serve failure")
			}
			if stdout.Len() != 0 {
				t.Fatalf("failure must not contaminate stdout, got %q", stdout.String())
			}
			if strings.Contains(err.Error(), "SENTINEL") {
				t.Fatalf("returned error leaked the sensitive sentinel: %v", err)
			}
			if strings.Contains(stderr.String(), "SENTINEL") {
				t.Fatalf("stderr leaked the sensitive sentinel: %q", stderr.String())
			}
			if stderr.Len() == 0 {
				t.Fatal("stderr must still carry a bounded, actionable diagnostic")
			}
		})
	}
}
