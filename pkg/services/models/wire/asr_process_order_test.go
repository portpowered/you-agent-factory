package wire

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	platformgrpc "github.com/portpowered/infinite-you/pkg/platform/grpc"
	"github.com/portpowered/infinite-you/pkg/services/models"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	modelseffects "github.com/portpowered/infinite-you/pkg/services/models/internal/effects"
	scopedassets "github.com/portpowered/infinite-you/pkg/services/models/internal/services/assets"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/wire"
	inference "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference"
	inferencewire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/inference/wire"
	runtimehost "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host"
	runtimehostwire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_host/wire"
	runtimescopeswire "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes/wire"
	"google.golang.org/protobuf/proto"
)

func TestASRProcessOrderChangesLeaseOutcomeAfterManagedChildExit(t *testing.T) {
	t.Parallel()

	var schedules = make(map[string][]modelseffects.ASRLiveCorrelationEvent, 2)
	for _, test := range []struct {
		name          string
		responseFirst bool
	}{
		{name: "response completes before child exit", responseFirst: true},
		{name: "child exit revokes lease before response", responseFirst: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			schedules[test.name] = runControlledASROrder(t, test.responseFirst)
		})
	}
	responseFirst := correlationEvent(t, schedules["response completes before child exit"], modelseffects.ASRLiveCorrelationRPCTerminal)
	exitFirst := correlationEvent(t, schedules["child exit revokes lease before response"], modelseffects.ASRLiveCorrelationRPCTerminal)
	if responseFirst.RequestSemanticSHA256 != exitFirst.RequestSemanticSHA256 ||
		responseFirst.ResponseSemanticSHA256 != exitFirst.ResponseSemanticSHA256 {
		t.Fatalf("semantic digests changed across schedules: response-first=%#v exit-first=%#v", responseFirst, exitFirst)
	}
}

func TestASRProcessOrderCancellationCleansOwnedResourcesOnce(t *testing.T) {
	t.Parallel()
	scenario := newControlledASRScenario(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request := models.EnsureModelHostRequest{Scope: scenario.scope, Name: models.BuiltInModelNameASR}
	if _, err := scenario.host.EnsureModelHost(ctx, request); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	scenario.process = scenario.launcher.process()
	if scenario.process == nil {
		t.Fatal("managed LocalAI process was not started")
	}
	lease, err := scenario.host.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope: scenario.scope, Name: models.BuiltInModelNameASR, Holder: "asr-worker",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}
	resultChannel := make(chan controlledASRInvocation, 1)
	go func() {
		result, invokeErr := scenario.inference.InvokeModelWithLease(
			modelseffects.WithASRLiveCorrelation(
				modelseffects.WithRuntimeObservation(ctx, scenario.recorder, scenario.protocol.configuration()),
				scenario.correlation,
			),
			controlledASRRequest(scenario.scope, lease.Lease.Lease, scenario.audio),
		)
		resultChannel <- controlledASRInvocation{result: result, err: invokeErr}
	}()
	awaitRPCStart(t, scenario.connection.started, resultChannel)
	assertControlledASRRequest(t, scenario.connection, scenario.tempDirectory, scenario.audio)
	cancel()
	invocation := awaitInvocation(t, resultChannel)
	if invocation.err == nil || !errors.Is(invocation.err, models.ErrInferenceCancelled) ||
		invocation.result.Status != models.ModelInvocationStatusCancelled ||
		invocation.result.LeaseDisposition != models.InvocationLeaseReleased ||
		len(invocation.result.Outputs) != 0 || len(invocation.result.Content) != 0 {
		t.Fatalf("cancelled ASR invocation = result:%#v error:%v, want typed cancellation and no outputs", invocation.result, invocation.err)
	}
	assertASRStagingRemoved(t, scenario.tempDirectory)
	assertASRConnectionClosed(t, scenario.connection)
	assertLeaseStatus(t, scenario.host, scenario.scope, lease.Lease.Lease, models.ModelLeaseStatusReleased)
	scenario.process.exit(errors.New("controlled child exit after cancelled invocation"))
	awaitCorrelationSignal(t, scenario.correlation, modelseffects.ASRLiveCorrelationChildWaited)
	awaitCorrelationSignal(t, scenario.correlation, modelseffects.ASRLiveCorrelationHostFailureSeen)
	if scenario.process.waitCount() != 1 || scenario.connection.closeCount() != 1 || scenario.connection.dialCount() != 1 {
		t.Fatalf("cancelled ASR owned cleanup Wait=%d close=%d dial=%d, want one Wait, one close and one dial", scenario.process.waitCount(), scenario.connection.closeCount(), scenario.connection.dialCount())
	}
	events := scenario.correlation.Snapshot()
	want := []modelseffects.ASRLiveCorrelationEventKind{
		modelseffects.ASRLiveCorrelationChildStarted,
		modelseffects.ASRLiveCorrelationEndpointObserved,
		modelseffects.ASRLiveCorrelationChildWaited,
		modelseffects.ASRLiveCorrelationHostFailureSeen,
	}
	if len(events) != len(want) {
		t.Fatalf("cancelled ASR correlation events = %#v, want %d signals without a decoded terminal", events, len(want))
	}
	for index, event := range events {
		if event.Sequence != uint64(index+1) || event.Kind != want[index] {
			t.Fatalf("cancelled ASR event[%d] = %#v, want sequence=%d kind=%s", index, event, index+1, want[index])
		}
	}
}

func runControlledASROrder(t *testing.T, responseFirst bool) []modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	scenario := newControlledASRScenario(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	request := models.EnsureModelHostRequest{Scope: scenario.scope, Name: models.BuiltInModelNameASR}
	if _, err := scenario.host.EnsureModelHost(ctx, request); err != nil {
		t.Fatalf("EnsureModelHost: %v", err)
	}
	scenario.process = scenario.launcher.process()
	if scenario.process == nil {
		t.Fatal("managed LocalAI process was not started")
	}
	lease, err := scenario.host.AcquireModelLease(ctx, models.AcquireModelLeaseRequest{
		Scope: scenario.scope, Name: models.BuiltInModelNameASR, Holder: "asr-worker",
	})
	if err != nil {
		t.Fatalf("AcquireModelLease: %v", err)
	}

	configuration := scenario.protocol.configuration()
	resultChannel := make(chan controlledASRInvocation, 1)
	go func() {
		result, invokeErr := scenario.inference.InvokeModelWithLease(
			modelseffects.WithASRLiveCorrelation(
				modelseffects.WithRuntimeObservation(ctx, scenario.recorder, configuration),
				scenario.correlation,
			),
			controlledASRRequest(scenario.scope, lease.Lease.Lease, scenario.audio),
		)
		resultChannel <- controlledASRInvocation{result: result, err: invokeErr}
	}()
	awaitRPCStart(t, scenario.connection.started, resultChannel)
	assertControlledASRRequest(t, scenario.connection, scenario.tempDirectory, scenario.audio)

	scenario.connection.allowResponse()
	awaitCorrelationSignal(t, scenario.correlation, modelseffects.ASRLiveCorrelationRPCTerminal)
	if responseFirst {
		if err := scenario.correlation.ReleaseResponse(); err != nil {
			t.Fatalf("release response-first ASR result: %v", err)
		}
		invocation := awaitInvocation(t, resultChannel)
		assertCompletedASRInvocation(t, invocation)
		assertLeaseStatus(t, scenario.host, scenario.scope, lease.Lease.Lease, models.ModelLeaseStatusReleased)
		scenario.process.exit(errors.New("controlled managed child exit"))
	} else {
		scenario.process.exit(errors.New("controlled managed child exit"))
		awaitCorrelationSignal(t, scenario.correlation, modelseffects.ASRLiveCorrelationChildWaited)
		awaitCorrelationSignal(t, scenario.correlation, modelseffects.ASRLiveCorrelationHostFailureSeen)
		assertLeaseStatus(t, scenario.host, scenario.scope, lease.Lease.Lease, models.ModelLeaseStatusExpired)
		if err := scenario.correlation.ReleaseResponse(); err != nil {
			t.Fatalf("release exit-first ASR result: %v", err)
		}
		invocation := awaitInvocation(t, resultChannel)
		assertExpiredLeaseASRInvocation(t, invocation)
	}

	awaitSignal(t, scenario.recorderSink.crashed, "managed child crash evidence")
	awaitSignal(t, scenario.process.waited, "managed child Wait completion")
	awaitCorrelationSignal(t, scenario.correlation, modelseffects.ASRLiveCorrelationHostFailureSeen)
	assertASRStagingRemoved(t, scenario.tempDirectory)
	assertASRConnectionClosed(t, scenario.connection)
	assertSharedRuntimeEvidence(t, scenario.recorderSink.snapshot(), responseFirst, scenario.tempDirectory)
	if scenario.process.waitCount() != 1 || scenario.connection.closeCount() != 1 {
		t.Fatalf("owned cleanup counts Wait=%d connection close=%d, want one each", scenario.process.waitCount(), scenario.connection.closeCount())
	}
	events := scenario.correlation.Snapshot()
	assertASRLiveCorrelationOrder(t, events, responseFirst)
	return events
}

func newControlledASRScenario(t *testing.T) *controlledASRScenario {
	t.Helper()
	config := controlledASRRuntimeConfig()
	scopes, err := runtimescopeswire.NewService(func() string { return "causal-asr-test" })
	if err != nil {
		t.Fatalf("construct Runtime Scopes: %v", err)
	}
	privateScope, err := scopes.Open(models.RuntimeBinding{
		CacheDirectory: t.TempDir(),
		RuntimeConfig:  func() *models.RuntimeConfig { return &config },
	})
	if err != nil {
		t.Fatalf("open Runtime Scope: %v", err)
	}
	scope, err := (models.RuntimeScopeRef{}).Parse(string(privateScope))
	if err != nil {
		t.Fatalf("parse Runtime Scope: %v", err)
	}

	cachePath := t.TempDir()
	tempDirectory := t.TempDir()
	assets := &controlledASRAssets{inspection: scopedassets.RuntimeCacheInspection{
		Supported: true, Installed: true, Revision: "controlled-asr-cache-r1",
		CachePath: cachePath,
	}}
	clock := &controlledASRClock{}
	response, err := proto.Marshal(&localai.TranscriptResult{
		Text: "private transcript marker",
		Segments: []*localai.TranscriptSegment{{
			Id: 0, Start: 0, End: 500_000_000, Text: "private transcript marker",
		}},
	})
	if err != nil {
		t.Fatalf("marshal controlled ASR response: %v", err)
	}
	connection := &controlledASRConnection{
		started: make(chan struct{}, 1), responseReady: make(chan struct{}),
		response: response,
	}
	correlation, err := modelseffects.NewASRLiveCorrelationController(
		func(_ context.Context, host string, port int) (int, error) {
			if host != "127.0.0.1" || port != 45906 {
				return 0, modelseffects.ErrASRLiveCorrelationOwnership
			}
			return controlledASRChildPID, nil
		},
	)
	if err != nil {
		t.Fatalf("construct ASR live-correlation controller: %v", err)
	}
	launcher := &controlledASRLauncher{correlation: correlation}
	protocol := &controlledASRProtocol{}
	sink := &controlledASREvidenceSink{crashed: make(chan struct{}, 1)}
	recorder := modelseffects.NewOrderedRuntimeEvidenceRecorder(sink)
	host, err := runtimehostwire.NewService(
		scopes, assets, launcher, controlledASRNoHTTP{}, clock, nil, nil,
		runtimehost.Options{
			Platform:           models.AssetHostPlatform{OperatingSystem: "windows", Architecture: "amd64"},
			ProtocolNegotiator: protocol, CompatibilityChecker: controlledASRCompatibility{},
			RuntimeEvidence: recorder, IdleUnloadAfter: time.Hour,
		},
	)
	if err != nil {
		t.Fatalf("construct Runtime Host: %v", err)
	}

	invocation, err := inferenceRuntime(invocationRuntimeOptions{
		Dialer:           controlledASRDialer{connection: connection},
		ASRTempDirectory: func() string { return tempDirectory },
		ASRCreateTemp: func(directory, pattern string) (localai.TempFile, error) {
			return os.CreateTemp(directory, pattern)
		},
		ASRWriteFile: func(path string, content []byte) error {
			return os.WriteFile(path, content, 0o600)
		},
		ASRRemoveFile: os.Remove,
	})
	if err != nil {
		t.Fatalf("construct Models invocation runtime: %v", err)
	}
	catalog, err := catalogwire.NewService(scopes)
	if err != nil {
		t.Fatalf("construct Models Catalog: %v", err)
	}
	inferenceService, err := inferencewire.NewService(
		scopes, assets, catalog, host, invocation, inference.InertArtifactFileSystem{}, time.Now,
	)
	if err != nil {
		t.Fatalf("construct Models Inference: %v", err)
	}
	t.Cleanup(func() {
		connection.allowResponse()
		if launcher.process() != nil {
			launcher.process().exit(errors.New("controlled test cleanup"))
		}
		if shutdown, ok := host.(interface{ Shutdown(context.Context) error }); ok {
			_ = shutdown.Shutdown(context.Background())
		}
	})
	return &controlledASRScenario{
		scope: scope, host: host, inference: inferenceService,
		launcher: launcher, process: nil, protocol: protocol, connection: connection,
		tempDirectory: tempDirectory, audio: []byte("private audio marker"), correlation: correlation,
		recorder: recorder, recorderSink: sink,
	}
}

func controlledASRRuntimeConfig() models.RuntimeConfig {
	return models.RuntimeConfig{Workers: []models.RuntimeWorker{{
		Name: "asr-worker", Type: models.RuntimeWorkerTypeInference,
		Model: models.BuiltInModelNameASR, ModelLocality: models.RuntimeModelLocalityLocal,
		Command:    "fake-localai",
		Args:       []string{"--grpc-endpoint", "grpc://127.0.0.1:45906"},
		Operations: []models.RuntimeOperation{{Name: models.OperationASR}},
	}}}
}

func controlledASRRequest(
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
	audio []byte,
) models.InvokeModelRequest {
	return models.InvokeModelRequest{
		Scope: scope, Lease: lease, Holder: "asr-worker", ModelName: models.BuiltInModelNameASR,
		Model:     models.ModelReference{NameOrURI: models.BuiltInModelNameASR},
		Operation: models.OperationASR, Offline: true,
		Inputs: []models.InferenceInput{{
			Name: "audio", Modality: models.ModalityAudio, ContentType: "audio/wav",
			MediaType: "audio/wav", Content: string(audio),
		}},
	}
}

func assertControlledASRRequest(
	t *testing.T,
	connection *controlledASRConnection,
	stagingDirectory string,
	audio []byte,
) {
	t.Helper()
	method, endpoint, payload := connection.request()
	request := &localai.TranscriptRequest{}
	if err := proto.Unmarshal(payload, request); err != nil {
		t.Fatalf("decode AudioTranscription request: %v", err)
	}
	stagedAudio, err := os.ReadFile(request.GetDst())
	if err != nil {
		t.Fatalf("read staged ASR audio before response: %v", err)
	}
	if method != "/backend.Backend/AudioTranscription" || endpoint != "grpc://127.0.0.1:45906" ||
		filepath.Dir(request.GetDst()) != stagingDirectory || request.GetThreads() != 4 ||
		string(stagedAudio) != string(audio) {
		t.Fatalf("AudioTranscription request endpoint=%q method=%q threads=%d stagedAudio=%q", endpoint, method, request.GetThreads(), stagedAudio)
	}
}

func assertCompletedASRInvocation(t *testing.T, invocation controlledASRInvocation) {
	t.Helper()
	if invocation.err != nil || invocation.result.Status != models.ModelInvocationStatusCompleted ||
		invocation.result.LeaseDisposition != models.InvocationLeaseReleased || len(invocation.result.Content) != 2 ||
		invocation.result.Content[0].Name != "transcript" ||
		invocation.result.Content[0].Content != "private transcript marker" ||
		invocation.result.Content[1].Name != "segments" ||
		invocation.result.Content[1].MediaType != "application/json" ||
		len(invocation.result.Outputs) != 2 {
		t.Fatalf("response-first invocation = result:%#v error:%v, want decoded output and released lease", invocation.result, invocation.err)
	}
	outputContents := make(map[string]string, len(invocation.result.Outputs))
	for _, output := range invocation.result.Outputs {
		outputContents[output.Name] = output.Content
	}
	if outputContents["transcript"] != "private transcript marker" ||
		outputContents["segments"] != invocation.result.Content[1].Content {
		t.Fatalf("response-first named outputs = %#v, want transcript and segments", outputContents)
	}
	var segments []struct {
		ID    int32  `json:"id"`
		Start int64  `json:"start"`
		End   int64  `json:"end"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal([]byte(invocation.result.Content[1].Content), &segments); err != nil ||
		len(segments) != 1 || segments[0].ID != 0 || segments[0].Start != 0 ||
		segments[0].End != 500 || segments[0].Text != "private transcript marker" {
		t.Fatalf("response-first segments = %#v, decode error = %v, want one ordered 500 ms segment", segments, err)
	}
}

func assertExpiredLeaseASRInvocation(t *testing.T, invocation controlledASRInvocation) {
	t.Helper()
	if invocation.err == nil || !errors.Is(invocation.err, models.ErrInferenceFailed) ||
		!errors.Is(invocation.err, models.ErrHostLeaseExpired) ||
		invocation.result.Status != models.ModelInvocationStatusFailed ||
		invocation.result.LeaseDisposition != models.InvocationLeaseExpired ||
		len(invocation.result.Content) != 0 || len(invocation.result.Outputs) != 0 {
		t.Fatalf("exit-first invocation = result:%#v error:%v, want expired lease and withheld output", invocation.result, invocation.err)
	}
}

func assertLeaseStatus(
	t *testing.T,
	host runtimehost.Service,
	scope models.RuntimeScopeRef,
	lease models.ModelLeaseRef,
	want models.ModelLeaseStatus,
) {
	t.Helper()
	result, err := host.GetModelLease(context.Background(), models.GetModelLeaseRequest{Scope: scope, Lease: lease})
	if err != nil && !(want == models.ModelLeaseStatusExpired && errors.Is(err, models.ErrHostLeaseExpired)) {
		t.Fatalf("GetModelLease status error = %v, want %s", err, want)
	}
	if result.Lease.Status != want {
		t.Fatalf("GetModelLease status = %s, want %s", result.Lease.Status, want)
	}
}

func assertASRStagingRemoved(t *testing.T, directory string) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read ASR staging directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("ASR staging directory retained files: %v", entries)
	}
}

func assertASRConnectionClosed(t *testing.T, connection *controlledASRConnection) {
	t.Helper()
	if connection.closeCount() != 1 || connection.dialCount() != 1 {
		t.Fatalf("ASR protocol connection opened/closed = %d/%d, want one call and one close", connection.dialCount(), connection.closeCount())
	}
}

func assertSharedRuntimeEvidence(
	t *testing.T,
	records []modelseffects.RuntimeEvidenceRecord,
	responseFirst bool,
	tempDirectory string,
) {
	t.Helper()
	var asrSequence, crashSequence uint64
	for index, record := range records {
		if record.Sequence != uint64(index+1) {
			t.Fatalf("runtime evidence sequence[%d] = %d, want %d", index, record.Sequence, index+1)
		}
		if record.Kind == modelseffects.RuntimeEvidenceKindASRProtocol {
			asrSequence = record.Sequence
			if record.Outcome != modelseffects.RuntimeEvidenceOutcomeCompleted || record.ASRProtocol == nil ||
				record.ASRProtocol.Phase != modelseffects.RuntimeASRPhaseComplete ||
				record.ASRProtocol.RPCMethod != modelseffects.RuntimeASRPCMethodAudioTranscription ||
				record.ASRProtocol.RPCStatus != "OK" || !record.ASRProtocol.ResponseDecoded ||
				record.ASRProtocol.SelectedBackend != "LOCALAI_WHISPER" ||
				record.ASRProtocol.SelectedPlatform != "WINDOWS_AMD64" ||
				!record.ASRProtocol.ResponseReceived || !record.ASRProtocol.DestinationCompared ||
				!record.ASRProtocol.DestinationMatchesStaged || record.ASRProtocol.AudioBytes != uint64(len("private audio marker")) ||
				record.ASRProtocol.SegmentCount != 1 {
				t.Fatalf("ASR protocol observation = %#v, want a successful decoded pinned call", record)
			}
		}
		if record.Kind == modelseffects.RuntimeEvidenceKindStage &&
			record.Stage == modelseffects.RuntimeStageBackendStart &&
			record.Outcome == modelseffects.RuntimeEvidenceOutcomeFailed {
			crashSequence = record.Sequence
		}
	}
	if asrSequence == 0 || crashSequence == 0 ||
		(responseFirst && asrSequence >= crashSequence) || (!responseFirst && crashSequence >= asrSequence) {
		t.Fatalf("shared event order ASR=%d child-crash=%d responseFirst=%t records=%#v", asrSequence, crashSequence, responseFirst, records)
	}
	serialized, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("marshal bounded runtime evidence: %v", err)
	}
	for _, privateValue := range []string{
		"private audio marker", "private transcript marker", "grpc://127.0.0.1:45906", tempDirectory,
	} {
		if strings.Contains(string(serialized), privateValue) {
			t.Fatalf("runtime evidence leaked private value %q: %s", privateValue, serialized)
		}
	}
}

func awaitSignal(t *testing.T, signal <-chan struct{}, label string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-signal:
	case <-timer.C:
		t.Fatalf("timed out waiting for %s", label)
	}
}

func awaitCorrelationSignal(
	t *testing.T,
	controller *modelseffects.ASRLiveCorrelationController,
	kind modelseffects.ASRLiveCorrelationEventKind,
) modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	event, err := controller.WaitForSignal(ctx, kind)
	if err != nil {
		t.Fatalf("wait for ASR correlation signal %s: %v", kind, err)
	}
	return event
}

func assertASRLiveCorrelationOrder(
	t *testing.T,
	events []modelseffects.ASRLiveCorrelationEvent,
	responseFirst bool,
) {
	t.Helper()
	want := []modelseffects.ASRLiveCorrelationEventKind{
		modelseffects.ASRLiveCorrelationChildStarted,
		modelseffects.ASRLiveCorrelationEndpointObserved,
		modelseffects.ASRLiveCorrelationRPCTerminal,
	}
	if responseFirst {
		want = append(want,
			modelseffects.ASRLiveCorrelationResponseSent,
			modelseffects.ASRLiveCorrelationChildWaited,
			modelseffects.ASRLiveCorrelationHostFailureSeen,
		)
	} else {
		want = append(want,
			modelseffects.ASRLiveCorrelationChildWaited,
			modelseffects.ASRLiveCorrelationHostFailureSeen,
			modelseffects.ASRLiveCorrelationResponseSent,
		)
	}
	if len(events) != len(want) {
		t.Fatalf("ASR correlation events = %#v, want %d ordered signals", events, len(want))
	}
	for index, event := range events {
		if event.Sequence != uint64(index+1) || event.Kind != want[index] {
			t.Fatalf("ASR correlation event[%d] = %#v, want sequence=%d kind=%s", index, event, index+1, want[index])
		}
		if event.Kind == modelseffects.ASRLiveCorrelationEndpointObserved &&
			(event.Endpoint.Port != 45906 || event.Endpoint.ListenerProcessID != controlledASRChildPID || event.ProcessID != controlledASRChildPID) {
			t.Fatalf("ASR endpoint ownership event = %#v, want dynamic port and owned PID", event)
		}
		if event.Kind == modelseffects.ASRLiveCorrelationChildWaited && event.ProcessID != controlledASRChildPID {
			t.Fatalf("ASR child Wait event = %#v, want owned PID %d", event, controlledASRChildPID)
		}
		if event.Kind == modelseffects.ASRLiveCorrelationHostFailureSeen && event.ProcessID != controlledASRChildPID {
			t.Fatalf("ASR host-failure event = %#v, want owned PID %d", event, controlledASRChildPID)
		}
	}
}

func correlationEvent(
	t *testing.T,
	events []modelseffects.ASRLiveCorrelationEvent,
	kind modelseffects.ASRLiveCorrelationEventKind,
) modelseffects.ASRLiveCorrelationEvent {
	t.Helper()
	for _, event := range events {
		if event.Kind == kind {
			return event
		}
	}
	t.Fatalf("ASR correlation event %s missing from %#v", kind, events)
	return modelseffects.ASRLiveCorrelationEvent{}
}

func awaitRPCStart(t *testing.T, started <-chan struct{}, result <-chan controlledASRInvocation) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-started:
	case outcome := <-result:
		t.Fatalf("ASR invocation ended before AudioTranscription: result=%#v error=%v", outcome.result, outcome.err)
	case <-timer.C:
		t.Fatal("timed out waiting for ASR AudioTranscription RPC start")
	}
}

func awaitInvocation(t *testing.T, result <-chan controlledASRInvocation) controlledASRInvocation {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case outcome := <-result:
		return outcome
	case <-timer.C:
		t.Fatal("timed out waiting for ASR invocation")
		return controlledASRInvocation{}
	}
}

type controlledASRScenario struct {
	scope         models.RuntimeScopeRef
	host          runtimehost.Service
	inference     inference.Service
	launcher      *controlledASRLauncher
	process       *controlledASRProcess
	protocol      *controlledASRProtocol
	connection    *controlledASRConnection
	tempDirectory string
	audio         []byte
	correlation   *modelseffects.ASRLiveCorrelationController
	recorder      modelseffects.RuntimeEvidenceRecorder
	recorderSink  *controlledASREvidenceSink
}

type controlledASRInvocation struct {
	result models.InvokeModelResult
	err    error
}

type controlledASRAssets struct {
	scopedassets.Service
	inspection scopedassets.RuntimeCacheInspection
}

func (assets *controlledASRAssets) InspectRuntimeCache(
	context.Context,
	models.InspectModelAssetsRequest,
) (scopedassets.RuntimeCacheInspection, error) {
	return assets.inspection, nil
}

type controlledASRLauncher struct {
	mu             sync.Mutex
	managedProcess *controlledASRProcess
	correlation    *modelseffects.ASRLiveCorrelationController
}

const controlledASRChildPID = 8123

func (launcher *controlledASRLauncher) Start(
	_ context.Context,
	spec modelseffects.HostProcessStartSpec,
) (modelseffects.HostManagedProcess, error) {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	if launcher.managedProcess != nil {
		return nil, errors.New("controlled ASR launcher received a second start")
	}
	launcher.managedProcess = &controlledASRProcess{
		endpoint:    spec.HealthEndpoint,
		pid:         controlledASRChildPID,
		exitGate:    make(chan error, 1),
		waited:      make(chan struct{}),
		correlation: launcher.correlation,
	}
	if err := launcher.correlation.RecordManagedChildStarted(launcher.managedProcess.pid, spec.HealthEndpoint); err != nil {
		return nil, err
	}
	return launcher.managedProcess, nil
}

func (launcher *controlledASRLauncher) process() *controlledASRProcess {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.managedProcess
}

type controlledASRProcess struct {
	endpoint      string
	pid           int
	exitGate      chan error
	waited        chan struct{}
	correlation   *modelseffects.ASRLiveCorrelationController
	exitOnce      sync.Once
	waitOnce      sync.Once
	waitCalls     atomic.Int32
	waitResult    error
	waitSignalErr error
}

func (process *controlledASRProcess) HealthEndpoint() string { return process.endpoint }

func (process *controlledASRProcess) Wait() error {
	process.waitCalls.Add(1)
	process.waitOnce.Do(func() {
		process.waitResult = <-process.exitGate
		process.waitSignalErr = process.correlation.RecordChildWaited(process.pid, "NONZERO_EXIT", true, 1)
		close(process.waited)
	})
	return errors.Join(process.waitResult, process.waitSignalErr)
}

func (process *controlledASRProcess) Stop(context.Context) error {
	process.exit(errors.New("controlled ASR process stopped"))
	return nil
}

func (process *controlledASRProcess) RuntimeHostFailureObserved() {
	process.waitSignalErr = errors.Join(
		process.waitSignalErr,
		process.correlation.RecordHostFailureObserved(process.pid),
	)
}

func (process *controlledASRProcess) exit(err error) {
	process.exitOnce.Do(func() { process.exitGate <- err })
}

func (process *controlledASRProcess) waitCount() int32 {
	return process.waitCalls.Load()
}

type controlledASRProtocol struct {
	mu             sync.Mutex
	resolvedConfig modelseffects.ResolvedHostConfiguration
}

func (protocol *controlledASRProtocol) Negotiate(
	_ context.Context,
	_ string,
	request modelseffects.HostProtocolNegotiationRequest,
) (modelseffects.HostProtocolNegotiationResult, error) {
	protocol.mu.Lock()
	protocol.resolvedConfig = request.Configuration.Clone()
	protocol.mu.Unlock()
	return modelseffects.HostProtocolNegotiationResult{
		ProtocolVersion: request.Configuration.ProtocolVersion,
		Backend:         request.Configuration.Backend, Ready: true,
	}, nil
}

func (protocol *controlledASRProtocol) configuration() modelseffects.ResolvedHostConfiguration {
	protocol.mu.Lock()
	defer protocol.mu.Unlock()
	return protocol.resolvedConfig.Clone()
}

type controlledASRCompatibility struct{}

func (controlledASRCompatibility) Check(context.Context, modelseffects.HostCompatibilityRequest) error {
	return nil
}

type controlledASRNoHTTP struct{}

func (controlledASRNoHTTP) Do(*http.Request) (*http.Response, error) {
	return nil, errors.New("unexpected HTTP readiness request in pinned gRPC characterization")
}

type controlledASRClock struct{}

func (*controlledASRClock) Now() time.Time { return time.Now() }

func (clock *controlledASRClock) NewTimer(duration time.Duration) modelseffects.HostTimer {
	_ = clock
	return controlledASRTimer{timer: time.NewTimer(duration)}
}

type controlledASRTimer struct{ timer *time.Timer }

func (timer controlledASRTimer) C() <-chan time.Time { return timer.timer.C }
func (timer controlledASRTimer) Stop() bool          { return timer.timer.Stop() }

type controlledASRDialer struct{ connection *controlledASRConnection }

func (dialer controlledASRDialer) Dial(
	_ context.Context,
	endpoint string,
) (platformgrpc.Connection, error) {
	dialer.connection.mu.Lock()
	dialer.connection.endpoint = endpoint
	dialer.connection.dials++
	dialer.connection.mu.Unlock()
	return dialer.connection, nil
}

type controlledASRConnection struct {
	mu            sync.Mutex
	started       chan struct{}
	responseReady chan struct{}
	response      []byte
	endpoint      string
	method        string
	requestBytes  []byte
	dials         int
	closed        int
	responseOnce  sync.Once
}

func (connection *controlledASRConnection) Invoke(
	ctx context.Context,
	method string,
	request []byte,
) ([]byte, error) {
	connection.mu.Lock()
	connection.method = method
	connection.requestBytes = append([]byte(nil), request...)
	connection.mu.Unlock()
	connection.started <- struct{}{}
	select {
	case <-connection.responseReady:
		return append([]byte(nil), connection.response...), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (connection *controlledASRConnection) Close() error {
	connection.mu.Lock()
	connection.closed++
	connection.mu.Unlock()
	return nil
}

func (connection *controlledASRConnection) allowResponse() {
	connection.responseOnce.Do(func() { close(connection.responseReady) })
}

func (connection *controlledASRConnection) request() (string, string, []byte) {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.method, connection.endpoint, append([]byte(nil), connection.requestBytes...)
}

func (connection *controlledASRConnection) closeCount() int {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.closed
}

func (connection *controlledASRConnection) dialCount() int {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.dials
}

type controlledASREvidenceSink struct {
	mu      sync.Mutex
	records []modelseffects.RuntimeEvidenceRecord
	crashed chan struct{}
}

func (sink *controlledASREvidenceSink) RecordRuntimeEvidence(record modelseffects.RuntimeEvidenceRecord) {
	sink.mu.Lock()
	sink.records = append(sink.records, record)
	sink.mu.Unlock()
	if record.Kind == modelseffects.RuntimeEvidenceKindStage &&
		record.Stage == modelseffects.RuntimeStageBackendStart &&
		record.Outcome == modelseffects.RuntimeEvidenceOutcomeFailed {
		select {
		case sink.crashed <- struct{}{}:
		default:
		}
	}
}

func (sink *controlledASREvidenceSink) snapshot() []modelseffects.RuntimeEvidenceRecord {
	sink.mu.Lock()
	defer sink.mu.Unlock()
	return append([]modelseffects.RuntimeEvidenceRecord(nil), sink.records...)
}

var _ modelseffects.HostProcessLauncher = (*controlledASRLauncher)(nil)
var _ modelseffects.HostManagedProcess = (*controlledASRProcess)(nil)
var _ modelseffects.HostManagedProcessFailureObserver = (*controlledASRProcess)(nil)
var _ modelseffects.HostProtocolNegotiator = (*controlledASRProtocol)(nil)
var _ modelseffects.HostCompatibilityChecker = controlledASRCompatibility{}
var _ modelseffects.HostHTTPDoer = controlledASRNoHTTP{}
var _ modelseffects.HostClock = (*controlledASRClock)(nil)
var _ modelseffects.HostTimer = controlledASRTimer{}
var _ platformgrpc.Dialer = controlledASRDialer{}
var _ platformgrpc.Connection = (*controlledASRConnection)(nil)
var _ modelseffects.RuntimeEvidenceRecorder = (*controlledASREvidenceSink)(nil)
var _ scopedassets.Service = (*controlledASRAssets)(nil)
