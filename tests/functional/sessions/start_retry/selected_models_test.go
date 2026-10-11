package start_retry_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const selectedModelsRevision = "0123456789abcdef0123456789abcdef01234567"
const selectedModelsBackend = "localai-llamacpp"

type selectedModelsScenario struct {
	initialOpeningScenario
	peerHome   string
	backend    serviceedges.ModelBackendArtifactSelection
	mu         sync.Mutex
	starts     map[string]int
	negotiated map[string]int
}

func newSelectedModelsScenario(t *testing.T) *selectedModelsScenario {
	t.Helper()
	scenario := &selectedModelsScenario{initialOpeningScenario: initialOpeningScenarioWithConfig(t, initialOpeningFactoryConfig()),
		peerHome: t.TempDir(), starts: make(map[string]int), negotiated: make(map[string]int)}
	body := []byte("selected backend fixture")
	scenario.backend = serviceedges.ModelBackendArtifactSelection{Name: "fixture-backend.tar.gz", Location: "https://github.com/selected/fixture/releases/download/v1/fixture-backend.tar.gz", Bytes: int64(len(body)), SHA256: fmt.Sprintf("%x", sha256.Sum256(body))}
	for _, entry := range []struct{ dir, home, id string }{{scenario.candidateDir, scenario.home, scenario.candidateID}, {scenario.peerDir, scenario.peerHome, scenario.peerID}} {
		config := selectedModelsFactoryConfig(entry.id)
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(entry.dir, "factory.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		support.WriteWorkstationConfig(t, entry.dir, "process", "---\ntype: INFERENCE_RUN\n---\nReturn the model result.\n")
		source := "hf://selected/" + entry.id
		settings, err := json.Marshal(map[string]any{"models": map[string]any{"llm": map[string]any{"source": source}}})
		if err != nil {
			t.Fatal(err)
		}
		settingsPath := filepath.Join(entry.home, ".you-agent-factory", "config.json")
		if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(settingsPath, settings, 0o600); err != nil {
			t.Fatal(err)
		}
		cache := filepath.Join(entry.home, ".agent-factory", "models")
		seedSelectedModelAsset(t, cache, "model", source+"@"+selectedModelsRevision, "selected.gguf", []byte(entry.id))
		urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(scenario.backend.Location)))
		seedSelectedModelAsset(t, filepath.Join(cache, "backend-artifacts"), "backend", "backend://"+selectedModelsBackend+"/release://"+urlHash, scenario.backend.Name, body)
	}
	return scenario
}

func selectedModelsFactoryConfig(id string) map[string]any {
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	config["resources"] = []map[string]any{{"name": "llm-cache", "type": "MODEL", "capacity": 1, "model": "llm", "backend": selectedModelsBackend, "loadPolicy": "ON_DEMAND"}}
	config["workers"] = []map[string]any{{"name": "worker-a", "type": "INFERENCE_WORKER", "model": "llm", "modelProvider": "CODEX", "modelLocality": "LOCAL", "command": "llama-cpp",
		"args": []string{"--grpc-endpoint", "http://" + id + ".invalid"}, "resources": []map[string]any{{"name": "llm-cache", "capacity": 1}},
		"operations": []map[string]any{{"name": "OMNI", "inputs": []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}}, "outputs": []map[string]any{{"name": "text", "contentTypes": []string{"TEXT"}, "required": true}}}}}}
	station := config["workstations"].([]map[string]any)[0]
	station["type"], station["operation"] = "INFERENCE_RUN", "OMNI"
	station["operationBindings"] = []map[string]any{{"slot": "prompt", "selector": map[string]any{"type": "TEXT"}}}
	return config
}

// Cache fixtures model installed customer assets. All runtime reads still go
// through production Models; unexpected downloads fail at the exact HTTP edge.
func seedSelectedModelAsset(t *testing.T, cache, kind, source, name string, body []byte) {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("%s|%s|%s:%d:%s", kind, source, name, len(body), digest)
	snapshot := filepath.Join(cache, ".you-content-addressed", kind, fmt.Sprintf("%x", sha256.Sum256([]byte(identity))))
	if err := os.MkdirAll(snapshot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, err := json.Marshal(map[string]any{"kind": kind, "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), metadata, 0o600); err != nil {
		t.Fatal(err)
	}
}

func (scenario *selectedModelsScenario) Start(_ context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(spec.HealthEndpoint, "http://"), ".invalid")
	home := scenario.home
	if id == scenario.peerID {
		home = scenario.peerHome
	} else if id != scenario.candidateID {
		return nil, fmt.Errorf("unselected model endpoint %q", spec.HealthEndpoint)
	}
	body, err := os.ReadFile(spec.ModelPath)
	if err != nil || string(body) != id || !strings.HasPrefix(spec.ModelPath, home+string(filepath.Separator)) {
		return nil, fmt.Errorf("selected endpoint %s received another scope's model asset", id)
	}
	scenario.mu.Lock()
	scenario.starts[id]++
	scenario.mu.Unlock()
	return &selectedModelsProcess{endpoint: spec.HealthEndpoint, stopped: make(chan struct{})}, nil
}

type selectedModelsProcess struct {
	endpoint string
	stopped  chan struct{}
	once     sync.Once
}

func (process *selectedModelsProcess) HealthEndpoint() string { return process.endpoint }
func (process *selectedModelsProcess) Wait() error            { <-process.stopped; return nil }
func (process *selectedModelsProcess) Stop(context.Context) error {
	process.once.Do(func() { close(process.stopped) })
	return nil
}

func (scenario *selectedModelsScenario) Negotiate(_ context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	id := strings.TrimSuffix(strings.TrimPrefix(endpoint, "http://"), ".invalid")
	body, err := os.ReadFile(request.ModelPath)
	if err != nil || string(body) != id || request.ModelName != "llm" || request.Revision != selectedModelsRevision {
		return serviceedges.ModelHostProtocolNegotiationResult{}, fmt.Errorf("protocol model selection differs from selected endpoint %s", id)
	}
	scenario.mu.Lock()
	scenario.negotiated[id]++
	scenario.mu.Unlock()
	return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: request.ProtocolVersion, Backend: request.Backend, Ready: true}, nil
}

func (*selectedModelsScenario) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	return nil
}
func (*selectedModelsScenario) Predict(_ context.Context, request models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	id, _, _ := strings.Cut(request.Prompt, " selected Work")
	return models.InvocationProtocolResponse{Text: id + " COMPLETE"}, nil
}
func (scenario *selectedModelsScenario) Do(request *http.Request) (*http.Response, error) {
	if request.URL.Path != "/health" {
		return nil, fmt.Errorf("unexpected model network/download request %s", request.URL)
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("ok")), Header: make(http.Header)}, nil
}

func testSelectedModelsAcquisition(t *testing.T, sessions factorysessions.Service, scenario *selectedModelsScenario) {
	t.Helper()
	request := scenario.request()
	request.RuntimeSelection.ModelCacheDirectory = filepath.Join(scenario.home, ".agent-factory", "models")
	startInitialOpeningSession(t, sessions, request)
	if err := selectedProviderInvoke(t.Context(), sessions, scenario.candidateID); err != nil {
		history := initialOpeningHistory(t, sessions, scenario.candidateID)
		data, _ := json.Marshal(history.History[len(history.History)-2:])
		t.Fatalf("%v; public history: %s", err, data)
	}
	candidateHistory := initialOpeningHistory(t, sessions, scenario.candidateID)
	peer := scenario.request()
	peer.SessionID, peer.FolderPath = scenario.peerID, scenario.peerDir
	peer.RuntimeSelection.DefinitionSourcePath = filepath.Join(scenario.peerDir, "factory.json")
	peer.RuntimeSelection.ExecutionBaseDir, peer.RuntimeSelection.RuntimeInstanceID = scenario.peerDir, uuid.NewString()
	peer.RuntimeSelection.SystemConfigHome = scenario.peerHome
	peer.RuntimeSelection.ModelCacheDirectory = filepath.Join(scenario.peerHome, ".agent-factory", "models")
	startInitialOpeningSession(t, sessions, peer)
	for _, id := range []string{scenario.peerID, scenario.candidateID} {
		if err := selectedProviderInvoke(t.Context(), sessions, id); err != nil {
			t.Fatal(err)
		}
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.candidateID, candidateHistory)
	for _, id := range []string{scenario.candidateID, scenario.peerID} {
		assertSelectedModelsObservations(t, sessions, id)
	}
	scenario.mu.Lock()
	for _, id := range []string{scenario.candidateID, scenario.peerID} {
		if scenario.starts[id] != 1 || scenario.negotiated[id] != 1 {
			t.Errorf("model selection %s starts=%d negotiations=%d, want one per distinct scope", id, scenario.starts[id], scenario.negotiated[id])
		}
	}
	scenario.mu.Unlock()
	peerHistory := initialOpeningHistory(t, sessions, scenario.peerID)
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	if err := selectedProviderInvoke(t.Context(), sessions, scenario.peerID); err != nil {
		t.Fatal(err)
	}
	assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
}

func assertSelectedModelsObservations(t *testing.T, sessions factorysessions.Service, id string) {
	t.Helper()
	assertSelectedModelsHistory(t, id, initialOpeningHistory(t, sessions, id))
}
func assertSelectedModelsHistory(t *testing.T, id string, history *factorydefinitions.FactoryEventStream) {
	t.Helper()
	seen := make(map[string]int)
	for _, event := range history.History {
		if event.Type != "MODEL_REQUEST" && event.Type != "MODEL_RESPONSE" {
			continue
		}
		if event.Context.SessionID == nil || *event.Context.SessionID != id || event.Context.DispatchID == nil || event.Context.RequestID == nil {
			t.Fatalf("model observation %s lost its session/dispatch/request attribution: %#v", event.Type, event.Context)
		}
		seen[string(event.Type)]++
	}
	if seen["MODEL_REQUEST"] == 0 || seen["MODEL_REQUEST"] != seen["MODEL_RESPONSE"] {
		t.Fatalf("session %s model observations = %v, want paired scoped request/result events", id, seen)
	}
}

// Each immutable specialization has one reusable process. Readiness polling is
// acknowledged separately from model-health eligibility; no global clock switch.

type selectedSchedulingTimer struct {
	clock    *selectedSchedulingClock
	duration time.Duration
	deadline time.Time
	channel  chan time.Time
	stopped  atomic.Bool
	fired    bool
}

func (timer *selectedSchedulingTimer) C() <-chan time.Time { return timer.channel }
func (timer *selectedSchedulingTimer) Stop() bool {
	timer.clock.mu.Lock()
	defer timer.clock.mu.Unlock()
	active := !timer.fired && !timer.stopped.Load()
	timer.stopped.Store(true)
	delete(timer.clock.pending, timer)
	return active
}

type selectedSchedulingClock struct {
	*platformclock.Deterministic
	created chan *selectedSchedulingTimer
	mu      sync.Mutex
	pending map[*selectedSchedulingTimer]struct{}
}

func (clock *selectedSchedulingClock) NewTimer(d time.Duration) platformclock.Timer {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	timer := &selectedSchedulingTimer{clock: clock, duration: d, deadline: clock.Now().Add(d), channel: make(chan time.Time, 1)}
	clock.pending[timer] = struct{}{}
	clock.created <- timer
	if d <= 0 {
		clock.fireLocked(timer)
	}
	return timer
}
func (clock *selectedSchedulingClock) fireLocked(timer *selectedSchedulingTimer) {
	if timer.fired || timer.stopped.Load() {
		return
	}
	timer.fired = true
	delete(clock.pending, timer)
	timer.channel <- clock.Now()
}
func (clock *selectedSchedulingClock) acknowledge(timer *selectedSchedulingTimer) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.fireLocked(timer)
}
func (clock *selectedSchedulingClock) SetTick(tick int) {
	clock.mu.Lock()
	defer clock.mu.Unlock()
	clock.Deterministic.SetTick(tick)
	for timer := range clock.pending {
		if !clock.Now().Before(timer.deadline) {
			clock.fireLocked(timer)
		}
	}
}
func (clock *selectedSchedulingClock) After(d time.Duration) <-chan time.Time {
	return clock.NewTimer(d).C()
}
func newSelectedSchedulingClock() *selectedSchedulingClock {
	return &selectedSchedulingClock{Deterministic: platformclock.NewDeterministic(time.Unix(0, 0), time.Millisecond), created: make(chan *selectedSchedulingTimer, 512), pending: make(map[*selectedSchedulingTimer]struct{})}
}
func selectedSchedulingReceive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(30 * time.Second):
		t.Fatal("selected scheduling observation missing")
		var zero T
		return zero
	}
}
func selectedSchedulingActivate(t *testing.T, clock *selectedSchedulingClock, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	for {
		select {
		case err := <-done:
			return err
		case timer := <-clock.created:
			// Readiness and invocation fallback polls are acknowledged independently
			// of eligibility; a pending startup timer is retained by its scenario.
			if timer.duration == 10*time.Millisecond || timer.duration == 250*time.Millisecond {
				clock.acknowledge(timer)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("selected activation missing")
			return context.DeadlineExceeded
		}
	}
}

type selectedSchedulingModels struct {
	*selectedModelsScenario
	health  atomic.Int32
	failed  chan int
	stopped chan string
}

func (scenario *selectedSchedulingModels) Start(ctx context.Context, spec serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	process, err := scenario.selectedModelsScenario.Start(ctx, spec)
	if err != nil {
		return nil, err
	}
	return &selectedSchedulingProcess{process: process, stopped: scenario.stopped}, nil
}

type selectedSchedulingProcess struct {
	process interface {
		HealthEndpoint() string
		Wait() error
		Stop(context.Context) error
	}
	stopped chan string
	once    sync.Once
}

func (p *selectedSchedulingProcess) HealthEndpoint() string { return p.process.HealthEndpoint() }
func (p *selectedSchedulingProcess) Wait() error            { return p.process.Wait() }
func (p *selectedSchedulingProcess) Stop(ctx context.Context) error {
	err := p.process.Stop(ctx)
	p.once.Do(func() { p.stopped <- p.HealthEndpoint() })
	return err
}
func TestSelectedSchedulingModelHealthAndCancellationPreservePeer(t *testing.T) {
	t.Parallel()
	for _, specialized := range []bool{false, true} {
		t.Run(fmt.Sprint(specialized), func(t *testing.T) {
			t.Parallel()
			scenario := &selectedSchedulingModels{selectedModelsScenario: newSelectedModelsScenario(t), failed: make(chan int, 2), stopped: make(chan string, 16)}
			facts := clockwork.NewFakeClockAt(time.Date(2041, 2, 3, 4, 5, 6, 0, time.UTC))
			scheduler := newSelectedSchedulingClock()
			modelClock := scheduler
			if specialized {
				modelClock = newSelectedSchedulingClock()
			}
			sessions := startSelectedSchedulingModels(t, scenario, facts, scheduler, modelClock)
			request := scenario.request()
			request.RuntimeSelection.ModelCacheDirectory = filepath.Join(scenario.home, ".agent-factory", "models")
			if err := selectedSchedulingActivate(t, scheduler, func() error { _, err := sessions.Start(t.Context(), request); return err }); err != nil {
				t.Fatal(err)
			}
			peer := scenario.request()
			peer.SessionID = scenario.peerID
			peer.FolderPath = scenario.peerDir
			peer.RuntimeSelection.DefinitionSourcePath = filepath.Join(scenario.peerDir, "factory.json")
			peer.RuntimeSelection.ExecutionBaseDir = scenario.peerDir
			peer.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
			peer.RuntimeSelection.SystemConfigHome = scenario.peerHome
			peer.RuntimeSelection.ModelCacheDirectory = filepath.Join(scenario.peerHome, ".agent-factory", "models")
			if err := selectedSchedulingActivate(t, scheduler, func() error { _, err := sessions.Start(t.Context(), peer); return err }); err != nil {
				t.Fatal(err)
			}
			if err := selectedSchedulingInvoke(t, scheduler, sessions, scenario.peerID); err != nil {
				t.Fatal(err)
			}
			cancelSelectedSchedulingModel(t, sessions, scenario, facts, scheduler, modelClock)
			request.SessionID = uuid.NewString()
			request.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
			if err := selectedSchedulingActivate(t, scheduler, func() error { _, err := sessions.Start(t.Context(), request); return err }); err != nil {
				t.Fatal(err)
			}
			if err := selectedSchedulingInvoke(t, scheduler, sessions, request.SessionID); err != nil {
				t.Fatal(err)
			}
			assertSelectedModelsHistory(t, request.SessionID, initialOpeningHistoryAt(t, sessions, request.SessionID, facts.Now()))
			for _, event := range initialOpeningHistoryAt(t, sessions, request.SessionID, facts.Now()).History {
				if event.Type == "MODEL_RESPONSE" && event.Context.EventTime != facts.Now() {
					t.Fatalf("model fact time %v want %v", event.Context.EventTime, facts.Now())
				}
			}

			closeInitialOpeningSession(t, sessions, request.SessionID)
			if err := selectedSchedulingInvoke(t, scheduler, sessions, scenario.peerID); err != nil {
				t.Fatal(err)
			}
			closeInitialOpeningSession(t, sessions, scenario.peerID)

		})
	}
}

type selectedSchedulingHostClock struct{ *selectedSchedulingClock }

func (clock selectedSchedulingHostClock) NewTimer(d time.Duration) interface {
	C() <-chan time.Time
	Stop() bool
} {
	return clock.selectedSchedulingClock.NewTimer(d)
}

func (scenario *selectedSchedulingModels) Negotiate(ctx context.Context, endpoint string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	result, err := scenario.selectedModelsScenario.Negotiate(ctx, endpoint, request)
	if endpoint == "http://"+scenario.candidateID+".invalid" && scenario.health.Add(1) <= 2 {
		scenario.failed <- int(scenario.health.Load())
		result.Ready = false
	}
	return result, err
}

func selectedSchedulingInvoke(t *testing.T, clock *selectedSchedulingClock, sessions factorysessions.Service, id string) error {
	t.Helper()
	return selectedSchedulingActivate(t, clock, func() error { return selectedProviderInvoke(t.Context(), sessions, id) })
}

func startSelectedSchedulingModels(t *testing.T, scenario *selectedSchedulingModels, facts *clockwork.FakeClock, scheduler, modelClock *selectedSchedulingClock) factorysessions.Service {
	t.Helper()
	api := support.NewProcessAPIServer()
	edges := serviceedges.Edges{Clock: facts, ProcessScheduler: scheduler, RecordingReadFile: os.ReadFile, APIServerStarter: api.Start,
		ModelAssetHTTPClient: scenario, ModelHostHTTPClient: scenario, ModelHostProcessLauncher: scenario, ModelHostProtocolNegotiator: scenario, ModelHostCompatibilityChecker: scenario, ModelInvocationProtocolClient: scenario,
		ModelAssetHostPlatform:          models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelResolveHuggingFaceRevision: func(context.Context, string) (string, error) { return selectedModelsRevision, nil },
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return scenario.backend, nil
		},
	}
	if modelClock != scheduler {
		edges.ModelHostClock = selectedSchedulingHostClock{modelClock}
	}
	process, err := root.BuildProcess(t.Context(), edges)
	if err != nil {
		t.Fatal(err)
	}
	support.CleanupProcess(t, process)
	if len(scheduler.created) != 0 || len(modelClock.created) != 0 {
		t.Fatal("construction scheduled timers")
	}
	hostDir := support.ScaffoldFactory(t, initialOpeningFactoryConfig())
	home := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{"you", "run", "--dir", hostDir, "--continuously", "--with-server", "--quiet", "--no-record"})
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	inputs.Input.WorkingDirectory = hostDir
	support.InitializeCustomerHomeWithProcess(t, process, inputs.Input.Env, hostDir)
	command := support.StartProcessCommand(t, process, inputs.Input)
	if err := selectedSchedulingActivate(t, scheduler, func() error { api.WaitForURL(t); return nil }); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { command.Stop(t) })
	return process.FactorySessions().FactorySessions().(factorysessions.Service)
}
func cancelSelectedSchedulingModel(t *testing.T, sessions factorysessions.Service, scenario *selectedSchedulingModels, facts *clockwork.FakeClock, scheduler, modelClock *selectedSchedulingClock) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- selectedProviderInvoke(ctx, sessions, scenario.candidateID) }()
	selectedSchedulingReceive(t, scenario.failed)
	var timer *selectedSchedulingTimer
	for timer == nil {
		candidate := selectedSchedulingReceive(t, modelClock.created)
		if candidate.duration == 100*time.Millisecond {
			timer = candidate
		}
	}
	facts.Advance(time.Millisecond)
	if modelClock != scheduler {
		scheduler.SetTick(1000)
	}
	if scenario.health.Load() != 1 {
		t.Fatal("unselected advancement retried health")
	}
	select {
	case err := <-done:
		t.Fatalf("model returned before selected health scheduling: %v", err)
	default:
	}
	// The selected source drives an actual second failed health action.
	modelClock.SetTick(int(modelClock.Now().Sub(time.Unix(0, 0))/time.Millisecond) + 100)
	selectedSchedulingReceive(t, scenario.failed)
	if !timer.stopped.Load() {
		t.Fatal("fired health timer was not retired")
	}
	for {
		next := selectedSchedulingReceive(t, modelClock.created)
		if next.duration == 100*time.Millisecond {
			timer = next
			break
		}
	}
	// Closing the selected scope cancels its owned load; caller cancellation alone preserves shared loading.
	cancel()
	closeInitialOpeningSession(t, sessions, scenario.candidateID)
	err := selectedSchedulingReceive(t, done)
	if err == nil {
		t.Fatal("cancelled model invocation succeeded")
	}
	selectedSchedulingReceive(t, scenario.stopped)
	if !timer.stopped.Load() {
		t.Fatal("cancelled model health timer remained active")
	}
}
