package omni_artifact_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const omniModelSource = "hf://unsloth/gemma-4-E4B-it-GGUF/gemma-4-E4B-it-Q4_K_M.gguf@bfc15c382204943c3a8fff0c750b94ae2364d7a3"

// TestFactorySessionOmniArtifactJourney keeps the story's executable spine on
// the public Factory Session boundary: one root-built Process executes an
// explicit --session run, and the only model effect replaced is the
// provider-neutral protocol edge.
func TestFactorySessionOmniArtifactJourney(t *testing.T) {
	t.Parallel()

	const wantText = "Exact Unicode: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	response := fixture.invoke(t, "Return the exact fixture")
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED: %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil || part.Text != wantText {
		t.Fatalf("primary result part = %#v, err=%v, want exact %q", part, err, wantText)
	}
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol calls = %d, want one", fixture.protocol.Calls())
	}
	events := fixture.recording(t)
	journey := assertSuccessfulJourney(t, events, "Return the exact fixture", wantText)
	t.Logf("Factory Session=%s request=%s trace=%s work=%s artifact=%s mime=%s size=%d protocolCalls=%d",
		fixture.sessionID, response.RequestId, response.TraceId, requiredString(journey.outputWork.WorkId),
		journey.artifactID, requiredString(journey.outputPart.ContentType), len([]byte(wantText)), fixture.protocol.Calls())
}

func TestFactorySessionOmniArtifactReplayPreservesOrderAndLineage(t *testing.T) {
	t.Parallel()

	const wantText = "Replay exact: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	live := fixture.invoke(t, "Replay this exact fixture")
	liveEvents := fixture.recording(t)
	liveJourney := assertSuccessfulJourney(t, liveEvents, "Replay this exact fixture", wantText)
	replayedEvents := fixture.replay(t)
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol calls after replay = %d, want one live call and no replay call", fixture.protocol.Calls())
	}
	if !jsonEqual(t, replayedEvents, liveEvents) {
		t.Fatalf("replayed canonical events differ from live recording")
	}
	replayedJourney := assertSuccessfulJourney(t, replayedEvents, "Replay this exact fixture", wantText)
	if replayedJourney.artifactID != liveJourney.artifactID || requiredString(replayedJourney.outputWork.WorkId) != requiredString(liveJourney.outputWork.WorkId) {
		t.Fatalf("replayed Work/artifact identity = (%q,%q), want live (%q,%q)", requiredString(replayedJourney.outputWork.WorkId), replayedJourney.artifactID, requiredString(liveJourney.outputWork.WorkId), liveJourney.artifactID)
	}
	if live.Status != factoryapi.InvocationTerminalStatusCompleted || live.RequestId != requiredString(liveJourney.workRequest.Context.RequestId) || live.TraceId != firstTraceID(t, liveJourney.workRequest) {
		t.Fatalf("live response = %#v, want completed response with canonical request/trace identity", live)
	}
	assertEventLineage(t, liveEvents, "live and replay source")
}

func TestFactorySessionOmniArtifactMetadataIsSemantic(t *testing.T) {
	t.Parallel()

	const wantText = "Semantic Unicode: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	response := fixture.invoke(t, "Return semantic metadata")
	journey := assertSuccessfulJourney(t, fixture.recording(t), "Return semantic metadata", wantText)
	resultPart := workTextPart(t, response.PrimaryResult, "primary result")
	if resultPart.Type != factoryapi.WorkContentPartTypeText || resultPart.Text != wantText {
		t.Fatalf("primary result part = %#v, want exact semantic text", resultPart)
	}
	if resultPart.ContentType == nil || *resultPart.ContentType != "text/plain" {
		t.Fatalf("primary result content type = %#v, want text/plain", resultPart.ContentType)
	}
	if resultPart.ArtifactId == nil || *resultPart.ArtifactId != journey.artifactID {
		t.Fatalf("primary result artifact id = %#v, want %q", resultPart.ArtifactId, journey.artifactID)
	}
	if got, want := len([]byte(resultPart.Text)), len([]byte(wantText)); got != want {
		t.Fatalf("UTF-8 result size = %d, want %d bytes", got, want)
	}
	if journey.outputWork.State == nil || journey.outputWork.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("output Work state = %#v, want terminal", journey.outputWork.State)
	}
	if journey.outputWork.Content == nil || len(*journey.outputWork.Content) != 1 {
		t.Fatalf("output Work content = %#v, want one materialized part", journey.outputWork.Content)
	}
	outputPart := workTextPart(t, journey.outputWork.Content, "output Work")
	if outputPart.Text != wantText || outputPart.ArtifactId == nil || *outputPart.ArtifactId != journey.artifactID || outputPart.ContentType == nil || *outputPart.ContentType != "text/plain" {
		t.Fatalf("output Work part = %#v, want one text/plain artifact-bearing part", outputPart)
	}
}

func TestFactorySessionOmniArtifactLineageIsPreserved(t *testing.T) {
	t.Parallel()

	const wantText = "Lineage exact: café, 東京, and 🌍"
	fixture := newFactoryFixture(t, wantText)
	response := fixture.invoke(t, "Preserve this lineage")
	events := fixture.recording(t)
	journey := assertSuccessfulJourney(t, events, "Preserve this lineage", wantText)
	if response.RequestId != requiredString(journey.workRequest.Context.RequestId) || response.TraceId != firstTraceID(t, journey.workRequest) {
		t.Fatalf("response identity request=%q trace=%q, want event request=%q trace=%q", response.RequestId, response.TraceId, requiredString(journey.workRequest.Context.RequestId), firstTraceID(t, journey.workRequest))
	}
	assertEventLineage(t, events, "lineage")
	if journey.outputWork.WorkId == nil || journey.outputWork.CurrentChainingTraceId == nil || journey.outputWork.ChainingTraceDepth == nil {
		t.Fatalf("output Work lineage = %#v, want work/current-trace/depth identities", journey.outputWork)
	}
}

func TestFactorySessionOmniArtifactMaterializationFailureIsAtomic(t *testing.T) {
	t.Parallel()

	fixture := newFactoryFixture(t, "unused after rejection")
	fixture.protocol.SetError(errors.New("fixture materialization rejection"))
	err, stderr := fixture.invokeExpectFailure(t, "Reject this materialization")
	if err == nil {
		t.Fatal("materialization rejection error = nil, want failed Factory attempt")
	}
	events := fixture.recording(t)
	if fixture.protocol.Calls() != 1 {
		t.Fatalf("protocol calls = %d, want one rejected attempt", fixture.protocol.Calls())
	}
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeArtifactCreated {
			t.Fatalf("atomic materialization failure emitted artifact event: %#v", event)
		}
	}
	if event := optionalFactoryEvent(events, factoryapi.FactoryEventTypeModelResponse); event != nil {
		payload, decodeErr := event.Payload.AsModelResponseEventPayload()
		if decodeErr != nil {
			t.Fatalf("decode failed MODEL_RESPONSE: %v", decodeErr)
		}
		if payload.OutputContent != nil {
			t.Fatalf("failed MODEL_RESPONSE output = %#v, want no successful output", payload.OutputContent)
		}
	}
	dispatch := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse)
	dispatchPayload, decodeErr := dispatch.Payload.AsDispatchResponseEventPayload()
	if decodeErr != nil {
		t.Fatalf("decode failed DISPATCH_RESPONSE: %v", decodeErr)
	}
	if dispatchPayload.Outcome == factoryapi.WorkOutcomeAccepted {
		t.Fatalf("failed DISPATCH_RESPONSE = %#v, want failed route", dispatchPayload)
	}
	if dispatchPayload.OutputWork != nil {
		for _, outputWork := range *dispatchPayload.OutputWork {
			if outputWork.State != nil && outputWork.State.Type == factoryapi.WorkStateTypeTERMINAL {
				t.Fatalf("failed DISPATCH_RESPONSE recorded terminal output Work = %#v", outputWork)
			}
			if outputWork.State != nil && outputWork.State.Type != factoryapi.WorkStateTypeFAILED {
				t.Fatalf("failed DISPATCH_RESPONSE Work state = %#v, want FAILED", outputWork.State)
			}
			if outputWork.Content != nil {
				for _, part := range *outputWork.Content {
					textPart, partErr := part.AsWorkTextContentPart()
					if partErr == nil && textPart.ArtifactId != nil && *textPart.ArtifactId != "" {
						t.Fatalf("failed DISPATCH_RESPONSE recorded artifact-bearing content = %#v", textPart)
					}
				}
			}
		}
	}
	if strings.Contains(stderr, "fixture materialization rejection") {
		t.Fatalf("raw fixture failure leaked through CLI diagnostics: %q", stderr)
	}
}

func TestFactorySessionOmniArtifactReleaseIsExactlyOnce(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		fixture := newFactoryFixture(t, "release success")
		fixture.invoke(t, "Release success")
		fixture.assertRelease(t)
	})
	t.Run("backend error", func(t *testing.T) {
		fixture := newFactoryFixture(t, "unused backend error")
		fixture.protocol.SetError(errors.New("fixture backend error"))
		_, _ = fixture.invokeExpectFailure(t, "Release backend error")
		fixture.assertRelease(t)
	})
}

type factoryFixture struct {
	home          string
	sessionID     string
	dir           string
	recordingPath string
	protocol      *omniProtocolFixture
	process       support.Process
	launcher      *modelHostLauncher
}

func newFactoryFixture(t *testing.T, response string) *factoryFixture {
	t.Helper()

	home := t.TempDir()
	writeBuiltinModelCache(t, home)
	selection := llamaBackendSelection()
	writeBackendCache(t, home, selection)

	modelServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/health" {
			writer.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(writer, request)
	}))
	t.Cleanup(modelServer.Close)

	protocol := &omniProtocolFixture{response: response}
	launcher := &modelHostLauncher{endpoint: modelServer.URL}
	assetFiles := modelAssetFileSystem{home: home}
	edges := serviceedges.Edges{
		ModelAssetHTTPClient:           rejectingAssetHTTP{},
		ModelAssetHostPlatform:         models.AssetHostPlatform{OperatingSystem: "linux", Architecture: "amd64"},
		ModelAssetMakeDirectories:      assetFiles.MkdirAll,
		ModelAssetInspectPath:          assetFiles.Stat,
		ModelAssetResolveHomeDirectory: assetFiles.UserHomeDir,
		ModelAssetResolveEnvironment:   func(string) string { return "" },
		ModelAssetWriteFile:            assetFiles.WriteFile,
		ModelAssetRenamePath:           assetFiles.Rename,
		ModelAssetRemovePath:           assetFiles.Remove,
		ModelAssetReadFile:             assetFiles.ReadFile,
		ModelAssetReadDirectory:        assetFiles.ReadDir,
		ModelAssetCreateFile:           assetFiles.Create,
		ModelAssetOpenFile:             assetFiles.Open,
		ModelResolveBackendArtifact: func(context.Context, serviceedges.ModelBackendArtifactSelectionRequest) (serviceedges.ModelBackendArtifactSelection, error) {
			return selection, nil
		},
		ModelHostProcessLauncher:      launcher,
		ModelHostProtocolNegotiator:   modelHostProtocolNegotiator{},
		ModelHostCompatibilityChecker: modelHostCompatibilityChecker{},
		ModelHostHTTPClient:           modelServer.Client(),
		ModelRuntimeHTTPClient:        modelServer.Client(),
		ModelInvocationProtocolClient: protocol,
	}
	dir := support.ScaffoldFactory(t, omniFactoryConfig(modelServer.URL))
	support.WriteWorkstationConfig(t, dir, "execute-llm", "---\ntype: INFERENCE_RUN\n---\nReturn the model result.\n")
	recordingPath := filepath.Join(t.TempDir(), "omni-artifact-recording.json")
	process := support.BuildProcess(t, edges)
	return &factoryFixture{
		home: home, sessionID: "factory-session-omni-artifact", dir: dir,
		recordingPath: recordingPath, protocol: protocol, process: process, launcher: launcher,
	}
}

func (fixture *factoryFixture) invoke(t *testing.T, prompt string) factoryapi.InvocationResponse {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(fixture.dir, "factory.json"),
		"--session", fixture.sessionID, "--record", fixture.recordingPath, "--output", "primary", prompt,
	})
	inputs.Input.Env = functionalHomeEnvironment(fixture.home)
	inputs.Input.WorkingDirectory = fixture.dir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("root Process.Execute(Factory Session) error=%v stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	return support.DecodeInvocationResponseJSON(t, inputs.Stdout())
}

func (fixture *factoryFixture) invokeExpectFailure(t *testing.T, prompt string) (error, string) {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", filepath.Join(fixture.dir, "factory.json"),
		"--session", fixture.sessionID, "--record", fixture.recordingPath, "--output", "primary", prompt,
	})
	inputs.Input.Env = functionalHomeEnvironment(fixture.home)
	inputs.Input.WorkingDirectory = fixture.dir
	return fixture.process.Execute(inputs.Input), inputs.Stderr()
}

func (fixture *factoryFixture) recording(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	data, err := os.ReadFile(fixture.recordingPath)
	if err != nil {
		t.Fatalf("read Factory recording: %v", err)
	}
	var artifact struct {
		Events        []factoryapi.FactoryEvent `json:"events"`
		SchemaVersion string                    `json:"schemaVersion"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode Factory recording: %v", err)
	}
	if artifact.SchemaVersion == "" {
		t.Fatal("Factory recording schema version is empty")
	}
	if len(artifact.Events) == 0 {
		t.Fatal("Factory recording contains no canonical events")
	}
	return artifact.Events
}

func (fixture *factoryFixture) replay(t *testing.T) []factoryapi.FactoryEvent {
	t.Helper()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--dir", fixture.dir, "--replay", fixture.recordingPath,
		"--no-record", "--output", "primary",
	})
	inputs.Input.Env = functionalHomeEnvironment(fixture.home)
	inputs.Input.WorkingDirectory = fixture.dir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("root Process.Execute(Factory replay) error=%v stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	return fixture.recording(t)
}

type successfulJourney struct {
	workRequest      factoryapi.FactoryEvent
	dispatchRequest  factoryapi.FactoryEvent
	modelRequest     factoryapi.FactoryEvent
	modelResponse    factoryapi.FactoryEvent
	dispatchResponse factoryapi.FactoryEvent
	sessionCompleted factoryapi.FactoryEvent
	inputWork        factoryapi.Work
	outputWork       factoryapi.Work
	outputPart       factoryapi.WorkTextContentPart
	artifactID       string
}

func assertSuccessfulJourney(t *testing.T, events []factoryapi.FactoryEvent, prompt, wantText string) successfulJourney {
	t.Helper()
	journey := inspectJourney(t, events)
	if journey.inputWork.Content == nil || len(*journey.inputWork.Content) != 1 {
		t.Fatalf("input Work content = %#v, want one text part", journey.inputWork.Content)
	}
	inputPart := workTextPart(t, journey.inputWork.Content, "input Work")
	if inputPart.Text != prompt {
		t.Fatalf("input Work text = %q, want %q", inputPart.Text, prompt)
	}
	modelResponse, err := journey.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_RESPONSE: %v", err)
	}
	if modelResponse.Outcome != factoryapi.InferenceOutcomeSucceeded || modelResponse.OutputContent == nil || len(*modelResponse.OutputContent) != 1 {
		t.Fatalf("MODEL_RESPONSE = %#v, want successful one-part output", modelResponse)
	}
	modelPart := workTextPart(t, modelResponse.OutputContent, "MODEL_RESPONSE")
	if modelPart.Text != wantText {
		t.Fatalf("MODEL_RESPONSE text = %q, want %q", modelPart.Text, wantText)
	}
	if modelPart.ArtifactId == nil || *modelPart.ArtifactId == "" {
		t.Fatal("MODEL_RESPONSE output has no opaque artifact id")
	}
	if modelPart.ContentType == nil || *modelPart.ContentType != "text/plain" {
		t.Fatalf("MODEL_RESPONSE content type = %#v, want text/plain", modelPart.ContentType)
	}
	if modelPart.Type != factoryapi.WorkContentPartTypeText {
		t.Fatalf("MODEL_RESPONSE content discriminator = %q, want text", modelPart.Type)
	}
	if journey.outputPart.Text != wantText || journey.outputPart.ArtifactId == nil || *journey.outputPart.ArtifactId != *modelPart.ArtifactId {
		t.Fatalf("output Work part = %#v, want exact model artifact-bearing result %#v", journey.outputPart, modelPart)
	}
	journey.artifactID = *modelPart.ArtifactId
	return journey
}

func inspectJourney(t *testing.T, events []factoryapi.FactoryEvent) successfulJourney {
	t.Helper()
	if len(events) < 7 {
		t.Fatalf("canonical event count = %d, want at least seven events", len(events))
	}
	journey := successfulJourney{
		workRequest:      requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeWorkRequest),
		dispatchRequest:  requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchRequest),
		modelRequest:     requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelRequest),
		modelResponse:    requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelResponse),
		dispatchResponse: requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse),
		sessionCompleted: requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeSessionCompleted),
	}
	order := []factoryapi.FactoryEventType{
		factoryapi.FactoryEventTypeWorkRequest,
		factoryapi.FactoryEventTypeDispatchRequest,
		factoryapi.FactoryEventTypeModelRequest,
		factoryapi.FactoryEventTypeModelResponse,
		factoryapi.FactoryEventTypeDispatchResponse,
		factoryapi.FactoryEventTypeSessionCompleted,
	}
	previous := -1
	for _, eventType := range order {
		current := eventIndex(events, eventType)
		if current <= previous {
			t.Fatalf("canonical event order has %s at index %d after index %d", eventType, current, previous)
		}
		previous = current
	}
	workPayload, err := journey.workRequest.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST: %v", err)
	}
	if workPayload.Works == nil || len(*workPayload.Works) != 1 {
		t.Fatalf("WORK_REQUEST works = %#v, want one Work", workPayload.Works)
	}
	journey.inputWork = (*workPayload.Works)[0]
	dispatchPayload, err := journey.dispatchRequest.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_REQUEST: %v", err)
	}
	if len(dispatchPayload.Inputs) != 1 {
		t.Fatalf("DISPATCH_REQUEST inputs = %#v, want one Work reference", dispatchPayload.Inputs)
	}
	if journey.inputWork.WorkId == nil || dispatchPayload.Inputs[0].WorkId != *journey.inputWork.WorkId {
		t.Fatalf("dispatch input = %#v, want Work ID %q", dispatchPayload.Inputs[0], requiredString(journey.inputWork.WorkId))
	}
	if association := optionalFactoryEvent(events, factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation); association != nil {
		payload, decodeErr := association.Payload.AsDispatchWorkerSessionAssociationEventPayload()
		if decodeErr != nil || payload.WorkerSessionId == "" {
			t.Fatalf("worker-session association = %#v, decode error=%v", association, decodeErr)
		}
	}
	modelRequest, err := journey.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_REQUEST: %v", err)
	}
	if modelRequest.Operation != models.OperationOMNI || modelRequest.ModelRequestId == "" || modelRequest.Worker == "" {
		t.Fatalf("MODEL_REQUEST = %#v, want OMNI request identity", modelRequest)
	}
	modelResponse, err := journey.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_RESPONSE: %v", err)
	}
	if modelResponse.ModelRequestId != modelRequest.ModelRequestId {
		t.Fatalf("MODEL_RESPONSE model request identity = %q, want %q", modelResponse.ModelRequestId, modelRequest.ModelRequestId)
	}
	dispatchResponse, err := journey.dispatchResponse.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_RESPONSE: %v", err)
	}
	if dispatchResponse.Outcome != factoryapi.WorkOutcomeAccepted || dispatchResponse.OutputWork == nil || len(*dispatchResponse.OutputWork) != 1 {
		t.Fatalf("DISPATCH_RESPONSE = %#v, want accepted one-work output", dispatchResponse)
	}
	journey.outputWork = (*dispatchResponse.OutputWork)[0]
	if journey.outputWork.State == nil || journey.outputWork.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("output Work state = %#v, want terminal", journey.outputWork.State)
	}
	if journey.outputWork.Content == nil || len(*journey.outputWork.Content) != 1 {
		t.Fatalf("output Work content = %#v, want one materialized part", journey.outputWork.Content)
	}
	journey.outputPart = workTextPart(t, journey.outputWork.Content, "output Work")
	if journey.outputWork.WorkId == nil || requiredString(journey.outputWork.WorkId) != requiredString(journey.inputWork.WorkId) {
		t.Fatalf("output Work ID = %q, want input %q", requiredString(journey.outputWork.WorkId), requiredString(journey.inputWork.WorkId))
	}
	if journey.outputWork.CurrentChainingTraceId == nil || requiredString(journey.outputWork.CurrentChainingTraceId) != firstTraceID(t, journey.workRequest) {
		t.Fatalf("output Work current trace = %q, want %q", requiredString(journey.outputWork.CurrentChainingTraceId), firstTraceID(t, journey.workRequest))
	}
	if journey.outputWork.TraceId == nil || requiredString(journey.outputWork.TraceId) != firstTraceID(t, journey.workRequest) {
		t.Fatalf("output Work trace = %q, want %q", requiredString(journey.outputWork.TraceId), firstTraceID(t, journey.workRequest))
	}
	if !jsonEqual(t, journey.outputWork.PreviousChainingTraceIds, journey.dispatchResponse.Context.PreviousChainingTraceIds) {
		t.Fatalf("output Work prior chaining trace IDs = %#v, want dispatch context %#v", journey.outputWork.PreviousChainingTraceIds, journey.dispatchResponse.Context.PreviousChainingTraceIds)
	}
	if !jsonEqual(t, journey.outputWork.CurrentChainingTraceId, journey.dispatchResponse.Context.CurrentChainingTraceId) {
		t.Fatalf("output Work current chaining trace = %#v, want dispatch context %#v", journey.outputWork.CurrentChainingTraceId, journey.dispatchResponse.Context.CurrentChainingTraceId)
	}
	assertEventLineage(t, events, "canonical journey")
	return journey
}

func assertEventLineage(t *testing.T, events []factoryapi.FactoryEvent, label string) {
	t.Helper()
	workRequest := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeWorkRequest)
	dispatchRequest := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchRequest)
	modelRequest := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelRequest)
	modelResponse := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeModelResponse)
	dispatchResponse := requiredFactoryEvent(t, events, factoryapi.FactoryEventTypeDispatchResponse)
	requestID := requiredString(workRequest.Context.RequestId)
	traceID := firstTraceID(t, workRequest)
	workID := firstWorkID(t, workRequest)
	dispatchID := requiredString(dispatchRequest.Context.DispatchId)
	if requestID == "" || dispatchID == "" || workID == "" {
		t.Fatalf("%s identity request=%q dispatch=%q work=%q, want all identities", label, requestID, dispatchID, workID)
	}
	for _, event := range []factoryapi.FactoryEvent{dispatchRequest, modelRequest, modelResponse, dispatchResponse} {
		if event.Context.RequestId != nil && *event.Context.RequestId != requestID {
			t.Fatalf("%s event %s request=%q, want %q", label, event.Type, *event.Context.RequestId, requestID)
		}
		if event.Context.DispatchId != nil && *event.Context.DispatchId != dispatchID {
			t.Fatalf("%s event %s dispatch=%q, want %q", label, event.Type, *event.Context.DispatchId, dispatchID)
		}
		if event.Context.WorkIds != nil && firstWorkID(t, event) != workID {
			t.Fatalf("%s event %s Work IDs=%#v, want %q", label, event.Type, event.Context.WorkIds, workID)
		}
		if event.Context.TraceIds != nil && firstTraceID(t, event) != traceID {
			t.Fatalf("%s event %s trace IDs=%#v, want %q", label, event.Type, event.Context.TraceIds, traceID)
		}
	}
	if source := requiredString(workRequest.Context.Source); source == "" {
		t.Fatalf("%s WORK_REQUEST source is empty", label)
	}
	if dispatchResponse.Context.PreviousChainingTraceIds == nil {
		t.Fatalf("%s completion context has no prior chaining trace IDs", label)
	}
}

func (fixture *factoryFixture) assertRelease(t *testing.T) {
	t.Helper()
	if got := fixture.launcher.Starts(); got != 1 {
		t.Fatalf("managed model host starts = %d, want one", got)
	}
	if got := fixture.launcher.Stops(); got != 1 {
		t.Fatalf("managed model host stops = %d, want exactly one", got)
	}
}

func requiredFactoryEvent(t *testing.T, events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) factoryapi.FactoryEvent {
	t.Helper()
	var matches []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == eventType {
			matches = append(matches, event)
		}
	}
	if len(matches) != 1 {
		t.Fatalf("event type %s count = %d, want one", eventType, len(matches))
	}
	return matches[0]
}

func optionalFactoryEvent(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) *factoryapi.FactoryEvent {
	for index := range events {
		if events[index].Type == eventType {
			return &events[index]
		}
	}
	return nil
}

func eventIndex(events []factoryapi.FactoryEvent, eventType factoryapi.FactoryEventType) int {
	for index, event := range events {
		if event.Type == eventType {
			return index
		}
	}
	return -1
}

func firstTraceID(t *testing.T, event factoryapi.FactoryEvent) string {
	t.Helper()
	if event.Context.TraceIds == nil || len(*event.Context.TraceIds) == 0 || (*event.Context.TraceIds)[0] == "" {
		t.Fatalf("event %s has no trace identity: %#v", event.Type, event.Context)
	}
	return (*event.Context.TraceIds)[0]
}

func firstWorkID(t *testing.T, event factoryapi.FactoryEvent) string {
	t.Helper()
	if event.Context.WorkIds == nil || len(*event.Context.WorkIds) == 0 || (*event.Context.WorkIds)[0] == "" {
		t.Fatalf("event %s has no Work identity: %#v", event.Type, event.Context)
	}
	return (*event.Context.WorkIds)[0]
}

func workTextPart(t *testing.T, content *factoryapi.WorkContent, label string) factoryapi.WorkTextContentPart {
	t.Helper()
	if content == nil || len(*content) != 1 {
		t.Fatalf("%s content = %#v, want one part", label, content)
	}
	part, err := (*content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode %s text part: %v", label, err)
	}
	return part
}

func jsonEqual(t *testing.T, left, right interface{}) bool {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatalf("marshal left JSON: %v", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatalf("marshal right JSON: %v", err)
	}
	return string(leftJSON) == string(rightJSON)
}

func omniFactoryConfig(endpoint string) map[string]any {
	return map[string]any{
		"name": "omni-artifact-functional",
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{{
				"name": "prompt", "externalName": "prompt", "required": true,
				"bindings": []map[string]any{{"kind": "POSITIONAL", "position": 1}, {"kind": "STDIN"}, {"kind": "NAMED"}},
			}},
		},
		"workTypes": []map[string]any{{
			"name":             "task",
			"handlingBehavior": []string{factorydefinitions.WorkTypeHandlingBehaviorDefault},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name": "llm-cache", "type": factorydefinitions.ResourceTypeModel, "capacity": 1,
			"model": "llm", "backend": "localai-llamacpp", "loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name": "llm-worker", "type": factorydefinitions.WorkerTypeInference, "model": "llm",
			"modelProvider": "CODEX", "modelLocality": factorydefinitions.ModelLocalityLocal,
			"command": "llama-cpp", "args": []string{"--grpc-endpoint", endpoint},
			"resources": []map[string]any{{"name": "llm-cache", "capacity": 1}},
			"operations": []map[string]any{{
				"name":    models.OperationOMNI,
				"inputs":  []map[string]any{{"name": "prompt", "contentTypes": []string{"TEXT"}, "required": true}},
				"outputs": []map[string]any{{"name": "text", "contentTypes": []string{"TEXT"}, "required": true}},
			}},
		}},
		"workstations": []map[string]any{{
			"name": "execute-llm", "type": factorydefinitions.WorkstationTypeInference, "operation": models.OperationOMNI,
			"worker": "llm-worker", "body": "Return the model result.",
			"operationBindings": []map[string]any{{
				"slot":     "prompt",
				"selector": map[string]any{"type": "TEXT"},
			}},
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	}
}

type omniProtocolFixture struct {
	mu       sync.Mutex
	response string
	failure  error
	calls    int
}

func (fixture *omniProtocolFixture) Predict(ctx context.Context, request models.InvocationProtocolRequest) (models.InvocationProtocolResponse, error) {
	if err := ctx.Err(); err != nil {
		return models.InvocationProtocolResponse{}, err
	}
	fixture.mu.Lock()
	fixture.calls++
	response := fixture.response
	failure := fixture.failure
	fixture.mu.Unlock()
	if failure != nil {
		return models.InvocationProtocolResponse{}, failure
	}
	return models.InvocationProtocolResponse{Text: response}, nil
}

func (fixture *omniProtocolFixture) SetError(err error) {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	fixture.failure = err
}

func (fixture *omniProtocolFixture) Calls() int {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	return fixture.calls
}

type modelHostLauncher struct {
	mu       sync.Mutex
	endpoint string
	starts   int
	stops    int
}

func (launcher *modelHostLauncher) Start(context.Context, serviceedges.HostProcessStartSpec) (interface {
	HealthEndpoint() string
	Wait() error
	Stop(context.Context) error
}, error) {
	launcher.mu.Lock()
	launcher.starts++
	endpoint := launcher.endpoint
	launcher.mu.Unlock()
	return &modelHostProcess{endpoint: endpoint, launcher: launcher, stopped: make(chan struct{})}, nil
}

func (launcher *modelHostLauncher) Starts() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.starts
}

func (launcher *modelHostLauncher) Stops() int {
	launcher.mu.Lock()
	defer launcher.mu.Unlock()
	return launcher.stops
}

type modelHostProcess struct {
	endpoint string
	launcher *modelHostLauncher
	stopped  chan struct{}
	once     sync.Once
}

func (process *modelHostProcess) HealthEndpoint() string { return process.endpoint }
func (process *modelHostProcess) Wait() error {
	<-process.stopped
	return nil
}
func (process *modelHostProcess) Stop(context.Context) error {
	process.once.Do(func() {
		close(process.stopped)
		process.launcher.mu.Lock()
		process.launcher.stops++
		process.launcher.mu.Unlock()
	})
	return nil
}

type modelHostProtocolNegotiator struct{}

func (modelHostProtocolNegotiator) Negotiate(_ context.Context, _ string, request serviceedges.ModelHostProtocolNegotiationRequest) (serviceedges.ModelHostProtocolNegotiationResult, error) {
	return serviceedges.ModelHostProtocolNegotiationResult{ProtocolVersion: "localai-backend-v1", Backend: request.Backend, Ready: true}, nil
}

type modelHostCompatibilityChecker struct{}

func (modelHostCompatibilityChecker) Check(context.Context, serviceedges.ModelHostCompatibilityRequest) error {
	return nil
}

type rejectingAssetHTTP struct{}

func (rejectingAssetHTTP) Do(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected model asset network request")
}

type modelAssetFileSystem struct{ home string }

func (filesystem modelAssetFileSystem) MkdirAll(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (filesystem modelAssetFileSystem) Stat(path string) (os.FileInfo, error) { return os.Stat(path) }
func (filesystem modelAssetFileSystem) UserHomeDir() (string, error)          { return filesystem.home, nil }
func (filesystem modelAssetFileSystem) WriteFile(path string, data []byte, mode os.FileMode) error {
	return os.WriteFile(path, data, mode)
}
func (filesystem modelAssetFileSystem) Rename(oldPath, newPath string) error {
	return os.Rename(oldPath, newPath)
}
func (filesystem modelAssetFileSystem) Remove(path string) error { return os.Remove(path) }
func (filesystem modelAssetFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}
func (filesystem modelAssetFileSystem) ReadDir(path string) ([]os.DirEntry, error) {
	return os.ReadDir(path)
}
func (filesystem modelAssetFileSystem) Create(path string) (io.WriteCloser, error) {
	return os.Create(path)
}
func (filesystem modelAssetFileSystem) Open(path string) (io.ReadCloser, error) { return os.Open(path) }

func writeBuiltinModelCache(t *testing.T, home string) {
	t.Helper()
	name := "gemma-4-E4B-it-Q4_K_M.gguf"
	body := []byte("functional model fixture")
	digest := fmt.Sprintf("%x", sha256.Sum256(body))
	identity := fmt.Sprintf("model|%s|%s:%d:%s", omniModelSource, name, len(body), digest)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", ".you-content-addressed", "model", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create model snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, name), body, 0o644); err != nil {
		t.Fatalf("write model snapshot: %v", err)
	}
	metadata := map[string]any{
		"kind": "model", "identity": identity, "source": omniModelSource, "sourceKey": omniModelSource,
		"artifacts": []map[string]any{{"Name": name, "Bytes": len(body), "SHA256": digest}},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal model metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), data, 0o644); err != nil {
		t.Fatalf("write model metadata: %v", err)
	}
}

func llamaBackendSelection() serviceedges.ModelBackendArtifactSelection {
	return serviceedges.ModelBackendArtifactSelection{
		Name:     "localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Location: "https://github.com/portpowered/infinite-you/releases/download/localai-backends-v1-374fb240161479665f1e4d2c422dbe152f7eb585fc4ee82dabd182517feae2f1/localai-backend-localai-llamacpp-linux-amd64-6b4dc2116a92c5c8f2782bfe51fabe5ee66fb5ef.tar.gz",
		Bytes:    28,
		SHA256:   "9285e7ffc76aaadf4dfcc6b2de5e23c6b01d4e7068e8f2dd65673626cc5de4ed",
	}
}

func writeBackendCache(t *testing.T, home string, selection serviceedges.ModelBackendArtifactSelection) {
	t.Helper()
	urlHash := fmt.Sprintf("%x", sha256.Sum256([]byte(selection.Location)))
	source := "backend://localai-llamacpp/release://" + urlHash
	identity := fmt.Sprintf("backend|%s|%s:%d:%s", source, selection.Name, selection.Bytes, selection.SHA256)
	identityHash := fmt.Sprintf("%x", sha256.Sum256([]byte(identity)))
	snapshot := filepath.Join(home, ".agent-factory", "models", "backend-artifacts", ".you-content-addressed", "backend", identityHash)
	if err := os.MkdirAll(snapshot, 0o755); err != nil {
		t.Fatalf("create backend snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, selection.Name), []byte("localai-llamacpp/linux-amd64"), 0o644); err != nil {
		t.Fatalf("write backend snapshot: %v", err)
	}
	metadata := map[string]any{
		"kind": "backend", "identity": identity, "source": source, "sourceKey": source,
		"artifacts": []map[string]any{{"Name": selection.Name, "Bytes": selection.Bytes, "SHA256": selection.SHA256}},
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal backend metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot, ".you-assets.json"), data, 0o644); err != nil {
		t.Fatalf("write backend metadata: %v", err)
	}
}

func functionalHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return append(os.Environ(), "USERPROFILE="+home)
	}
	if runtime.GOOS == "plan9" {
		return append(os.Environ(), "home="+home)
	}
	return append(os.Environ(), "HOME="+home)
}

func requiredString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
