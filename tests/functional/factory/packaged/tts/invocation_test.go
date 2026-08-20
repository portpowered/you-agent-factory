package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedTTSFakeAudioFixture = "RIFF....WAVEpayload"

// TestPackagedTTSRequiredInputProducesAudioArtifactMetadata proves that invoking
// the packaged @you/tts Factory with required text input completes under a fake
// model edge and returns public primary-result audio artifact metadata for
// the successful TTS outcome.
func TestPackagedTTSRequiredInputProducesAudioArtifactMetadata(t *testing.T) {
	text := fmt.Sprintf(
		"functional packaged tts required input %d",
		time.Now().UnixNano(),
	)

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	overwritePackagedTTSFactoryWithProviderFakeTopology(t, factoryDir)

	fakeProvider := newPackagedTTSFakeProvider([]byte(packagedTTSFakeAudioFixture))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + homeDir,
			"USERPROFILE=" + homeDir,
		},
		Edges: serviceedges.Edges{
			ProviderOverride: fakeProvider,
		},
	})

	response := postPackagedTTSInvocation(t, server, text)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one metadata text part", response.PrimaryResult)
	}

	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}

	var metadata factorydefinitions.TTSInvocationMetadata
	if err := json.Unmarshal([]byte(part.Text), &metadata); err != nil {
		t.Fatalf("metadata JSON: %v; text = %q", err, part.Text)
	}
	if strings.TrimSpace(metadata.ArtifactPath) == "" {
		t.Fatalf("artifactPath is empty, want non-empty audio artifact path")
	}
	if metadata.MediaType != "audio/wav" {
		t.Fatalf("mediaType = %q, want audio/wav", metadata.MediaType)
	}
	wantBackend := factorydefinitions.DefaultTTSModelName + "/" + factorydefinitions.DefaultTTSBackendName
	if metadata.Backend != wantBackend {
		t.Fatalf("backend = %q, want %q", metadata.Backend, wantBackend)
	}

	audioBytes, err := os.ReadFile(metadata.ArtifactPath)
	if err != nil {
		t.Fatalf("read artifact audio file %q: %v", metadata.ArtifactPath, err)
	}
	if string(audioBytes) != packagedTTSFakeAudioFixture {
		t.Fatalf("artifact audio = %q, want fake model fixture payload", audioBytes)
	}
	if fakeProvider.callCount() == 0 {
		t.Fatal("fake provider Infer was not called, want packaged factory inference to reach the fake model edge")
	}
}

// TestFactoryTTSSuccessProjectsAudioWorkAndEvents proves the direct Factory
// TTS dispatch contract at the root Process boundary. The fixture deliberately
// uses a non-packaged workstation name so the generic Factory result remains
// the AUDIO Work emitted by the worker rather than the packaged invocation's
// metadata-only primary-result presentation.
func TestFactoryTTSSuccessProjectsAudioWorkAndEvents(t *testing.T) {
	text := "functional factory tts audio projection"
	dir := scaffoldFactoryTTSAudioDispatch(t)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:        "tts request",
		WorkTypeID:  "task",
		TargetState: "init",
		Content: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText,
			Text: text,
			Slot: "text",
		}},
	})

	provider := newPackagedTTSFakeProvider([]byte(packagedTTSFakeAudioFixture))
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		30*time.Second,
	)

	if provider.callCount() != 1 {
		t.Fatalf("TTS provider call count = %d, want one", provider.callCount())
	}
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}

	outputWork := factoryTTSCompletedWork(t, listed)
	audio := factoryTTSAudioPart(t, outputWork)
	audioPath := provider.lastAudioPath()
	if audioPath == "" {
		t.Fatal("provider audio path is empty, want deterministic backend artifact path")
	}
	if audio.File == nil || *audio.File != audioPath {
		t.Fatalf("AUDIO Work file = %#v, want provider artifact path %q", audio.File, audioPath)
	}
	if audio.Slot == nil || *audio.Slot != "audio" {
		t.Fatalf("AUDIO Work slot = %#v, want audio", audio.Slot)
	}
	if audio.ContentType == nil || *audio.ContentType != "audio/wav" {
		t.Fatalf("AUDIO Work contentType = %#v, want audio/wav", audio.ContentType)
	}
	content, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read TTS artifact %q: %v", audioPath, err)
	}
	if string(content) != packagedTTSFakeAudioFixture {
		t.Fatalf("TTS artifact = %q, want deterministic fixture payload", content)
	}

	if !session.IsDefault {
		t.Fatalf("Factory Session = %#v, want default session", session)
	}
	assertFactoryTTSSuccessEvents(t, events, factorysessions.DefaultSessionID, outputWork, text, audio)
}

func factoryTTSCompletedWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
) factoryapi.Work {
	t.Helper()
	var completed []factoryapi.Work
	for _, candidate := range listed.Results {
		if candidate.WorkTypeName == nil || *candidate.WorkTypeName != "task" ||
			candidate.State == nil || candidate.State.Name != "complete" {
			continue
		}
		completed = append(completed, candidate)
	}
	if len(completed) != 1 {
		t.Fatalf("completed TTS Work = %#v, want one task:complete item", completed)
	}
	if completed[0].WorkId == nil || strings.TrimSpace(*completed[0].WorkId) == "" {
		t.Fatalf("completed TTS Work id = %#v, want non-empty identity", completed[0].WorkId)
	}
	return completed[0]
}

func factoryTTSAudioPart(t *testing.T, item factoryapi.Work) factoryapi.WorkAudioContentPart {
	t.Helper()
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("completed TTS Work content = %#v, want one AUDIO part", item.Content)
	}
	audio, err := (*item.Content)[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("completed TTS Work content as AUDIO = %v; content = %#v", err, item.Content)
	}
	if audio.Type != factoryapi.WorkContentPartTypeAudio {
		t.Fatalf("completed TTS Work content type = %q, want AUDIO", audio.Type)
	}
	if audio.File == nil || strings.TrimSpace(*audio.File) == "" {
		t.Fatalf("completed TTS Work AUDIO = %#v, want artifact reference", audio)
	}
	return audio
}

type factoryTTSSuccessEvents struct {
	workRequest      *factoryapi.FactoryEvent
	dispatchRequest  *factoryapi.FactoryEvent
	modelRequest     *factoryapi.FactoryEvent
	modelResponse    *factoryapi.FactoryEvent
	dispatchResponse *factoryapi.FactoryEvent
}

func assertFactoryTTSSuccessEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	outputWork factoryapi.Work,
	wantText string,
	wantAudio factoryapi.WorkAudioContentPart,
) {
	t.Helper()
	observed := collectFactoryTTSSuccessEvents(t, events, sessionID)
	workID := *outputWork.WorkId
	requestID := factoryTTSRequiredContextID(t, observed.workRequest, "request")
	traceID := factoryTTSRequiredTraceID(t, observed.workRequest)
	dispatchID := factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch")
	assertFactoryTTSContextCorrelation(t, observed, workID, requestID, traceID, dispatchID)
	assertFactoryTTSWorkRequest(t, observed.workRequest, workID, wantText)
	assertFactoryTTSDispatchRequest(t, observed.dispatchRequest, workID)
	assertFactoryTTSModelEvents(t, observed, wantAudio)
	assertFactoryTTSDispatchResponse(t, observed.dispatchResponse, workID, wantAudio)
}

func collectFactoryTTSSuccessEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
) factoryTTSSuccessEvents {
	t.Helper()
	var observed factoryTTSSuccessEvents
	for index := range events {
		event := &events[index]
		if strings.TrimSpace(event.Id) == "" {
			t.Fatalf("Factory Event[%d] id is empty", index)
		}
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event[%d] session id = %#v, want %q", index, event.Context.SessionId, sessionID)
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			factoryTTSRequireSessionID(t, event, sessionID)
			observed.workRequest = event
		case factoryapi.FactoryEventTypeDispatchRequest:
			factoryTTSRequireSessionID(t, event, sessionID)
			observed.dispatchRequest = event
		case factoryapi.FactoryEventTypeModelRequest:
			factoryTTSRequireSessionID(t, event, sessionID)
			observed.modelRequest = event
		case factoryapi.FactoryEventTypeModelResponse:
			factoryTTSRequireSessionID(t, event, sessionID)
			observed.modelResponse = event
		case factoryapi.FactoryEventTypeDispatchResponse:
			factoryTTSRequireSessionID(t, event, sessionID)
			observed.dispatchResponse = event
		}
	}
	if observed.workRequest == nil || observed.dispatchRequest == nil || observed.modelRequest == nil ||
		observed.modelResponse == nil || observed.dispatchResponse == nil {
		t.Fatalf("TTS Factory Events missing required request/dispatch/model/response records: %#v", observed)
	}
	return observed
}

func assertFactoryTTSContextCorrelation(
	t *testing.T,
	events factoryTTSSuccessEvents,
	workID, requestID, traceID, dispatchID string,
) {
	t.Helper()
	if got := factoryTTSContextWorkIDs(events.dispatchRequest); len(got) != 1 || got[0] != workID {
		t.Fatalf("DISPATCH_REQUEST work ids = %#v, want [%q]", got, workID)
	}
	for _, event := range []*factoryapi.FactoryEvent{events.dispatchRequest, events.modelRequest, events.modelResponse} {
		if got := factoryTTSRequiredContextID(t, event, "dispatch"); got != dispatchID {
			t.Fatalf("%s dispatch id = %q, want %q", event.Type, got, dispatchID)
		}
		if got := factoryTTSRequiredContextID(t, event, "request"); got != requestID {
			t.Fatalf("%s request id = %q, want %q", event.Type, got, requestID)
		}
		if got := factoryTTSContextWorkIDs(event); len(got) != 1 || got[0] != workID {
			t.Fatalf("%s work ids = %#v, want [%q]", event.Type, got, workID)
		}
		if got := factoryTTSContextTraceIDs(event); len(got) != 1 || got[0] != traceID {
			t.Fatalf("%s trace ids = %#v, want [%q]", event.Type, got, traceID)
		}
	}
	response := events.dispatchResponse
	if response.Context.DispatchId == nil || *response.Context.DispatchId != dispatchID {
		t.Fatalf("DISPATCH_RESPONSE dispatch id = %#v, want %q", response.Context.DispatchId, dispatchID)
	}
	if response.Context.RequestId != nil && *response.Context.RequestId != requestID {
		t.Fatalf("DISPATCH_RESPONSE request id = %q, want %q when present", *response.Context.RequestId, requestID)
	}
	if got := factoryTTSContextWorkIDs(response); len(got) != 1 || got[0] != workID {
		t.Fatalf("DISPATCH_RESPONSE work ids = %#v, want [%q]", got, workID)
	}
	if got := factoryTTSContextTraceIDs(response); len(got) > 0 && (len(got) != 1 || got[0] != traceID) {
		t.Fatalf("DISPATCH_RESPONSE trace ids = %#v, want [%q] when present", got, traceID)
	}
}

func assertFactoryTTSWorkRequest(t *testing.T, event *factoryapi.FactoryEvent, workID, wantText string) {
	t.Helper()
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode WORK_REQUEST %q: %v", event.Id, err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || payload.Works == nil || len(*payload.Works) != 1 {
		t.Fatalf("WORK_REQUEST payload = %#v, want one factory request work", payload)
	}
	requestedWork := (*payload.Works)[0]
	if requestedWork.WorkId == nil || *requestedWork.WorkId != workID || requestedWork.WorkTypeName == nil || *requestedWork.WorkTypeName != "task" {
		t.Fatalf("WORK_REQUEST work = %#v, want %q task", requestedWork, workID)
	}
	if requestedWork.Content == nil || len(*requestedWork.Content) != 1 {
		t.Fatalf("WORK_REQUEST content = %#v, want one text part", requestedWork.Content)
	}
	requestedText, err := (*requestedWork.Content)[0].AsWorkTextContentPart()
	if err != nil || requestedText.Text != wantText || requestedText.Slot == nil || *requestedText.Slot != "text" {
		t.Fatalf("WORK_REQUEST text = %#v, want text %q in text slot", requestedText, wantText)
	}
}

func assertFactoryTTSDispatchRequest(t *testing.T, event *factoryapi.FactoryEvent, workID string) {
	t.Helper()
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_REQUEST %q: %v", event.Id, err)
	}
	if payload.TransitionId != "tts-dispatch" || len(payload.Inputs) != 1 || payload.Inputs[0].WorkId != workID {
		t.Fatalf("DISPATCH_REQUEST payload = %#v, want tts-dispatch with %q input", payload, workID)
	}
}

func assertFactoryTTSModelEvents(
	t *testing.T,
	events factoryTTSSuccessEvents,
	wantAudio factoryapi.WorkAudioContentPart,
) {
	t.Helper()
	request, err := events.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_REQUEST %q: %v", events.modelRequest.Id, err)
	}
	if request.Operation != "TTS" || request.Worker != "tts-executor" || request.Model != factorydefinitions.DefaultTTSModelName || request.ModelRequestId == "" {
		t.Fatalf("MODEL_REQUEST payload = %#v, want TTS/%s/%s and request identity", request, factorydefinitions.DefaultTTSModelName, "tts-executor")
	}
	response, err := events.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode MODEL_RESPONSE %q: %v", events.modelResponse.Id, err)
	}
	if response.Operation != "TTS" || response.Outcome != factoryapi.InferenceOutcomeSucceeded || response.ModelRequestId != request.ModelRequestId {
		t.Fatalf("MODEL_RESPONSE payload = %#v, want successful TTS response correlated to %q", response, request.ModelRequestId)
	}
	if response.OutputContent == nil || len(*response.OutputContent) != 1 {
		t.Fatalf("MODEL_RESPONSE outputContent = %#v, want one text-encoded AUDIO part", response.OutputContent)
	}
	modelText, err := (*response.OutputContent)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("MODEL_RESPONSE output as text: %v", err)
	}
	assertFactoryTTSRawAudioOutput(t, "MODEL_RESPONSE", modelText.Text, wantAudio)
}

func assertFactoryTTSDispatchResponse(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	workID string,
	wantAudio factoryapi.WorkAudioContentPart,
) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode DISPATCH_RESPONSE %q: %v", event.Id, err)
	}
	if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != "tts-dispatch" {
		t.Fatalf("DISPATCH_RESPONSE payload = %#v, want accepted TTS response", payload)
	}
	if payload.Output == nil {
		t.Fatal("DISPATCH_RESPONSE output is nil, want serialized AUDIO payload")
	}
	assertFactoryTTSRawAudioOutput(t, "DISPATCH_RESPONSE", *payload.Output, wantAudio)
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("DISPATCH_RESPONSE outputWork = %#v, want one output Work", payload.OutputWork)
	}
	responseWork := (*payload.OutputWork)[0]
	if responseWork.WorkId == nil || *responseWork.WorkId != workID || responseWork.State == nil || responseWork.State.Name != "complete" {
		t.Fatalf("DISPATCH_RESPONSE output Work = %#v, want %q in complete", responseWork, workID)
	}
	responseAudio := factoryTTSAudioPart(t, responseWork)
	assertFactoryTTSAudioShape(t, responseAudio, wantAudio, "DISPATCH_RESPONSE")
}

func assertFactoryTTSRawAudioOutput(
	t *testing.T,
	label, raw string,
	wantAudio factoryapi.WorkAudioContentPart,
) {
	t.Helper()
	var output []work.WorkContentPart
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		t.Fatalf("decode %s output: %v; output = %q", label, err, raw)
	}
	if len(output) != 1 || output[0].Type != work.WorkContentPartTypeAudio ||
		output[0].File != *wantAudio.File || output[0].Slot != *wantAudio.Slot ||
		output[0].ContentType != *wantAudio.ContentType {
		t.Fatalf("%s serialized output = %#v, want one correlated AUDIO part", label, output)
	}
}

func factoryTTSRequireSessionID(t *testing.T, event *factoryapi.FactoryEvent, want string) {
	t.Helper()
	if event.Context.SessionId == nil || *event.Context.SessionId != want {
		t.Fatalf("%s session id = %#v, want %q", event.Type, event.Context.SessionId, want)
	}
}

func assertFactoryTTSAudioShape(
	t *testing.T,
	got factoryapi.WorkAudioContentPart,
	want factoryapi.WorkAudioContentPart,
	label string,
) {
	t.Helper()
	if got.Type != factoryapi.WorkContentPartTypeAudio || got.Url != want.Url ||
		got.ContentType == nil || want.ContentType == nil || *got.ContentType != *want.ContentType ||
		got.Slot == nil || want.Slot == nil || *got.Slot != *want.Slot ||
		got.File == nil || want.File == nil || *got.File != *want.File {
		t.Fatalf("%s AUDIO shape = %#v, want %#v", label, got, want)
	}
}

func factoryTTSRequiredContextID(t *testing.T, event *factoryapi.FactoryEvent, name string) string {
	t.Helper()
	var value *string
	switch name {
	case "request":
		value = event.Context.RequestId
	case "dispatch":
		value = event.Context.DispatchId
	default:
		t.Fatalf("unknown Factory Event context id %q", name)
	}
	if value == nil || strings.TrimSpace(*value) == "" {
		t.Fatalf("%s %s id = %#v, want non-empty identity", event.Type, name, value)
	}
	return *value
}

func factoryTTSRequiredTraceID(t *testing.T, event *factoryapi.FactoryEvent) string {
	t.Helper()
	ids := factoryTTSContextTraceIDs(event)
	if len(ids) != 1 || strings.TrimSpace(ids[0]) == "" {
		t.Fatalf("%s trace ids = %#v, want one non-empty trace", event.Type, ids)
	}
	return ids[0]
}

func factoryTTSContextWorkIDs(event *factoryapi.FactoryEvent) []string {
	if event == nil || event.Context.WorkIds == nil {
		return nil
	}
	return append([]string(nil), (*event.Context.WorkIds)...)
}

func factoryTTSContextTraceIDs(event *factoryapi.FactoryEvent) []string {
	if event == nil || event.Context.TraceIds == nil {
		return nil
	}
	return append([]string(nil), (*event.Context.TraceIds)...)
}

// TestPackagedTTSOptionalVoiceAndFormatReachModel proves that optional voice and
// format options supplied through the public packaged-factory invocation
// surface reach the fake model edge on resolved TTS operation bindings.
func TestPackagedTTSOptionalVoiceAndFormatReachModel(t *testing.T) {
	text := fmt.Sprintf(
		"functional packaged tts optional voice format %d",
		time.Now().UnixNano(),
	)
	voice := "alloy"
	format := "mp3"

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	overwritePackagedTTSFactoryWithOptionalVoiceAndFormatTopology(t, factoryDir)

	fakeProvider := newPackagedTTSFakeProvider([]byte(packagedTTSFakeAudioFixture))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + homeDir,
			"USERPROFILE=" + homeDir,
		},
		Edges: serviceedges.Edges{
			ProviderOverride: fakeProvider,
		},
	})

	response := postPackagedTTSInvocationWithArgs(t, server, map[string]any{
		"text":   text,
		"voice":  voice,
		"format": format,
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}

	request := fakeProvider.lastRequest()
	if request == nil {
		t.Fatal("fake provider Infer was not called, want packaged factory inference to reach the fake model edge")
	}
	if voiceBinding, ok := modelBindingJSON(request.ModelBindings, "voice"); !ok {
		t.Fatalf("model bindings = %#v, want voice slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(voiceBinding, "name"); got != voice {
		t.Fatalf("voice binding name = %q, want %q; binding = %s", got, voice, voiceBinding)
	}
	if formatBinding, ok := modelBindingJSON(request.ModelBindings, "format"); !ok {
		t.Fatalf("model bindings = %#v, want format slot binding", request.ModelBindings)
	} else if got := stringValueFromBindingJSON(formatBinding, "name"); got != format {
		t.Fatalf("format binding name = %q, want %q; binding = %s", got, format, formatBinding)
	}
}

// TestPackagedTTSModelFailureReturnsNoFalseArtifact proves that a forced model
// failure during packaged @you/tts invocation returns a failed public terminal
// outcome without success-shaped TTS audio artifact metadata in the primary
// result for that run.
func TestPackagedTTSModelFailureReturnsNoFalseArtifact(t *testing.T) {
	text := fmt.Sprintf(
		"functional packaged tts model failure %d",
		time.Now().UnixNano(),
	)

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	overwritePackagedTTSFactoryWithProviderFakeTopology(t, factoryDir)

	fakeProvider := newPackagedTTSFailingFakeProvider("omnivoice invoke failed: exit status 1")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Env: []string{
			"HOME=" + homeDir,
			"USERPROFILE=" + homeDir,
		},
		Edges: serviceedges.Edges{
			ProviderOverride: fakeProvider,
		},
	})

	response := postPackagedTTSInvocation(t, server, text)
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONTTSGENERATIONFAILED {
		t.Fatalf(
			"invocation errorCode = %#v, want INVOCATION_TTS_GENERATION_FAILED",
			response.ErrorCode,
		)
	}
	if primaryResultContainsTTSArtifactMetadata(t, response.PrimaryResult) {
		t.Fatalf(
			"primary result = %#v, want no success-shaped TTS audio artifact metadata on model failure",
			response.PrimaryResult,
		)
	}
	if fakeProvider.callCount() == 0 {
		t.Fatal("fake provider Infer was not called, want packaged factory inference to reach the fake model edge")
	}
}

type packagedTTSFakeProvider struct {
	testutil.ProviderServiceAdapter
	mu        sync.Mutex
	audio     []byte
	audioPath string
	calls     int
	last      *workerexecution.ProviderInferenceRequest
}

func newPackagedTTSFakeProvider(audio []byte) *packagedTTSFakeProvider {
	provider := &packagedTTSFakeProvider{audio: append([]byte(nil), audio...)}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	return provider
}

func (provider *packagedTTSFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *packagedTTSFakeProvider) lastAudioPath() string {
	if provider == nil {
		return ""
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.audioPath
}

func (provider *packagedTTSFakeProvider) lastRequest() *workerexecution.ProviderInferenceRequest {
	if provider == nil {
		return nil
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if provider.last == nil {
		return nil
	}
	cloned := workerexecution.CloneProviderInferenceRequest(*provider.last)
	return &cloned
}

func (provider *packagedTTSFakeProvider) Infer(
	_ context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	provider.mu.Lock()
	provider.calls++
	cloned := workerexecution.CloneProviderInferenceRequest(request)
	provider.last = &cloned
	provider.mu.Unlock()
	if strings.TrimSpace(request.ModelOperation) != "TTS" {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"packaged tts fake provider unexpected operation %q",
			request.ModelOperation,
		)
	}

	outputFile := filepath.Join(os.TempDir(), fmt.Sprintf("packaged-tts-fake-%d.wav", time.Now().UnixNano()))
	if err := os.WriteFile(outputFile, provider.audio, 0o644); err != nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("write fake tts audio artifact: %w", err)
	}
	provider.mu.Lock()
	provider.audioPath = outputFile
	provider.mu.Unlock()

	encoded, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        outputFile,
		ContentType: "audio/wav",
		Slot:        "audio",
	}})
	if err != nil {
		return workerexecution.InferenceResponse{}, fmt.Errorf("marshal fake tts audio content: %w", err)
	}
	return workerexecution.InferenceResponse{
		Content: string(encoded),
		Diagnostics: &workerexecution.WorkDiagnostics{
			Metadata: map[string]string{
				workerexecution.ProviderResponseMetadataCompletionEvidence: "provider_response",
			},
		},
	}, nil
}

type packagedTTSFailingFakeProvider struct {
	testutil.ProviderServiceAdapter
	mu          sync.Mutex
	calls       int
	last        *workerexecution.ProviderInferenceRequest
	failMessage string
}

func newPackagedTTSFailingFakeProvider(message string) *packagedTTSFailingFakeProvider {
	provider := &packagedTTSFailingFakeProvider{failMessage: message}
	provider.ProviderServiceAdapter.InferFunc = provider.Infer
	return provider
}

func (provider *packagedTTSFailingFakeProvider) callCount() int {
	if provider == nil {
		return 0
	}
	provider.mu.Lock()
	defer provider.mu.Unlock()
	return provider.calls
}

func (provider *packagedTTSFailingFakeProvider) Infer(
	_ context.Context,
	request workerexecution.ProviderInferenceRequest,
) (workerexecution.InferenceResponse, error) {
	provider.mu.Lock()
	provider.calls++
	cloned := workerexecution.CloneProviderInferenceRequest(request)
	provider.last = &cloned
	provider.mu.Unlock()
	if strings.TrimSpace(request.ModelOperation) != "TTS" {
		return workerexecution.InferenceResponse{}, fmt.Errorf(
			"packaged tts failing fake provider unexpected operation %q",
			request.ModelOperation,
		)
	}
	return workerexecution.InferenceResponse{}, errors.New(provider.failMessage)
}

// overwritePackagedTTSFactoryWithProviderFakeTopology keeps the installed
// @you/tts layout but replaces authored topology with a cloud-backed inference
// worker that reaches the provider override fake edge.
func overwritePackagedTTSFactoryWithProviderFakeTopology(t *testing.T, factoryDir string) {
	overwritePackagedTTSFactoryTopology(t, factoryDir, scaffoldPackagedTTSLikeFactory)
}

func overwritePackagedTTSFactoryWithOptionalVoiceAndFormatTopology(t *testing.T, factoryDir string) {
	overwritePackagedTTSFactoryTopology(t, factoryDir, scaffoldPackagedTTSLikeFactoryWithOptionalVoiceAndFormat)
}

func overwritePackagedTTSFactoryTopology(
	t *testing.T,
	factoryDir string,
	scaffoldFactory func(*testing.T) string,
) {
	t.Helper()

	scaffoldDir := scaffoldFactory(t)
	scaffoldConfig, err := os.ReadFile(filepath.Join(scaffoldDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read scaffold factory.json: %v", err)
	}

	var scaffold map[string]any
	if err := json.Unmarshal(scaffoldConfig, &scaffold); err != nil {
		t.Fatalf("unmarshal scaffold factory.json: %v", err)
	}
	scaffold["id"] = factorydefinitions.PackagedTTSFactoryProject
	scaffold["name"] = "tts"

	updated, err := json.MarshalIndent(scaffold, "", "  ")
	if err != nil {
		t.Fatalf("marshal scaffold factory.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile), updated, 0o644); err != nil {
		t.Fatalf("write installed factory.json: %v", err)
	}

	for _, name := range []string{"factory.yaml", "factory.yml"} {
		path := filepath.Join(factoryDir, name)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			t.Fatalf("remove %s: %v", name, err)
		}
	}
	if err := os.RemoveAll(filepath.Join(factoryDir, "workers")); err != nil {
		t.Fatalf("remove installed worker sidecars: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(factoryDir, "workstations")); err != nil {
		t.Fatalf("remove installed workstation sidecars: %v", err)
	}
}

func scaffoldPackagedTTSLikeFactory(t *testing.T) string {
	return scaffoldTTSLikeFactory(t, factorydefinitions.PackagedTTSInvokeWorkstationName)
}

func scaffoldFactoryTTSAudioDispatch(t *testing.T) string {
	return scaffoldTTSLikeFactory(t, "tts-dispatch")
}

func scaffoldTTSLikeFactory(t *testing.T, workstationName string) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "tts",
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      factorydefinitions.DefaultTTSModelName,
			"backend":    factorydefinitions.DefaultTTSBackendName,
			"loadPolicy": "ON_DEMAND",
		}},
		"workTypes": []map[string]any{{
			"name": "task",
			"handlingBehavior": []string{
				factorydefinitions.WorkTypeHandlingBehaviorDefault,
			},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-executor",
			"type":          factorydefinitions.WorkerTypeInference,
			"model":         factorydefinitions.DefaultTTSModelName,
			"modelLocality": factorydefinitions.ModelLocalityCloud,
			"modelProvider": "CODEX",
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"contentTypes": []string{"TEXT"},
					"required":     true,
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{"AUDIO"},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      workstationName,
			"type":      "INFERENCE_RUN",
			"operation": "TTS",
			"worker":    "tts-executor",
			"operationBindings": []map[string]any{{
				"slot": "text",
				"selector": map[string]any{
					"type": "TEXT",
				},
			}},
			"inputs": []map[string]string{{
				"workType": "task",
				"state":    "init",
			}},
			"outputs": []map[string]string{{
				"workType": "task",
				"state":    "complete",
			}},
			"onFailure": []map[string]string{{
				"workType": "task",
				"state":    "failed",
			}},
		}},
	})
}

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func scaffoldPackagedTTSLikeFactoryWithOptionalVoiceAndFormat(t *testing.T) string {
	t.Helper()
	return support.ScaffoldFactory(t, map[string]any{
		"name": "tts",
		"invocationSignature": map[string]any{
			"parameters": []map[string]any{
				{
					"name":     "text",
					"required": true,
					"bindings": []map[string]any{
						{"kind": "POSITIONAL", "position": 1},
						{"kind": "STDIN"},
					},
				},
				{
					"name":         "voice",
					"externalName": "voice",
					"typeHint":     "STRING",
					"bindings":     []map[string]any{{"kind": "NAMED"}},
				},
				{
					"name":         "format",
					"externalName": "format",
					"typeHint":     "STRING",
					"bindings":     []map[string]any{{"kind": "NAMED"}},
				},
			},
		},
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      factorydefinitions.DefaultTTSModelName,
			"backend":    factorydefinitions.DefaultTTSBackendName,
			"loadPolicy": "ON_DEMAND",
		}},
		"workTypes": []map[string]any{{
			"name": "task",
			"handlingBehavior": []string{
				factorydefinitions.WorkTypeHandlingBehaviorDefault,
			},
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":          "tts-executor",
			"type":          factorydefinitions.WorkerTypeInference,
			"model":         factorydefinitions.DefaultTTSModelName,
			"modelLocality": factorydefinitions.ModelLocalityCloud,
			"modelProvider": "CODEX",
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{
					{
						"name":         "text",
						"contentTypes": []string{"TEXT"},
						"required":     true,
					},
					{
						"name":         "voice",
						"contentTypes": []string{"JSON"},
					},
					{
						"name":         "format",
						"contentTypes": []string{"JSON"},
					},
				},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{"AUDIO"},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      "execute-tts",
			"type":      "INFERENCE_RUN",
			"operation": "TTS",
			"worker":    "tts-executor",
			"operationBindings": []map[string]any{
				{
					"slot": "text",
					"selector": map[string]any{
						"type": "TEXT",
					},
				},
				{
					"slot": "voice",
					"config": []map[string]any{{
						"type": "JSON",
						"role": "voice",
						"json": map[string]any{"name": "${voice}"},
					}},
				},
				{
					"slot": "format",
					"config": []map[string]any{{
						"type": "JSON",
						"role": "format",
						"json": map[string]any{"name": "${format}"},
					}},
				},
			},
			"inputs": []map[string]string{{
				"workType": "task",
				"state":    "init",
			}},
			"outputs": []map[string]string{{
				"workType": "task",
				"state":    "complete",
			}},
			"onFailure": []map[string]string{{
				"workType": "task",
				"state":    "failed",
			}},
		}},
	})
}

func postPackagedTTSInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	text string,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: invocationTextSourceKindPtr(),
		Content:    invocationTextContentPtr(text),
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}

	endpoint := strings.TrimSuffix(server.URL(), "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func postPackagedTTSInvocationWithArgs(
	t *testing.T,
	server *support.FunctionalAPIServer,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(factoryapi.InvocationRequest{Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}

	endpoint := strings.TrimSuffix(server.URL(), "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func modelBindingJSON(
	bindings []workerexecution.ResolvedModelOperationBinding,
	slot string,
) (json.RawMessage, bool) {
	for _, binding := range bindings {
		if binding.Slot != slot || len(binding.Content) == 0 {
			continue
		}
		part := binding.Content[0]
		if len(part.JSON) > 0 {
			return part.JSON, true
		}
	}
	return nil, false
}

func stringValueFromBindingJSON(payload json.RawMessage, key string) string {
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return ""
	}
	value, _ := decoded[key].(string)
	return value
}

func invocationTextSourceKindPtr() *factoryapi.InvocationInputSourceKind {
	sourceKind := factoryapi.InvocationInputSourceKindText
	return &sourceKind
}

func invocationTextContentPtr(text string) *factoryapi.WorkContent {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: text,
	}); err != nil {
		panic(fmt.Sprintf("build invocation text content: %v", err))
	}
	content := factoryapi.WorkContent{part}
	return &content
}

func primaryResultContainsTTSArtifactMetadata(
	t *testing.T,
	primaryResult *factoryapi.WorkContent,
) bool {
	t.Helper()
	if primaryResult == nil || len(*primaryResult) == 0 {
		return false
	}
	for _, part := range *primaryResult {
		textPart, err := part.AsWorkTextContentPart()
		if err != nil {
			continue
		}
		var metadata factorydefinitions.TTSInvocationMetadata
		if err := json.Unmarshal([]byte(textPart.Text), &metadata); err != nil {
			continue
		}
		if strings.TrimSpace(metadata.ArtifactPath) != "" && metadata.MediaType != "" {
			return true
		}
	}
	return false
}
