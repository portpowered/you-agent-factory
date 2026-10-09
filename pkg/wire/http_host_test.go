package wire

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
	webhookswire "github.com/portpowered/infinite-you/pkg/services/webhooks/wire"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

// fleetCatalogProjectionGate isolates the catalog dependency. Its full
// projection cannot finish until canceled; registry enumeration is immediately
// available, independently of that expensive phase.
type fleetCatalogProjectionGate struct {
	projectionEntered chan struct{}
	workersessions.ObservationService
}

func (g *fleetCatalogProjectionGate) ListFactorySessions(ctx context.Context) ([]factorysessions.ReadProjection, error) {
	close(g.projectionEntered)
	<-ctx.Done()
	return nil, ctx.Err()
}

func (g *fleetCatalogProjectionGate) ListLiveSessionIDs() []string {
	return []string{"00000000-0000-4000-8000-000000000001"}
}

func (g *fleetCatalogProjectionGate) WorkerSessionsObservationForSession(string) workersessions.ObservationService {
	return g.ObservationService
}

type fleetCatalogSessionSource struct{ workersessions.Service }

type fleetCatalogReady struct {
	source   workersessions.ObservationService
	ids      []string
	selected []string
	cancel   context.CancelFunc
}

func (g fleetCatalogReady) ListFactorySessions(context.Context) ([]factorysessions.ReadProjection, error) {
	return []factorysessions.ReadProjection{{Context: factorysessions.ProjectionContext{
		FactorySessionID: "00000000-0000-4000-8000-000000000001",
	}}}, nil
}

func (g fleetCatalogReady) ListLiveSessionIDs() []string {
	if g.ids != nil {
		return g.ids
	}
	return []string{"00000000-0000-4000-8000-000000000001"}
}

func (g *fleetCatalogReady) WorkerSessionsObservationForSession(id string) workersessions.ObservationService {
	g.selected = append(g.selected, id)
	if g.cancel != nil {
		g.cancel()
	}
	return g.source
}

func TestWorkerSessionFleetCatalogPreservesFactoryRegistry(t *testing.T) {
	t.Parallel()
	factorySource := &fleetCatalogSessionSource{}
	directSource := &fleetCatalogSessionSource{}
	got, err := workerSessionObservationSources(t.Context(), &fleetCatalogReady{source: factorySource}, directSource)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != factorySource || got[1] != directSource {
		t.Fatalf("catalog sources = %#v, want Factory registry followed by process registry", got)
	}
}

func TestWorkerSessionFleetCatalogOrdersUniqueSourcesAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		t.Run(fmt.Sprintf("cancel-during-selection-%t", canceled), func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			catalog := &fleetCatalogReady{ids: []string{" b ", "", "a", "b", " a "}, source: &fleetCatalogSessionSource{}}
			if canceled {
				catalog.cancel = cancel
			}
			got, err := workerSessionObservationSources(ctx, catalog, nil)
			if canceled {
				if !errors.Is(err, context.Canceled) || got != nil || len(catalog.selected) != 1 {
					t.Fatalf("canceled selection=(%#v, %v), calls=%v", got, err, catalog.selected)
				}
				return
			}
			if err != nil || len(got) != 2 || len(catalog.selected) != 2 || catalog.selected[0] != "a" || catalog.selected[1] != "b" {
				t.Fatalf("selection=(%#v, %v), ordered IDs=%v", got, err, catalog.selected)
			}
			cancel()
			got, err = workerSessionObservationSources(ctx, catalog, nil)
			if !errors.Is(err, context.Canceled) || got != nil || len(catalog.selected) != 2 {
				t.Fatalf("pre-canceled selection=(%#v, %v), calls=%v", got, err, catalog.selected)
			}
		})
	}
}

func TestWorkerSessionFleetActiveReadCausalRegression(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	gate := &fleetCatalogProjectionGate{projectionEntered: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, err := workerSessionObservationSources(ctx, gate, nil)
		result <- err
	}()
	select {
	case <-gate.projectionEntered:
		cancel()
		err := <-result
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("catalog cancellation = %v, want context.Canceled", err)
		}
		t.Fatal("fleet catalog entered full Factory Session projection before selecting available registry identities; Work-scoped reads bypass this catalog")
	case err := <-result:
		if err != nil {
			t.Fatalf("catalog enumeration = %v", err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		<-result
		t.Fatal("catalog did not enter either controlled phase")
	}
}

func TestProvideAPIServerStarterHonorsRootEdgeOverride(t *testing.T) {
	t.Parallel()

	called := false
	override := platformhttpserver.Starter(func(_ context.Context, request platformhttpserver.StartRequest) error {
		called = true
		if request.Port != 8123 || !request.AutoPort || request.Handler == nil {
			t.Fatalf("override request = %+v", request)
		}
		return nil
	})
	starter, err := provideAPIServerStarter(serviceedges.Edges{APIServerStarter: override}, nil)
	if err != nil {
		t.Fatalf("provideAPIServerStarter: %v", err)
	}
	if err := starter(t.Context(), platformhttpserver.StartRequest{
		Handler: http.NotFoundHandler(), Port: 8123, AutoPort: true,
	}); err != nil {
		t.Fatalf("starter override: %v", err)
	}
	if !called {
		t.Fatal("root APIServerStarter override was not selected")
	}
}

func TestProvideBrowserOpenerHonorsRootEdgeOverride(t *testing.T) {
	t.Parallel()

	called := false
	override := platformbrowser.Opener(func(context.Context, string) error {
		called = true
		return nil
	})
	selected := provideBrowserOpener(serviceedges.Edges{BrowserOpener: override})
	if err := selected(t.Context(), "https://factory.example"); err != nil || !called {
		t.Fatalf("browser override = (called %t, error %v)", called, err)
	}
	if provideBrowserOpener(serviceedges.Edges{}) == nil {
		t.Fatal("provideBrowserOpener default = nil, want host adapter")
	}

	for _, tc := range browserOpenerEnvironmentCases() {
		assertBrowserOpenerEnvironmentCase(t, tc)
	}

	for _, value := range []string{"", "1"} {
		assertInjectedBrowserOpener(t, value)
	}
}

type browserOpenerEnvironmentCase struct {
	name     string
	present  bool
	value    string
	wantNoOp bool
}

func browserOpenerEnvironmentCases() []browserOpenerEnvironmentCase {
	return []browserOpenerEnvironmentCase{
		{name: "missing"},
		{name: "empty", present: true},
		{name: "zero", present: true, value: "0"},
		{name: "true", present: true, value: "true"},
		{name: "whitespace", present: true, value: " 1"},
		{name: "exact one", present: true, value: "1", wantNoOp: true},
	}
}

func assertBrowserOpenerEnvironmentCase(t *testing.T, tc browserOpenerEnvironmentCase) {
	t.Helper()

	var hostFactoryCalls int
	fallbackCalled := false
	fallbackErr := errors.New("controlled browser fallback")
	fallback := platformbrowser.Opener(func(context.Context, string) error {
		fallbackCalled = true
		return fallbackErr
	})
	selected := provideBrowserOpenerWith(
		serviceedges.Edges{},
		func(string) (string, bool) { return tc.value, tc.present },
		func() platformbrowser.Opener {
			hostFactoryCalls++
			return fallback
		},
	)
	if selected == nil {
		t.Fatal("selected browser opener = nil")
	}

	err := selected(context.Background(), "https://factory.example")
	if tc.wantNoOp {
		if err != nil {
			t.Fatalf("opt-out opener error = %v, want nil", err)
		}
		if hostFactoryCalls != 0 {
			t.Fatalf("host factory calls = %d, want 0 under exact opt-out", hostFactoryCalls)
		}
		if fallbackCalled {
			t.Fatal("real fallback was called under exact opt-out")
		}
		return
	}
	if !errors.Is(err, fallbackErr) {
		t.Fatalf("fallback error = %v, want %v", err, fallbackErr)
	}
	if hostFactoryCalls != 1 {
		t.Fatalf("host factory calls = %d, want exactly 1", hostFactoryCalls)
	}
	if !fallbackCalled {
		t.Fatal("controlled real fallback was not called")
	}
}

func assertInjectedBrowserOpener(t *testing.T, value string) {
	t.Helper()

	injectedCalled := false
	injected := platformbrowser.Opener(func(context.Context, string) error {
		injectedCalled = true
		return nil
	})
	hostFactoryCalls := 0
	selected := provideBrowserOpenerWith(
		serviceedges.Edges{BrowserOpener: injected},
		func(string) (string, bool) { return value, true },
		func() platformbrowser.Opener {
			hostFactoryCalls++
			return func(context.Context, string) error {
				return errors.New("host fallback must not be selected")
			}
		},
	)
	if err := selected(context.Background(), "https://factory.example"); err != nil {
		t.Fatalf("injected opener error = %v", err)
	}
	if !injectedCalled {
		t.Fatal("injected opener was not called")
	}
	if hostFactoryCalls != 0 {
		t.Fatalf("host factory calls = %d, want 0 for explicit injection", hostFactoryCalls)
	}
}

func TestFactoryWebhooksDefaultClientDoesNotFollowRedirects(t *testing.T) {
	var targetMu sync.Mutex
	targetRequests := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetMu.Lock()
		targetRequests++
		targetMu.Unlock()
	}))
	defer target.Close()

	configuredRequests := make(chan struct{}, 1)
	configured := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		configuredRequests <- struct{}{}
		writer.Header().Set("Location", target.URL+"/redirect-target")
		writer.WriteHeader(http.StatusTemporaryRedirect)
	}))
	defer configured.Close()

	deadLetters := make(chan []byte, 1)
	events := &wireWebhookEvents{event: wireWebhookEvent()}
	selected := serviceedges.Edges{
		Clock: platformclock.Real{}, ProcessScheduler: platformclock.Real{},
		FactoryWebhookSecretResolver: func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			return "redirect-secret", nil
		},
		FactoryWebhookDeadLetterAppender: func(_ string, line []byte) error {
			deadLetters <- append([]byte(nil), line...)
			return nil
		},
	}
	service := webhookswire.NewService(events, provideFactoryWebhookHTTPClient(selected), provideFactoryWebhookSecretResolver(selected), provideFactoryWebhookClock(selected), provideFactoryWebhookDeadLetterAppender(selected), logging.NoopLogger{})
	subscription, err := service.Start(context.Background(), webhooks.StartRequest{
		Definitions: []factorydefinitions.FactoryWebhookConfig{{
			Name:             "redirect-endpoint",
			Enabled:          true,
			URL:              configured.URL + "/configured",
			SigningSecretRef: "secrets/redirect",
			Filter: factorydefinitions.FactoryWebhookFilterConfig{
				EventTypes: []string{factorydefinitions.FactoryWebhookEventTypeWorkStateChange},
			},
		}},
		RuntimeSource:  wireLoadedFactorySource{},
		DeadLetterPath: "runtime/dead-letter.jsonl",
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer func() {
		if err := subscription(context.Background()); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()
	if _, ok := <-configuredRequests; !ok {
		t.Fatal("configured endpoint did not receive the request")
	}
	select {
	case <-configuredRequests:
		t.Fatal("configured endpoint received more than one request")
	default:
	}
	line := receiveWireDeadLetter(t, deadLetters)
	var record struct {
		AttemptCount   int    `json:"attemptCount"`
		StatusCode     int    `json:"statusCode"`
		TerminalReason string `json:"terminalReason"`
	}
	if err := json.Unmarshal(line, &record); err != nil {
		t.Fatalf("decode dead-letter record: %v", err)
	}
	if record.AttemptCount != 1 || record.StatusCode != http.StatusTemporaryRedirect || record.TerminalReason != "non_retryable_http_status" {
		t.Fatalf("redirect dead-letter = %#v, want one terminal 307 attempt", record)
	}
	targetMu.Lock()
	defer targetMu.Unlock()
	if targetRequests != 0 {
		t.Fatalf("redirect target received %d requests, want none", targetRequests)
	}
}

func receiveWireDeadLetter(t *testing.T, records <-chan []byte) []byte {
	t.Helper()
	select {
	case record := <-records:
		return record
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for webhook dead-letter record")
		return nil
	}
}

type wireWebhookEvents struct {
	recordings.Service
	event recordings.CanonicalEvent
}

func (events *wireWebhookEvents) SubscribeFrom(context.Context, recordings.SubscribeRequest) (recordings.SubscribeResult, error) {
	outcomes := make(chan recordings.SubscriptionOutcome, 1)
	outcomes <- recordings.SubscriptionOutcome{Kind: recordings.SubscriptionEvent, Event: events.event}
	return recordings.SubscribeResult{Subscription: func(ctx context.Context) recordings.SubscriptionOutcome {
		select {
		case outcome := <-outcomes:
			return outcome
		case <-ctx.Done():
			return recordings.SubscriptionOutcome{Kind: recordings.SubscriptionClosed}
		}
	}}, nil
}

type wireLoadedFactorySource struct {
	factorydefinitions.RuntimeDefinitionLookup
}

func (wireLoadedFactorySource) FactoryDir() string { return "/factories/test" }
func (wireLoadedFactorySource) FactoryConfig() *factorydefinitions.FactoryConfig {
	return &factorydefinitions.FactoryConfig{}
}
func (wireLoadedFactorySource) RuntimeBaseDir() string { return "/runtime/test" }

func wireWebhookEvent() recordings.CanonicalEvent {
	return recordings.CanonicalEvent{
		ID:         "wire-event-1",
		Sequence:   1,
		RecordedAt: time.Date(2026, time.August, 11, 9, 0, 0, 0, time.UTC),
		Kind:       recordings.CanonicalEventKind(factorydefinitions.FactoryWebhookEventTypeWorkStateChange),
		Payload:    `{"workId":"work-1","fromState":"queued","toState":"done"}`,
	}
}

var _ factorydefinitions.LoadedFactorySource = wireLoadedFactorySource{}
var _ recordings.Service = (*wireWebhookEvents)(nil)

// stubVisualizationRoot records the lifecycle calls the composed visualization
// role makes on the constructed Factory Visualization root. Unimplemented root
// methods stay nil so any unexpected call fails loudly.
type stubVisualizationRoot struct {
	factoryvisualization.Service
	activations int
	drains      int
}

func (root *stubVisualizationRoot) Activate(
	context.Context,
	factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	root.activations++
	return factoryvisualization.ActivateResult{}, nil
}

func (root *stubVisualizationRoot) StopDrain(
	context.Context,
	factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	root.drains++
	return factoryvisualization.StopDrainResult{}, nil
}

// recordedVisualizationBuild is one observed call into the injected Factory
// Visualization runtime factory.
type recordedVisualizationBuild struct {
	calls int
	clock factoryvisualization.Clock
	sink  factoryvisualization.Sink
	root  *stubVisualizationRoot
}

func recordingVisualizationFactory(build *recordedVisualizationBuild) factoryvisualization.RuntimeFactory {
	return func(
		_ string,
		clock factoryvisualization.Clock,
		sink factoryvisualization.Sink,
		_ factoryvisualization.ErrorReporter,
	) (factoryvisualization.Service, error) {
		build.calls++
		build.clock = clock
		build.sink = sink
		build.root = &stubVisualizationRoot{}
		return build.root, nil
	}
}

// stubSinkOwner is the composition-root sink registry.
type stubSinkOwner struct {
	factoryvisualization.RuntimeSinkOwner
	sinks map[factoryvisualization.RuntimeSinkID]factoryvisualization.Sink
}

func (owner stubSinkOwner) RuntimeSink(
	id factoryvisualization.RuntimeSinkID,
) (factoryvisualization.Sink, bool) {
	sink, ok := owner.sinks[id]
	return sink, ok
}

// markerHandler is a comparable owner handler so a test can assert the exact
// handler instance the bound transport runs.
type markerHandler struct{}

func (*markerHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func boundHandlerAdapter(handler http.Handler) httpRuntimeBinding {
	return func(string, initializer.InvocationCancellation) (http.Handler, error) {
		return handler, nil
	}
}

func stubRunRuntimePlanLifecycle(
	factorysessionwire.LifecyclePlanRequest,
) (lifecycle.Plan, error) {
	return lifecycle.Plan{}, nil
}

func TestProvideRunRuntimeRunnerBuilderRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	factory := recordingVisualizationFactory(&recordedVisualizationBuild{})
	binding := boundHandlerAdapter(&markerHandler{})
	missing := []struct {
		name    string
		root    *factorysessionwire.Root
		factory factoryvisualization.RuntimeFactory
		sinks   factoryvisualization.RuntimeSinkOwner
		binding httpRuntimeBinding
		runner  lifecycle.RunnerFactory
		plan    factorysessionwire.LifecyclePlanOperation
	}{
		{"process root", nil, factory, stubSinkOwner{}, binding, lifecycle.NewRunner, stubRunRuntimePlanLifecycle},
		{"visualization factory", &factorysessionwire.Root{}, nil, stubSinkOwner{}, binding, lifecycle.NewRunner, stubRunRuntimePlanLifecycle},
		{"visualization sink owner", &factorysessionwire.Root{}, factory, nil, binding, lifecycle.NewRunner, stubRunRuntimePlanLifecycle},
		{"HTTP binding", &factorysessionwire.Root{}, factory, stubSinkOwner{}, nil, lifecycle.NewRunner, stubRunRuntimePlanLifecycle},
		{"runner factory", &factorysessionwire.Root{}, factory, stubSinkOwner{}, binding, nil, stubRunRuntimePlanLifecycle},
		{"lifecycle plan operation", &factorysessionwire.Root{}, factory, stubSinkOwner{}, binding, lifecycle.NewRunner, nil},
	}
	for _, operation := range missing {
		if _, err := provideRunRuntimeRunnerBuilder(
			operation.root, serviceedges.Edges{}, operation.factory, operation.sinks, operation.binding, operation.runner, operation.plan,
		); err == nil {
			t.Fatalf("missing %s = nil error, want a construction failure", operation.name)
		}
	}
}

type defaultWorkTypePolicy struct {
	id          string
	err         error
	factoryName string
}

func (policy defaultWorkTypePolicy) DefaultWorkType(config *factorydefinitions.FactoryConfig) (string, error) {
	if policy.factoryName != "" && (config == nil || config.Name != policy.factoryName) {
		return "", errors.New("selected Factory configuration is missing")
	}
	return policy.id, policy.err
}

type currentFactorySessionRead struct {
	factorysessions.LiveControlService
	config *factorydefinitions.FactoryConfig
	err    error
}

func (api currentFactorySessionRead) GetFactorySession(
	context.Context,
	string,
) (factorysessions.LiveControlSnapshot, error) {
	return factorysessions.LiveControlSnapshot{Context: factorysessions.ProjectionContext{FactoryCfg: api.config}}, api.err
}

// TestDefaultWorkTypeResolverPreservesSessionAdmissionPolicy pins the
// omitted-work-type policy the Work HTTP adapter is composed with: a missing
// collaborator, an unknown session, an unset current Factory, or a policy that
// cannot name a default all resolve to the empty work type rather than failing
// admission, while an opaque Factory Session projection failure is surfaced verbatim.
func TestDefaultWorkTypeResolverPreservesSessionAdmissionPolicy(t *testing.T) {
	t.Parallel()

	checks := []struct {
		name       string
		sessions   factorysessions.LiveControlService
		invocation factorydefinitions.InvocationWorkTypeService
		want       string
		wantErr    string
	}{
		{name: "missing dependencies"},
		{name: "missing invocation policy", sessions: currentFactorySessionRead{}},
		// Both not-found rows supply an invocation policy that would name a
		// work type, so reaching the empty result proves the not-found
		// fallback rather than the missing-collaborator short circuit above.
		{
			name:       "session not found",
			sessions:   currentFactorySessionRead{err: apisurface.ErrFactorySessionNotFound},
			invocation: defaultWorkTypePolicy{id: "default-task"},
		},
		{
			name:       "current factory not found",
			sessions:   currentFactorySessionRead{err: apisurface.ErrCurrentFactoryNotFound},
			invocation: defaultWorkTypePolicy{id: "default-task"},
		},
		{
			name:       "opaque definition error",
			sessions:   currentFactorySessionRead{err: errors.New("definition failed")},
			invocation: defaultWorkTypePolicy{},
			wantErr:    "definition failed",
		},
		{
			name:       "invocation policy error",
			sessions:   currentFactorySessionRead{},
			invocation: defaultWorkTypePolicy{err: errors.New("policy failed")},
		},
		{
			name:       "default type",
			sessions:   currentFactorySessionRead{config: &factorydefinitions.FactoryConfig{Name: "selected-factory"}},
			invocation: defaultWorkTypePolicy{id: "default-task", factoryName: "selected-factory"},
			want:       "default-task",
		},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			resolver := newDefaultWorkTypeResolver(check.sessions, check.invocation)
			got, err := resolver(context.Background(), "session-alpha")
			if check.wantErr != "" {
				if err == nil || err.Error() != check.wantErr {
					t.Fatalf("error = %v, want %q", err, check.wantErr)
				}
				return
			}
			if err != nil || got != check.want {
				t.Fatalf("resolver = (%q, %v), want (%q, nil)", got, err, check.want)
			}
		})
	}
}

type workerSessionControlRouterServiceFake struct {
	workersessions.Service
	id           string
	controlCalls int
}

func (service *workerSessionControlRouterServiceFake) Get(
	_ context.Context,
	request workersessions.GetRequest,
) (workersessions.Session, error) {
	if request.ID != service.id {
		return workersessions.Session{}, workersessions.ErrSessionNotFound
	}
	return workersessions.Session{ID: service.id, State: workersessions.StateRunning}, nil
}

func (service *workerSessionControlRouterServiceFake) Cancel(
	_ context.Context,
	request workersessions.ControlRequest,
) (workersessions.ControlResult, error) {
	service.controlCalls++
	return workersessions.ControlResult{
		Session: workersessions.Session{ID: request.ID, State: workersessions.StateCanceled},
		Action:  workersessions.ControlActionCancel,
		Outcome: workersessions.ControlOutcomeApplied,
	}, nil
}

func TestWorkerSessionControlRouterUsesExactIdentityOwner(t *testing.T) {
	firstRuntime := &workerSessionControlRouterServiceFake{id: "another-session"}
	owner := &workerSessionControlRouterServiceFake{id: "target-session"}
	processDefault := &workerSessionControlRouterServiceFake{id: "target-session"}
	router := workerSessionControlRouter{sources: func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{firstRuntime, owner, processDefault}, nil
	}}

	result, err := router.Cancel(context.Background(), workersessions.ControlRequest{ID: "target-session"})
	if err != nil {
		t.Fatalf("Cancel() error = %v, want exact owner control", err)
	}
	if result.Session.ID != "target-session" || result.Session.State != workersessions.StateCanceled || result.Outcome != workersessions.ControlOutcomeApplied {
		t.Fatalf("Cancel() = %#v, want APPLIED CANCELED target session", result)
	}
	if firstRuntime.controlCalls != 0 || owner.controlCalls != 1 || processDefault.controlCalls != 0 {
		t.Fatalf("control calls = first %d, owner %d, process default %d; want only exact owner", firstRuntime.controlCalls, owner.controlCalls, processDefault.controlCalls)
	}
}

func TestWorkerSessionControlRouterKeepsUnknownIdentityNotFound(t *testing.T) {
	owner := &workerSessionControlRouterServiceFake{id: "known-session"}
	router := workerSessionControlRouter{sources: func(context.Context) ([]workersessions.Service, error) {
		return []workersessions.Service{owner}, nil
	}}

	_, err := router.Cancel(context.Background(), workersessions.ControlRequest{ID: "unknown-session"})
	if !errors.Is(err, workersessions.ErrSessionNotFound) {
		t.Fatalf("Cancel(unknown) error = %v, want ErrSessionNotFound", err)
	}
	if owner.controlCalls != 0 {
		t.Fatalf("unknown identity caused %d control calls, want zero", owner.controlCalls)
	}
}

func TestDefaultBrowserOpenerNeverLaunchesRealBrowserInTestBinary(t *testing.T) {
	t.Parallel()

	if !testing.Testing() {
		t.Fatal("testing.Testing() = false inside a test binary")
	}
	// Harness path: process wiring with no BrowserOpener edge and no opt-out.
	selected := provideBrowserOpenerWith(
		serviceedges.Edges{},
		func(string) (string, bool) { return "", false },
		hostBrowserOpener,
	)
	if err := selected(context.Background(), "http://localhost:7437/dashboard/ui"); !errors.Is(err, errBrowserLaunchInTestBinary) {
		t.Fatalf("default opener error = %v, want %v", err, errBrowserLaunchInTestBinary)
	}
	if err := provideBrowserOpener(serviceedges.Edges{})(context.Background(), "http://x"); !errors.Is(err, errBrowserLaunchInTestBinary) {
		t.Fatalf("provideBrowserOpener default error = %v, want refusal", err)
	}
	// Production selection (not a test binary) still yields the real host adapter.
	if hostBrowserOpenerFor(false, "windows") == nil {
		t.Fatal("production opener = nil")
	}
	// An injected fake still records the open.
	var opened []string
	fake := platformbrowser.Opener(func(_ context.Context, u string) error { opened = append(opened, u); return nil })
	if err := provideBrowserOpener(serviceedges.Edges{BrowserOpener: fake})(context.Background(), "http://dash"); err != nil || len(opened) != 1 {
		t.Fatalf("fake opener = (%v, %v)", opened, err)
	}
}
