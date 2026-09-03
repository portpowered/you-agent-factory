package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	acp "github.com/portpowered/infinite-you/pkg/transports/acp"
	acpwire "github.com/portpowered/infinite-you/pkg/transports/acp/wire"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
	"github.com/spf13/cobra"
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

func noOpServeSystemInitializer() startupcli.Initializer {
	return startupcli.Functions{InitializeSystemFunc: func(context.Context, string) error { return nil }}
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

func TestServerFamily_VisibleInRootHelpAndDistinctFromWorkersAcpAndMCP(t *testing.T) {
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)

	requireRunnableCommand(t, root, "you server", "server")
	acpCmd := requireRunnableCommand(t, root, "you server acp", "server", "acp")
	mcpCmd := requireRunnableCommand(t, root, "you server mcp", "server", "mcp")
	if mcpCmd == acpCmd {
		t.Fatal("you server mcp must remain distinct from you server acp")
	}

	assertRetiredServerPathsAbsent(t, root)
	assertRetiredServerFamiliesAbsent(t, root)

	workersACP := requireCommand(t, root, "you workers acp", "workers", "acp")
	if workersACP == acpCmd {
		t.Fatal("you workers acp must remain a distinct command from you server acp")
	}

	assertServerHelpListsChildren(t, root)
}

func requireCommand(t *testing.T, root *cobra.Command, display string, path ...string) *cobra.Command {
	t.Helper()
	cmd, _, err := root.Find(path)
	if err != nil {
		t.Fatalf("find %q: %v", display, err)
	}
	return cmd
}

func requireRunnableCommand(t *testing.T, root *cobra.Command, display string, path ...string) *cobra.Command {
	t.Helper()
	cmd := requireCommand(t, root, display, path...)
	if !cmd.Runnable() {
		t.Fatalf("%s must be runnable", display)
	}
	return cmd
}

func assertRetiredServerPathsAbsent(t *testing.T, root *cobra.Command) {
	t.Helper()
	for _, retired := range [][]string{{"serve", "acp"}, {"mcp", "serve"}, {"acp", "serve"}} {
		if found, _, findErr := root.Find(retired); findErr == nil && found != root {
			t.Fatalf("retired command path %q must not remain reachable", strings.Join(retired, " "))
		}
	}
}

func assertRetiredServerFamiliesAbsent(t *testing.T, root *cobra.Command) {
	t.Helper()
	for _, sub := range root.Commands() {
		if sub.Name() == "mcp" || sub.Name() == "serve" {
			t.Fatalf("retired top-level command %q must not remain registered", sub.Name())
		}
	}
}

func assertServerHelpListsChildren(t *testing.T, root *cobra.Command) {
	t.Helper()
	var help bytes.Buffer
	root.SetOut(&help)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"server", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute you server --help: %v", err)
	}
	for _, child := range []string{"acp", "mcp"} {
		if !strings.Contains(help.String(), child) {
			t.Fatalf("you server --help missing %q child:\n%s", child, help.String())
		}
	}
}

func TestServerHelpDocumentsContinuousNonResumableHosting(t *testing.T) {
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)
	server := requireCommand(t, root, "you server", "server")

	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"server", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute you server --help: %v", err)
	}

	help := stdout.String()
	for _, want := range []string{
		"continuous, non-resumable hosting",
		"you run --with-server --continuously --record <path>",
	} {
		if !strings.Contains(help, want) {
			t.Fatalf("you server --help missing %q:\n%s", want, help)
		}
	}
	for _, flag := range []string{"record", "resume", "replay"} {
		if server.Flags().Lookup(flag) != nil {
			t.Fatalf("you server unexpectedly exposes --%s", flag)
		}
	}
}

// TestServeACPCommand_HelpRendersManifestExamplesAndNoLocalFlags executes the
// real --help path (not a manifest-text read) so a drift between the
// authoritative manifest and the projected runtime command tree -- for
// example a missing Examples section, the way a hand-built cobra.Command
// that only copies Use/Short/Long would silently produce -- fails this test.
//
// you.server.acp declares no local manifest inputs of its own, so its only
// visible local flag is Cobra's built-in --help. It inherits the root's
// you.flag.{debug,verbose} persistent records like every other command
// family, but --json, --listen, --pprof, --remote, and --server are deliberately suppressed (see
// suppressUnrelatedServeACPFlags in root_serve.go): --json would promise
// structured output on stdout, already reserved for ACP protocol frames,
// --remote selects a running server this command never contacts, and --server
// configures an HTTP endpoint this command never contacts. This
// asserts the complete rendered flag surface (local and inherited) instead
// of only local flags, so an undeclared or unexpected flag -- including a
// resurfaced --json/--listen/--pprof/--remote/--server -- would fail this test too.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestServeACPCommand_HelpRendersManifestExamplesAndNoLocalFlags(t *testing.T) {
	var stdout bytes.Buffer
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(nil, nil, nil)
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"server", "acp", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute server acp --help: %v", err)
	}

	got := stdout.String()
	for _, want := range []string{
		"stdin", "stdout", "stderr", "JSON-RPC",
		"Examples:",
		"you server acp",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("you server acp --help missing %q:\n%s", want, got)
		}
	}
	for _, unwanted := range []string{"--json", "--listen", "--pprof", "--remote", "--server"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("you server acp --help must not advertise unrelated flag %q:\n%s", unwanted, got)
		}
	}

	acpCmd, _, err := root.Find([]string{"server", "acp"})
	if err != nil {
		t.Fatalf("find %q: %v", "you server acp", err)
	}
	wantVisibleLocal := map[string]bool{"help": false}
	acpCmd.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		if flag.Name == "json" || flag.Name == "listen" || flag.Name == "pprof" || flag.Name == "remote" || flag.Name == "server" {
			if !flag.Hidden {
				t.Fatalf("you server acp local flag %q must be hidden", flag.Name)
			}
			return
		}
		if _, ok := wantVisibleLocal[flag.Name]; !ok {
			t.Fatalf("you server acp declares no manifest inputs but has local flag %q", flag.Name)
		}
		wantVisibleLocal[flag.Name] = true
	})
	for name, seen := range wantVisibleLocal {
		if !seen {
			t.Fatalf("you server acp is missing the standard local flag %q", name)
		}
	}

	wantInherited := map[string]bool{"debug": false, "verbose": false}
	acpCmd.InheritedFlags().VisitAll(func(flag *pflag.Flag) {
		if _, ok := wantInherited[flag.Name]; !ok {
			t.Fatalf("you server acp advertises unrelated inherited flag %q", flag.Name)
		}
		wantInherited[flag.Name] = true
	})
	for name, seen := range wantInherited {
		if !seen {
			t.Fatalf("you server acp is missing the standard inherited flag %q", name)
		}
	}
	if !strings.Contains(got, "Global Flags:") {
		t.Fatalf("you server acp --help missing rendered Global Flags section:\n%s", got)
	}
}

// TestServeACPCommand_RejectsUnrelatedGlobalFlags proves --json, --pprof, --remote, and --server
// are not silently accepted and ignored: passing any of them fails the invocation
// with a clear diagnostic before any ACP server is dispatched.
func TestServeACPCommand_RejectsUnrelatedGlobalFlags(t *testing.T) {
	for _, args := range [][]string{
		{"server", "acp", "--json"},
		{"server", "acp", "--pprof"},
		{"server", "acp", "--remote"},
		{"server", "acp", "--server", "http://localhost:9999"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &fakeACPServer{}
			factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
			factory.acpServer = fake

			root := factory.NewCommand(nil, nil, nil)
			var stdout, stderr bytes.Buffer
			root.SetIn(strings.NewReader(""))
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs(args)

			if err := root.Execute(); err == nil {
				t.Fatalf("execute %v: want an error, got success", args)
			}
			if fake.calls != 0 {
				t.Fatalf("ACP server Serve called %d times, want 0 for a rejected unrelated flag", fake.calls)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout must stay empty on a rejected unrelated flag, got %q", stdout.String())
			}
		})
	}
}

func TestServeMCPCommand_RejectsPprof(t *testing.T) {
	root := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI}).NewCommand(
		func() (string, error) { return t.TempDir(), nil }, nil, noOpServeSystemInitializer(),
	)
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"server", "mcp", "--pprof"})

	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "server mcp: --pprof is not supported") {
		t.Fatalf("execute server mcp --pprof error = %v, want protocol-stdio rejection", err)
	}
}

func TestServeACPCommand_DispatchesToInjectedACPServerWithExactStreamsAndContext(t *testing.T) {
	fake := &fakeACPServer{}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	root := factory.NewCommand(func() (string, error) { return "operator-home", nil }, nil, noOpServeSystemInitializer())
	stdin := strings.NewReader("acp protocol input")
	var stdout bytes.Buffer
	root.SetIn(stdin)
	root.SetOut(&stdout)
	root.SetErr(io.Discard)

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	root.SetContext(ctx)
	root.SetArgs([]string{"server", "acp"})

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

func TestServeACPCommandCapturesInvocationOwnedProfile(t *testing.T) {
	t.Parallel()

	fake := &fakeACPServer{}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake
	lookup := func(name string) (string, bool) {
		values := map[string]string{
			operatorsettings.EnvDefaultWorkerModelProvider: "codex",
			operatorsettings.EnvDefaultWorkerModel:         "gpt-5",
		}
		value, ok := values[name]
		return value, ok
	}
	root := factory.NewCommand(func() (string, error) { return "isolated-home", nil }, lookup, noOpServeSystemInitializer())
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"server", "acp"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	profile, ok := acp.InvocationProfileFromContext(fake.gotCtx)
	want := acp.InvocationProfile{HomeDir: "isolated-home", WorkerModelProvider: "codex", WorkerModel: "gpt-5"}
	if !ok || profile != want {
		t.Fatalf("ACP invocation profile = (%+v, %t), want (%+v, true)", profile, ok, want)
	}
}

func TestServeACPCommandInitializesSystemBeforeServing(t *testing.T) {
	fake := &fakeACPServer{}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	var initializedHome string
	initializer := startupcli.Functions{InitializeSystemFunc: func(_ context.Context, homeDir string) error {
		initializedHome = homeDir
		return nil
	}}
	root := factory.NewCommand(func() (string, error) { return "operator-home", nil }, nil, initializer)
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	var stderr bytes.Buffer
	root.SetErr(&stderr)
	root.SetArgs([]string{"server", "acp"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if initializedHome != "operator-home" {
		t.Fatalf("initialized home = %q, want operator-home", initializedHome)
	}
	if fake.calls != 1 {
		t.Fatalf("Serve call count = %d, want 1 after system initialization", fake.calls)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("server ACP diagnostics = %q, want clean protocol diagnostics", got)
	}
}

func TestServeACPCommand_CleanEOFSucceeds(t *testing.T) {
	fake := &fakeACPServer{serveErr: nil}
	factory := withTestInjectedPlatformRoles(CommandFactory{ModelsCLI: rootModelsCLI})
	factory.acpServer = fake

	root := factory.NewCommand(func() (string, error) { return "operator-home", nil }, nil, noOpServeSystemInitializer())
	root.SetIn(strings.NewReader(""))
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs([]string{"server", "acp"})

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

	root := factory.NewCommand(func() (string, error) { return "operator-home", nil }, nil, noOpServeSystemInitializer())
	root.SetIn(strings.NewReader(""))
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	root.SetContext(ctx)
	root.SetArgs([]string{"server", "acp"})
	cancel()

	if err := root.Execute(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestExecuteCommandResultPreservesCancellationIdentityAfterDiagnosticStateChanges(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		err      error
		rendered bool
	}{
		{name: "unrendered", err: context.Canceled},
		{name: "wrapped unrendered", err: fmt.Errorf("ACP server stopped: %w", context.Canceled)},
		{name: "already rendered", err: fmt.Errorf("ACP server stopped: %w", context.Canceled), rendered: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var stderr bytes.Buffer
			diagnostics := clidiag.NewDiagnosticWriter(&stderr)
			if tc.rendered {
				if !clidiag.WriteFailure(diagnostics, &clidiag.Failure{
					Code:    "ACP_TEST_FAILURE",
					Message: "already rendered",
				}) {
					t.Fatal("WriteFailure() = false, want a rendered diagnostic")
				}
			}
			beforeCancellation := stderr.String()

			got := executeCommandResult(diagnostics, tc.err)
			if got != context.Canceled {
				t.Fatalf("executeCommandResult() = %T %v, want context.Canceled by identity", got, got)
			}

			if tc.rendered {
				if stderr.String() != beforeCancellation {
					t.Fatalf("already-rendered stderr changed from %q to %q, want no duplicate diagnostic", beforeCancellation, stderr.String())
				}
				return
			}
			if got := stderr.String(); got != "Error: context canceled\n" {
				t.Fatalf("unrendered cancellation stderr = %q, want exact cancellation diagnostic", got)
			}
		})
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
	server := acpwire.NewServer(nil, nil, nil, nil, nil, nil, nil, nil)
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

	root := factory.NewCommand(func() (string, error) { return "operator-home", nil }, nil, noOpServeSystemInitializer())
	root.SetIn(signalingStdin)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)

	ctx, cancel := context.WithCancel(context.Background())
	root.SetContext(ctx)
	root.SetArgs([]string{"server", "acp"})

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
	root.SetArgs([]string{"server", "acp"})

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

			root := factory.NewCommand(func() (string, error) { return "operator-home", nil }, nil, noOpServeSystemInitializer())
			root.SetIn(strings.NewReader(""))
			var stdout, stderr bytes.Buffer
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			root.SetArgs([]string{"server", "acp"})

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
