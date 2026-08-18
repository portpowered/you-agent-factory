package wire

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	platformbrowser "github.com/portpowered/infinite-you/pkg/platform/browser"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/webhooks"
)

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
	starter, err := provideAPIServerStarter(serviceedges.Edges{APIServerStarter: override})
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
	service := provideFactoryWebhooksService(serviceedges.Edges{
		FactoryWebhookSecretResolver: func(context.Context, factorydefinitions.LoadedFactorySource, string) (string, error) {
			return "redirect-secret", nil
		},
		FactoryWebhookDeadLetterAppender: func(_ string, line []byte) error {
			deadLetters <- append([]byte(nil), line...)
			return nil
		},
	}, logging.NoopLogger{})
	events := &wireWebhookEvents{event: wireWebhookEvent()}
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
		Events:         events,
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

// stubTransportRuntime captures the handler the bound transport component runs.
type stubTransportRuntime struct {
	factorysessionwire.ProcessRuntime
	handler http.Handler
	runs    int
}

func (runtime *stubTransportRuntime) RunTransport(_ context.Context, handler http.Handler) error {
	runtime.runs++
	runtime.handler = handler
	return nil
}

type stubVisualizationClock struct{}

func (stubVisualizationClock) Now() time.Time { return time.Unix(0, 0).UTC() }

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
		_ factoryvisualization.RuntimeReader,
		_ recordings.ProjectionService,
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

func openedRuntimeForAdapter(runtime *stubTransportRuntime) factorysessionwire.OpenedApplicationRuntime {
	return factorysessionwire.OpenedApplicationRuntime{
		Process:   runtime,
		Resources: factorysessionwire.RuntimeResources{Clock: stubVisualizationClock{}},
	}
}

// markerHandler is a comparable owner handler so a test can assert the exact
// handler instance the bound transport runs.
type markerHandler struct{}

func (*markerHandler) ServeHTTP(http.ResponseWriter, *http.Request) {}

func boundHandlerAdapter(handler http.Handler) httpRuntimeBinding {
	return func(factorysessionwire.OpenedApplicationRuntime) (http.Handler, error) { return handler, nil }
}

func TestProvideApplicationRuntimeAdapterOmitsTheVisualizationRoleWithoutASink(t *testing.T) {
	t.Parallel()

	build := &recordedVisualizationBuild{}
	handler := &markerHandler{}
	runtime := &stubTransportRuntime{}
	adapt, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{}, recordingVisualizationFactory(build), boundHandlerAdapter(handler), lifecycle.NewRunner,
	)
	if err != nil {
		t.Fatalf("provideApplicationRuntimeAdapter: %v", err)
	}

	components, err := adapt(openedRuntimeForAdapter(runtime), nil)
	if err != nil {
		t.Fatalf("adapt without a sink: %v", err)
	}
	if components.Visualization != nil {
		t.Fatal("visualization role was bound without a selected sink")
	}
	if build.calls != 0 {
		t.Fatalf("visualization factory calls = %d, want 0 without a sink", build.calls)
	}
	if components.Transport == nil {
		t.Fatal("transport component = nil, want the bound owner transport")
	}
	waiter, ok := components.Transport.(lifecycle.Waiter)
	if !ok {
		t.Fatal("transport component is not a Waiter; the lifecycle plan needs a blocking primary")
	}
	if err := components.Transport.Start(t.Context()); err != nil {
		t.Fatalf("transport start: %v", err)
	}
	if err := waiter.Wait(t.Context()); err != nil {
		t.Fatalf("transport wait: %v", err)
	}
	if runtime.runs != 1 || runtime.handler != handler {
		t.Fatalf("RunTransport calls = %d, bound handler matches = %t", runtime.runs, runtime.handler == handler)
	}
}

func TestProvideApplicationRuntimeAdapterBindsAnInertVisualizationRoleForASelectedSink(t *testing.T) {
	t.Parallel()

	build := &recordedVisualizationBuild{}
	sink := factoryvisualization.SinkFunc(func(factoryvisualization.View) {})
	adapt, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{},
		recordingVisualizationFactory(build),
		boundHandlerAdapter(&markerHandler{}),
		lifecycle.NewRunner,
	)
	if err != nil {
		t.Fatalf("provideApplicationRuntimeAdapter: %v", err)
	}

	components, err := adapt(openedRuntimeForAdapter(&stubTransportRuntime{}), sink)
	if err != nil {
		t.Fatalf("adapt with a sink: %v", err)
	}
	if build.calls != 1 {
		t.Fatalf("visualization factory calls = %d, want 1", build.calls)
	}
	if build.sink == nil || build.clock == nil {
		t.Fatalf("visualization inputs: sink bound = %t, clock bound = %t", build.sink != nil, build.clock != nil)
	}
	if components.Visualization == nil {
		t.Fatal("visualization role = nil, want a bound role for the selected sink")
	}
	if err := components.Visualization.Start(t.Context()); err != nil {
		t.Fatalf("visualization role start: %v", err)
	}
	if build.root.activations != 0 {
		t.Fatalf("Activate calls = %d, want 0; the role stays inert until explicit activation", build.root.activations)
	}
	if err := components.Visualization.Stop(t.Context()); err != nil {
		t.Fatalf("visualization role stop: %v", err)
	}
	if build.root.drains != 1 {
		t.Fatalf("StopDrain calls = %d, want exactly 1 on role shutdown", build.root.drains)
	}
}

func TestProvideApplicationRuntimeAdapterHonorsRootVisualizationEdgeOverrides(t *testing.T) {
	t.Parallel()

	build := &recordedVisualizationBuild{}
	fixed := factoryvisualization.SinkFunc(func(factoryvisualization.View) {})
	var observed factoryvisualization.Root
	adapt, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{
			FactoryVisualizationSink:         fixed,
			FactoryVisualizationRootObserver: func(root factoryvisualization.Root) { observed = root },
		},
		recordingVisualizationFactory(build),
		boundHandlerAdapter(&markerHandler{}),
		lifecycle.NewRunner,
	)
	if err != nil {
		t.Fatalf("provideApplicationRuntimeAdapter: %v", err)
	}

	components, err := adapt(openedRuntimeForAdapter(&stubTransportRuntime{}), nil)
	if err != nil {
		t.Fatalf("adapt with a fixed root sink: %v", err)
	}
	if components.Visualization == nil {
		t.Fatal("root FactoryVisualizationSink override did not bind a visualization role")
	}
	if build.root == nil || observed != factoryvisualization.Root(build.root) {
		t.Fatal("root FactoryVisualizationRootObserver did not receive the composed visualization root")
	}
}

func TestProvideApplicationRuntimeAdapterFailsClosedOnMissingOperations(t *testing.T) {
	t.Parallel()

	build := &recordedVisualizationBuild{}
	if _, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{}, nil, boundHandlerAdapter(&markerHandler{}), lifecycle.NewRunner,
	); err == nil {
		t.Fatal("missing visualization factory = nil error, want a construction failure")
	}
	if _, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{}, recordingVisualizationFactory(build), nil, lifecycle.NewRunner,
	); err == nil {
		t.Fatal("missing HTTP binding = nil error, want a construction failure")
	}
	if _, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{}, recordingVisualizationFactory(build), boundHandlerAdapter(&markerHandler{}), nil,
	); err == nil {
		t.Fatal("missing runner factory = nil error, want a construction failure")
	}

	bindingFailure := errors.New("owner HTTP binding failed")
	adapt, err := provideApplicationRuntimeAdapter(
		serviceedges.Edges{},
		recordingVisualizationFactory(build),
		func(factorysessionwire.OpenedApplicationRuntime) (http.Handler, error) { return nil, bindingFailure },
		lifecycle.NewRunner,
	)
	if err != nil {
		t.Fatalf("provideApplicationRuntimeAdapter: %v", err)
	}
	if _, err := adapt(openedRuntimeForAdapter(&stubTransportRuntime{}), nil); !errors.Is(err, bindingFailure) {
		t.Fatalf("adapt with a failing HTTP binding = %v, want the binding failure", err)
	}
}
