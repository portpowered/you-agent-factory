package contract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// One compatible graph and one host serve all parallel, explicit-session cells.
// Construction/catalog observations precede hosting because they are global.
func TestProvidersCompletedPeers(t *testing.T) {
	// Default construction has no activation and requires no available provider.
	defaultProcess := support.BuildProcess(t, serviceedges.Edges{})
	if err := defaultProcess.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	f := newCompletedPeersFixture(t)
	t.Run("TestProvidersCompletedPeersInertConstruction", func(t *testing.T) {
		if f.commandCalls.Load() != 0 || f.pipeCalls.Load() != 0 || f.external.Stats().InvokeCalls != 0 {
			t.Fatal("construction invoked provider effects")
		}
	})
	t.Run("TestProvidersCompletedPeersCatalogAndRegistrationOverrides", func(t *testing.T) { testCompletedPeersCatalogAndRegistrationOverrides(t, f) })
	f.start(t)
	t.Run("TestProvidersCompletedPeersNativeCommandsAndDenial", func(t *testing.T) { testCompletedPeersNativeCommandsAndDenial(t, f) })
	t.Run("TestProvidersCompletedPeersTimeoutCancellationAndIsolation", func(t *testing.T) { testCompletedPeersTimeoutCancellationAndIsolation(t, f) })
	t.Run("TestProvidersCompletedPeersACPChannelFailure", func(t *testing.T) { testCompletedPeersACPChannelFailure(t, f) })
	t.Run("TestProvidersCompletedPeersClockAndOutputIsolation", func(t *testing.T) { testCompletedPeersClockAndOutputIsolation(t, f) })
}

func testCompletedPeersCatalogAndRegistrationOverrides(t *testing.T, f *completedPeersFixture) {
	if f.commandCalls.Load() != 0 || f.pipeCalls.Load() != 0 {
		t.Fatal("construction invoked provider effects")
	}
	input := support.FakeInputs(t.Context(), []string{"you", "providers", "list", "--json"})
	input.Input.Env = f.env
	if err := f.process.Execute(input.Input); err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Providers []struct {
			ID           string
			Capabilities []string
		}
	}
	if err := json.Unmarshal([]byte(input.Stdout()), &catalog); err != nil {
		t.Fatal(err)
	}
	foundExternal, foundCodex := false, false
	for _, provider := range catalog.Providers {
		if provider.ID == "completed-peer-external" {
			foundExternal = true
		}
		if provider.ID == "codex" {
			foundCodex = true
			if !slices.Equal(provider.Capabilities, []string{"prompt_submission"}) {
				t.Fatalf("published overridden capabilities = %v", provider.Capabilities)
			}
		}
	}
	if !foundExternal || !foundCodex || f.commandCalls.Load() != 0 || f.pipeCalls.Load() != 0 {
		t.Fatal("catalog lost an override/registration or launched a provider")
	}
}

func testCompletedPeersNativeCommandsAndDenial(t *testing.T, f *completedPeersFixture) {
	t.Parallel()
	for _, name := range []string{"codex", "claude", "denied", "authorized", "external"} {
		route := f.routes[name]
		id := f.open(t, route)
		content := route.answer
		if name == "denied" {
			content = "permission"
		}
		f.assertWork(t, id, name != "denied", content)
		if name == "external" {
			if f.external.Stats().InvokeCalls != 1 {
				t.Fatal("external route did not execute")
			}
			continue
		}
		if name == "denied" {
			select {
			case <-route.entered:
				t.Fatal("denied route launched command")
			default:
			}
			continue
		}
		req := <-route.entered
		if req.Command != route.provider || req.ExecutionScopeID != id {
			t.Fatalf("command=%q scope=%q, want %s/%s", req.Command, req.ExecutionScopeID, route.provider, id)
		}
		if name == "authorized" && slices.Contains(req.Args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatal("rejected bypass leaked into authorized invocation")
		}
	}
}

func testCompletedPeersTimeoutCancellationAndIsolation(t *testing.T, f *completedPeersFixture) {
	t.Parallel()
	timeout, peer := f.routes["timeout"], f.routes["timeout-peer"]
	timeoutID, peerID := f.open(t, timeout), f.open(t, peer)
	waitCommand(t, timeout.entered)
	waitCommand(t, peer.entered)
	select {
	case timer := <-f.scheduler.selected:
		f.clock.millis.Store(37000)
		f.scheduler.expired.Store(true)
		timer.fire()
	case <-time.After(30 * time.Second):
		t.Fatal("selected scheduler did not receive provider timeout")
	}
	waitSignal(t, timeout.joined)
	f.assertWork(t, timeoutID, false, `"reason":"timeout"`)
	select {
	case <-peer.joined:
		t.Fatal("timeout canceled independent peer")
	default:
	}
	close(peer.release)
	f.assertWork(t, peerID, true, peer.answer)
	canceled, surviving := f.routes["canceled"], f.routes["cancel-peer"]
	cancelID, survivingID := f.open(t, canceled), f.open(t, surviving)
	waitCommand(t, canceled.entered)
	waitCommand(t, surviving.entered)
	support.TerminateFactorySessionAt(t, f.baseURL, cancelID)
	waitSignal(t, canceled.joined)
	// Termination stops the runtime and restores interrupted Work to its
	// existing state; it does not publish a failed Work transition.
	support.WaitForSessionStopped(t, f.baseURL, cancelID, 30*time.Second)
	canceledWork := support.GetJSON[api.ListWorkResponse](t, f.baseURL+"/factory-sessions/"+cancelID+"/work")
	if support.CountWorkAtCustomerState(canceledWork, "task:init") != 1 || support.CountWorkAtCustomerState(canceledWork, "task:done") != 0 {
		t.Fatalf("terminated Work = %#v, want retained initial Work and no success", canceledWork)
	}
	select {
	case <-surviving.joined:
		t.Fatal("session cancellation canceled peer")
	default:
	}
	close(surviving.release)
	f.assertWork(t, survivingID, true, surviving.answer)
	reuse := f.routes["reuse"]
	f.assertWork(t, f.open(t, reuse), true, reuse.answer)
}

func testCompletedPeersACPChannelFailure(t *testing.T, f *completedPeersFixture) {
	t.Parallel()
	route := f.routes["acp"]
	f.assertWork(t, f.open(t, route), false, "")
	if f.pipeCalls.Load() == 0 {
		t.Fatalf("channel opens = %d", f.pipeCalls.Load())
	}
	native := f.routes["acp-peer"]
	f.assertWork(t, f.open(t, native), true, native.answer)
}

func testCompletedPeersClockAndOutputIsolation(t *testing.T, f *completedPeersFixture) {
	t.Parallel()
	for _, name := range []string{"output-one", "output-two"} {
		route := f.routes[name]
		id := f.open(t, route)
		f.assertWork(t, id, true, route.answer)
		events := support.GetFactoryEventsForSessionAt(t, f.baseURL, id)
		encoded, _ := json.Marshal(events)
		if strings.Contains(string(encoded), "private-command-diagnostic") {
			t.Fatal("events leaked native diagnostic")
		}
		other := "output-one"
		if name == other {
			other = "output-two"
		}
		if strings.Contains(string(encoded), f.routes[other].answer) {
			t.Fatal("events leaked peer output")
		}
		// The deliberately historical Now-only source must reach public facts;
		// the separate scheduler remains usable without advancing replay ticks.
		if !strings.Contains(string(encoded), "2001-02-03") {
			t.Fatal("public events did not use selected process clock")
		}
	}
}

type completedPeerRoute struct {
	dir, provider, answer string
	entered               chan platformprocess.CommandRequest
	release               chan struct{}
	joined                chan struct{}
	joinOnce              sync.Once
}

type completedPeersFixture struct {
	process                 support.ApplicationProcess
	server                  *support.ProcessAPIServer
	env                     []string
	baseURL, hostDir        string
	routes                  map[string]*completedPeerRoute
	byDir                   map[string]*completedPeerRoute
	clock                   *completedPeerClock
	scheduler               *completedPeerScheduler
	external                *providerswire.ProgressingIntegration
	commandCalls, pipeCalls atomic.Int32
}

func newCompletedPeersFixture(t *testing.T) *completedPeersFixture {
	t.Helper()
	f := &completedPeersFixture{server: support.NewProcessAPIServer(), routes: map[string]*completedPeerRoute{},
		byDir: map[string]*completedPeerRoute{}, clock: &completedPeerClock{},
		scheduler: &completedPeerScheduler{selected: make(chan *completedPeerTimer, 4)},
		external:  providerswire.ProgressingExternalIntegration("completed-peer-external", "external answer COMPLETE"),
	}
	f.env = builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir())
	f.hostDir = support.ScaffoldSingleStepFactory(t, "completed-peers-host")
	for _, name := range []string{"codex", "claude", "denied", "authorized", "external", "timeout", "timeout-peer", "canceled", "cancel-peer", "reuse", "acp", "acp-peer", "output-one", "output-two"} {
		f.addRoute(t, name)
	}
	var err error
	f.process, err = support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		APIServerStarter: f.server.Start, ProviderCommandRunner: f,
		Clock: f.clock, ProcessScheduler: f.scheduler,
		ProviderCatalogCapabilityOverrides: []providerswire.CatalogCapabilityOverride{{Provider: providers.IDCodex, Capabilities: []providers.Capability{providers.CapabilityPromptSubmission}}},
		ProviderRegistrations: providerswire.ProviderRegistrations{{Manifest: providerswire.Manifest{
			ID: "completed-peer-external", ImplementationAvailability: providerswire.ImplementationExternallySupplied,
			TechnicalSupportLevel: providerswire.SupportProduction, MaximumExecutionCapabilities: providerswire.ExecutionCapabilities{PromptSubmission: true},
		}, Integration: f.external}},
		ProvidersExecutableLocator: completedPeerLocator{},
		ProvidersStdioPipeFactory: func() (platformprocess.StdioChannel, error) {
			f.pipeCalls.Add(1)
			return nil, errors.New("controlled channel unavailable")
		},
		PlatformProcessCommandFactory: func(name string, args ...string) *exec.Cmd { return exec.Command(name, args...) },
	})
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, f.process)
	return f
}

func (f *completedPeersFixture) addRoute(t *testing.T, name string) {
	t.Helper()
	provider, model := "agy", "gemini-3.6-flash-high"
	if slices.Contains([]string{"codex", "denied", "authorized"}, name) {
		provider, model = "codex", "gpt-5.4"
	}
	if name == "claude" {
		provider, model = "claude", "claude-sonnet-4-6"
	}
	if name == "external" {
		provider, model = "completed-peer-external", "fixture-model"
	}
	if name == "acp" {
		provider, model = "cursor", "fixture-model"
	}
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	identity := provider
	if identity == "agy" {
		identity = "antigravity"
	}
	config := fmt.Sprintf("---\ntype: MODEL_WORKER\nmodelProvider: %s\nmodel: %s\nstopToken: COMPLETE\ntimeout: 1h\nskipPermissions: false\n", identity, model)
	if name == "timeout" {
		config = strings.ReplaceAll(config, "timeout: 1h", "timeout: 37s")
	}
	if name == "denied" {
		config = strings.ReplaceAll(config, "skipPermissions: false", "skipPermissions: true")
	}
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"completed provider peers"}`))
	support.WriteAgentConfig(t, dir, "worker", config+"---\nProcess the input task.\n")
	route := &completedPeerRoute{dir: filepath.Clean(dir), provider: provider, answer: name + " answer COMPLETE", entered: make(chan platformprocess.CommandRequest, 16), joined: make(chan struct{})}
	if name == "external" {
		route.answer = "external answer COMPLETE"
	}
	if slices.Contains([]string{"timeout", "timeout-peer", "canceled", "cancel-peer"}, name) {
		route.release = make(chan struct{})
	}
	f.routes[name], f.byDir[filepath.Clean(dir)] = route, route
}

func (f *completedPeersFixture) start(t *testing.T) {
	t.Helper()
	input := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", f.hostDir, "--continuously", "--with-server", "--server", "http://127.0.0.1:1", "--quiet", "--no-record"})
	input.Input.Env, input.Input.WorkingDirectory = f.env, f.hostDir
	support.StartProcessCommand(t, f.process, input.Input)
	f.baseURL = f.server.WaitForURL(t)
}

func (f *completedPeersFixture) open(t *testing.T, route *completedPeerRoute) string {
	t.Helper()
	id := support.OpenFactorySessionAt(t, f.baseURL, route.dir).Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, f.baseURL, id) })
	return id
}

func (f *completedPeersFixture) assertWork(t *testing.T, id string, success bool, content string) {
	t.Helper()
	support.WaitForSessionTerminalStatus(t, f.baseURL, id, 30*time.Second)
	listed := support.GetJSON[api.ListWorkResponse](t, f.baseURL+"/factory-sessions/"+id+"/work")
	state := "task:failed"
	if success {
		state = "task:done"
	}
	if support.CountWorkAtCustomerState(listed, state) != 1 {
		encoded, _ := json.Marshal(listed)
		t.Fatalf("session %s Work = %s, want %s", id, encoded, state)
	}
	events := support.GetFactoryEventsForSessionAt(t, f.baseURL, id)
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatal(err)
	}
	if content != "" && !strings.Contains(string(encoded), content) {
		t.Fatalf("session %s events lack %q: %s", id, content, encoded)
	}
}

func (f *completedPeersFixture) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	f.commandCalls.Add(1)
	route := f.byDir[filepath.Clean(req.WorkDir)]
	if route == nil {
		return platformprocess.CommandResult{}, fmt.Errorf("unregistered command directory")
	}
	defer route.joinOnce.Do(func() { close(route.joined) })
	route.entered <- req
	if route.release != nil {
		select {
		case <-route.release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
	var output []byte
	switch req.Command {
	case "codex":
		output = support.CodexSuccessStdout(route.answer)
	case "claude":
		output = support.ClaudeSuccessStdout(route.answer)
	case "agy":
		output, _ = json.Marshal(map[string]any{"event": "result", "result": map[string]any{"conversation_id": route.answer, "status": "SUCCESS", "response": route.answer, "duration_seconds": 1.0, "num_turns": 1, "usage": map[string]int{"input_tokens": 1, "output_tokens": 1, "thinking_tokens": 0, "cache_read_tokens": 0, "total_tokens": 2}}})
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected command %s", req.Command)
	}
	return platformprocess.CommandResult{Stdout: output, Stderr: []byte("private-command-diagnostic")}, nil
}

type completedPeerLocator struct{}

func (completedPeerLocator) LookPath(name string) (string, error) { return name, nil }

type completedPeerClock struct{ millis atomic.Int64 }

func (c *completedPeerClock) Now() time.Time {
	return time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC).Add(time.Duration(c.millis.Load()) * time.Millisecond)
}

type completedPeerScheduler struct {
	selected chan *completedPeerTimer
	expired  atomic.Bool
}

func (*completedPeerScheduler) Now() time.Time                         { return time.Now() }
func (*completedPeerScheduler) After(d time.Duration) <-chan time.Time { return time.After(d) }
func (s *completedPeerScheduler) NewTimer(d time.Duration) platformclock.Timer {
	if d != 37*time.Second {
		return platformclock.Real{}.NewTimer(d)
	}
	timer := &completedPeerTimer{channel: make(chan time.Time, 1)}
	if s.expired.Load() {
		timer.fire()
	} else {
		s.selected <- timer
	}
	return timer
}

type completedPeerTimer struct {
	mu      sync.Mutex
	stopped bool
	channel chan time.Time
}

func (t *completedPeerTimer) C() <-chan time.Time { return t.channel }
func (t *completedPeerTimer) Stop() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	active := !t.stopped
	t.stopped = true
	return active
}
func (t *completedPeerTimer) fire() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.stopped {
		t.channel <- time.Now()
		t.stopped = true
	}
}

func waitCommand(t *testing.T, signal <-chan platformprocess.CommandRequest) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("provider command did not enter")
	}
}
func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(30 * time.Second):
		t.Fatal("provider command did not join")
	}
}
