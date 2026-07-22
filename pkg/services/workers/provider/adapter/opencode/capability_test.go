package opencode_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode"
)

func TestResolverCachesNegotiationAndDeclaresSelectedCapabilities(t *testing.T) {
	t.Parallel()
	identifier := &fakeIdentifier{installation: opencode.Installation{Executable: "/bin/opencode", Fingerprint: "first"}}
	discoverer := &fakeDiscoverer{decision: opencode.Decision{Version: "1.2.3", Mode: opencode.ModeStructured}}
	resolver := newResolver(t, identifier, discoverer)

	first := resolve(t, resolver)
	second := resolve(t, resolver)
	if discoverer.calls.Load() != 1 {
		t.Fatalf("discovery calls = %d, want 1", discoverer.calls.Load())
	}
	if first != second || first.Installation.Fingerprint != "first" {
		t.Fatalf("decisions = %#v and %#v", first, second)
	}
	capabilities := first.Capabilities()
	if !capabilities.NativeStreaming || capabilities.MessageDeltas || !capabilities.MessageSnapshots || capabilities.ReasoningSummaries || capabilities.ToolLifecycle || !capabilities.StableItemIDs || capabilities.FinalOnly {
		t.Fatalf("structured capabilities = %#v", capabilities)
	}
}

func TestResolverRenegotiatesWhenExecutableIdentityChanges(t *testing.T) {
	t.Parallel()
	identifier := &fakeIdentifier{installation: opencode.Installation{Executable: "/bin/opencode", Fingerprint: "first"}}
	discoverer := &fakeDiscoverer{decision: opencode.Decision{Version: "1.0.0", Mode: opencode.ModeStructured}}
	resolver := newResolver(t, identifier, discoverer)
	resolve(t, resolver)

	identifier.mu.Lock()
	identifier.installation.Fingerprint = "replacement"
	identifier.mu.Unlock()
	discoverer.mu.Lock()
	discoverer.decision.Version = "2.0.0"
	discoverer.mu.Unlock()
	decision := resolve(t, resolver)
	if discoverer.calls.Load() != 2 || decision.Version != "2.0.0" {
		t.Fatalf("calls = %d, decision = %#v", discoverer.calls.Load(), decision)
	}
}

func TestResolverKnownUnsupportedSelectsAccurateFinalOnlyCapabilities(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, &fakeDiscoverer{
		decision: opencode.Decision{Version: "0.9.0", Mode: opencode.ModeFinalOnly},
	})
	decision := resolve(t, resolver)
	capabilities := decision.Capabilities()
	if decision.Mode != opencode.ModeFinalOnly || capabilities.NativeStreaming || capabilities.MessageDeltas || capabilities.ToolLifecycle || !capabilities.MessageSnapshots || !capabilities.FinalOnly {
		t.Fatalf("final-only decision = %#v, capabilities = %#v", decision, capabilities)
	}
}

func TestResolverFailedDiscoveryDoesNotPoisonCache(t *testing.T) {
	t.Parallel()
	discoverer := &fakeDiscoverer{err: errors.New("temporary probe failure")}
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, discoverer)
	if _, err := resolver.Resolve(context.Background(), "opencode"); err == nil {
		t.Fatal("Resolve() error = nil, want discovery failure")
	}
	discoverer.mu.Lock()
	discoverer.err = nil
	discoverer.decision = opencode.Decision{Version: "1.0.0", Mode: opencode.ModeStructured}
	discoverer.mu.Unlock()
	resolve(t, resolver)
	if discoverer.calls.Load() != 2 {
		t.Fatalf("discovery calls = %d, want 2", discoverer.calls.Load())
	}
}

func TestResolverDowngradeIsReusableAndIdempotentForConcurrentStaleCallers(t *testing.T) {
	t.Parallel()
	discoverer := &fakeDiscoverer{decision: opencode.Decision{Version: "1.2.3", Mode: opencode.ModeStructured}}
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, discoverer)
	structured := resolve(t, resolver)
	first, err := resolver.Downgrade(structured)
	if err != nil {
		t.Fatalf("Downgrade() error = %v", err)
	}
	second, err := resolver.Downgrade(structured)
	if err != nil {
		t.Fatalf("idempotent Downgrade() error = %v", err)
	}
	cached := resolve(t, resolver)
	if first.Mode != opencode.ModeFinalOnly || second != first || cached != first || discoverer.calls.Load() != 1 {
		t.Fatalf("downgrades = %#v / %#v, cached = %#v, calls = %d", first, second, cached, discoverer.calls.Load())
	}
}

func TestResolverConcurrentCallsShareOneDiscovery(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	discoverer := &fakeDiscoverer{decision: opencode.Decision{Version: "1.0.0", Mode: opencode.ModeStructured}, release: release}
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, discoverer)
	const callers = 24
	results := make(chan error, callers)
	for range callers {
		go func() {
			_, err := resolver.Resolve(context.Background(), "opencode")
			results <- err
		}()
	}
	deadline := time.Now().Add(time.Second)
	for discoverer.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(release)
	for range callers {
		if err := <-results; err != nil {
			t.Fatalf("Resolve() error = %v", err)
		}
	}
	if discoverer.calls.Load() != 1 {
		t.Fatalf("discovery calls = %d, want 1", discoverer.calls.Load())
	}
}

func TestResolverCanceledWaitDoesNotCancelSharedDiscovery(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	discoverer := &fakeDiscoverer{decision: opencode.Decision{Version: "1.0.0", Mode: opencode.ModeStructured}, release: release}
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, discoverer)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := resolver.Resolve(ctx, "opencode"); done <- err }()
	deadline := time.Now().Add(time.Second)
	for discoverer.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("Resolve() error = %v, want context canceled", err)
	}
	close(release)
	resolve(t, resolver)
	if discoverer.calls.Load() != 1 {
		t.Fatalf("discovery calls = %d, want 1", discoverer.calls.Load())
	}
}

func TestResolverBoundsConcurrentDistinctInstallations(t *testing.T) {
	t.Parallel()
	release := make(chan struct{})
	identifier := &fakeIdentifier{installation: opencode.Installation{Executable: "/bin/opencode", Fingerprint: "first"}}
	discoverer := &fakeDiscoverer{decision: opencode.Decision{Version: "1.0.0", Mode: opencode.ModeStructured}, release: release}
	resolver, err := opencode.NewResolver(identifier, discoverer, 1, time.Second)
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}
	first := make(chan error, 1)
	go func() { _, resolveErr := resolver.Resolve(context.Background(), "opencode"); first <- resolveErr }()
	deadline := time.Now().Add(time.Second)
	for discoverer.calls.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	identifier.mu.Lock()
	identifier.installation.Fingerprint = "second"
	identifier.mu.Unlock()
	if _, err := resolver.Resolve(context.Background(), "opencode"); !errors.Is(err, opencode.ErrCapabilityCacheFull) {
		t.Fatalf("Resolve() error = %v, want cache full", err)
	}
	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first Resolve() error = %v", err)
	}
}

func TestCommandDiscovererUsesPromptFreeBoundedProbe(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{results: []workerprocess.CommandResult{
		{Stdout: []byte("opencode 1.2.3\n")},
		{ExitCode: 2, Stdout: []byte("human help is ignored")},
	}}
	discoverer := opencode.CommandDiscoverer{Runner: runner}
	decision, err := discoverer.Discover(context.Background(), installation())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if decision.Mode != opencode.ModeFinalOnly || decision.Version != "opencode 1.2.3" {
		t.Fatalf("decision = %#v", decision)
	}
	if len(runner.requests) != 2 || len(runner.requests[0].Stdin) != 0 || len(runner.requests[1].Stdin) != 0 {
		t.Fatalf("requests = %#v", runner.requests)
	}
	want := []string{"run", "--format", "json", "--help"}
	for index := range want {
		if runner.requests[1].Args[index] != want[index] {
			t.Fatalf("probe args = %#v, want %#v", runner.requests[1].Args, want)
		}
	}
}

func newResolver(t *testing.T, identifier opencode.Identifier, discoverer opencode.Discoverer) *opencode.Resolver {
	t.Helper()
	resolver, err := opencode.NewResolver(identifier, discoverer, 4, time.Second)
	if err != nil {
		t.Fatalf("NewResolver() error = %v", err)
	}
	return resolver
}

func resolve(t *testing.T, resolver *opencode.Resolver) opencode.Decision {
	t.Helper()
	decision, err := resolver.Resolve(context.Background(), "opencode")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	return decision
}

func installation() opencode.Installation {
	return opencode.Installation{Executable: "/bin/opencode", Fingerprint: "digest"}
}

type fakeIdentifier struct {
	mu           sync.Mutex
	installation opencode.Installation
}

func (f *fakeIdentifier) Identify(context.Context, string) (opencode.Installation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.installation, nil
}

type fakeDiscoverer struct {
	mu       sync.Mutex
	decision opencode.Decision
	err      error
	release  <-chan struct{}
	calls    atomic.Int32
}

func (f *fakeDiscoverer) Discover(ctx context.Context, _ opencode.Installation) (opencode.Decision, error) {
	f.calls.Add(1)
	if f.release != nil {
		select {
		case <-ctx.Done():
			return opencode.Decision{}, ctx.Err()
		case <-f.release:
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.decision, f.err
}

type recordingRunner struct {
	requests []workerprocess.CommandRequest
	results  []workerprocess.CommandResult
}

func TestNewDefaultResolverConstructsProductionBoundaries(t *testing.T) {
	t.Parallel()
	resolver, err := opencode.NewDefaultResolver(
		&recordingRunner{},
		func(path string) (string, error) { return path, nil },
		fixedExecutableLocator(func(file string) (string, error) { return file, nil }),
		fixedExecutableFiles(func(string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("executable")), nil
		}),
	)
	if err != nil || resolver == nil {
		t.Fatalf("NewDefaultResolver() = %#v, %v", resolver, err)
	}
}

func TestNewDefaultResolverRequiresInjectedSymlinkResolver(t *testing.T) {
	t.Parallel()
	if resolver, err := opencode.NewDefaultResolver(&recordingRunner{}, nil, nil, nil); err == nil || resolver != nil {
		t.Fatalf("NewDefaultResolver() = %#v, %v; want missing resolver error", resolver, err)
	}
}

func TestNewDefaultResolverRequiresInjectedExecutableEffects(t *testing.T) {
	t.Parallel()
	resolve := func(path string) (string, error) { return path, nil }
	locator := fixedExecutableLocator(func(file string) (string, error) { return file, nil })
	files := fixedExecutableFiles(func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("executable")), nil
	})
	if resolver, err := opencode.NewDefaultResolver(&recordingRunner{}, resolve, nil, files); err == nil || resolver != nil || !strings.Contains(err.Error(), "locator is required") {
		t.Fatalf("missing locator result = %#v, %v", resolver, err)
	}
	if resolver, err := opencode.NewDefaultResolver(&recordingRunner{}, resolve, locator, nil); err == nil || resolver != nil || !strings.Contains(err.Error(), "file reader is required") {
		t.Fatalf("missing file reader result = %#v, %v", resolver, err)
	}
}

func TestExecutableIdentifierUsesOnlyInjectedLookupAndFileEffects(t *testing.T) {
	t.Parallel()
	const contents = "deterministic executable contents"
	lookedUp := ""
	opened := ""
	identifier := opencode.ExecutableIdentifier{
		ResolveSymlinks: func(path string) (string, error) {
			if !strings.HasSuffix(path, "opencode-edge") {
				t.Fatalf("absolute lookup path = %q, want opencode-edge suffix", path)
			}
			return "/canonical/opencode", nil
		},
		Locator: fixedExecutableLocator(func(file string) (string, error) {
			lookedUp = file
			return "opencode-edge", nil
		}),
		Files: fixedExecutableFiles(func(path string) (io.ReadCloser, error) {
			opened = path
			return io.NopCloser(strings.NewReader(contents)), nil
		}),
	}

	installation, err := identifier.Identify(context.Background(), " opencode ")
	if err != nil {
		t.Fatalf("Identify() error = %v", err)
	}
	wantDigest := sha256.Sum256([]byte(contents))
	if lookedUp != "opencode" || opened != "/canonical/opencode" ||
		installation.Executable != "/canonical/opencode" ||
		installation.Fingerprint != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("Identify() = %#v, lookup %q, open %q", installation, lookedUp, opened)
	}
}

func TestExecutableIdentifierFailsClosedWithoutInjectedEffects(t *testing.T) {
	t.Parallel()
	resolve := func(path string) (string, error) { return path, nil }
	files := fixedExecutableFiles(func(string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("executable")), nil
	})
	locator := fixedExecutableLocator(func(file string) (string, error) { return file, nil })

	if _, err := (opencode.ExecutableIdentifier{ResolveSymlinks: resolve, Files: files}).Identify(context.Background(), "opencode"); err == nil || !strings.Contains(err.Error(), "locator is required") {
		t.Fatalf("missing locator error = %v", err)
	}
	if _, err := (opencode.ExecutableIdentifier{ResolveSymlinks: resolve, Locator: locator}).Identify(context.Background(), "opencode"); err == nil || !strings.Contains(err.Error(), "file reader is required") {
		t.Fatalf("missing file reader error = %v", err)
	}
}

type fixedExecutableLocator func(string) (string, error)

func (f fixedExecutableLocator) LookPath(file string) (string, error) { return f(file) }

type fixedExecutableFiles func(string) (io.ReadCloser, error)

func (f fixedExecutableFiles) Open(path string) (io.ReadCloser, error) { return f(path) }

func (r *recordingRunner) Run(_ context.Context, request workerprocess.CommandRequest) (workerprocess.CommandResult, error) {
	r.requests = append(r.requests, request)
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}
