package tts

import (
	"encoding/json"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func assertPackagedTTSInvocationResponseIdentity(
	t *testing.T,
	response factoryapi.InvocationResponse,
) {
	t.Helper()
	if strings.TrimSpace(response.RequestId) == "" {
		t.Fatalf("invocation response requestId = %q, want non-empty request identity", response.RequestId)
	}
	if strings.TrimSpace(response.TraceId) == "" {
		t.Fatalf("invocation response traceId = %q, want non-empty trace identity", response.TraceId)
	}
	if response.SessionId != nil && *response.SessionId != factorysessions.DefaultSessionID {
		t.Fatalf("invocation response sessionId = %q, want %q when present", *response.SessionId, factorysessions.DefaultSessionID)
	}
	if response.WorkId != nil && strings.TrimSpace(*response.WorkId) == "" {
		t.Fatal("invocation response workId is empty when present")
	}
}

func assertPackagedTTSInvocationResponseIdentityForSession(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantSessionID, wantRequestID string,
) {
	t.Helper()
	assertPackagedTTSInvocationResponseIdentity(t, response)
	if response.SessionId != nil && *response.SessionId != wantSessionID {
		t.Fatalf("invocation response sessionId = %q, want %q when present", *response.SessionId, wantSessionID)
	}
	if strings.TrimSpace(wantRequestID) != "" && response.RequestId != wantRequestID {
		t.Fatalf("invocation response requestId = %q, want %q", response.RequestId, wantRequestID)
	}
}

func assertPackagedTTSProviderRequest(
	t *testing.T,
	request *workerexecution.ProviderInferenceRequest,
	wantText, wantTransition string,
) {
	t.Helper()
	assertPackagedTTSProviderRequestForSession(
		t, request, wantText, wantTransition, factorysessions.DefaultSessionID, "",
	)
}

func assertPackagedTTSProviderRequestForSession(
	t *testing.T,
	request *workerexecution.ProviderInferenceRequest,
	wantText, wantTransition, wantSessionID, wantRequestID string,
) {
	t.Helper()
	if request == nil {
		t.Fatal("provider request is nil, want one captured TTS attempt")
	}
	if request.ModelOperation != "TTS" || request.WorkerType != "tts-executor" {
		t.Fatalf("provider request operation/worker = %q/%q, want TTS/tts-executor", request.ModelOperation, request.WorkerType)
	}
	if request.Model != factorydefinitions.DefaultTTSModelName {
		t.Fatalf("provider request model = %q, want %q", request.Model, factorydefinitions.DefaultTTSModelName)
	}
	if !strings.EqualFold(strings.TrimSpace(request.ModelProvider), "CODEX") {
		t.Fatalf("provider request model provider = %q, want CODEX binding", request.ModelProvider)
	}
	if request.Dispatch.TransitionID != wantTransition {
		t.Fatalf("provider request transition = %q, want %q", request.Dispatch.TransitionID, wantTransition)
	}
	if strings.TrimSpace(request.Dispatch.DispatchID) == "" {
		t.Fatal("provider request dispatch id is empty")
	}
	if len(request.Dispatch.Execution.WorkIDs) != 1 || strings.TrimSpace(request.Dispatch.Execution.WorkIDs[0]) == "" {
		t.Fatalf("provider request work ids = %#v, want one non-empty work identity", request.Dispatch.Execution.WorkIDs)
	}
	correlation := request.Correlation
	for name, value := range map[string]string{
		"factory session": correlation.FactorySessionID,
		"runtime":         correlation.RuntimeID,
		"generation":      correlation.GenerationID,
		"dispatch":        correlation.DispatchID,
		"attempt":         correlation.AttemptID,
		"request":         correlation.RequestID,
		"trace":           correlation.TraceID,
	} {
		if strings.TrimSpace(value) == "" {
			t.Fatalf("provider request %s correlation id is empty: %#v", name, correlation)
		}
	}
	if correlation.FactorySessionID != wantSessionID {
		t.Fatalf("provider request factory session = %q, want %q", correlation.FactorySessionID, wantSessionID)
	}
	if strings.TrimSpace(wantRequestID) != "" && correlation.RequestID != wantRequestID {
		t.Fatalf("provider request correlation request = %q, want %q", correlation.RequestID, wantRequestID)
	}
	if correlation.DispatchID != request.Dispatch.DispatchID {
		t.Fatalf("provider request correlation dispatch = %q, want %q", correlation.DispatchID, request.Dispatch.DispatchID)
	}
	if correlation.RequestID != request.Dispatch.Execution.RequestID {
		t.Fatalf("provider request correlation request = %q, want dispatch request %q", correlation.RequestID, request.Dispatch.Execution.RequestID)
	}
	if correlation.TraceID != request.Dispatch.Execution.TraceID {
		t.Fatalf("provider request correlation trace = %q, want dispatch trace %q", correlation.TraceID, request.Dispatch.Execution.TraceID)
	}

	var textBinding *workerexecution.ResolvedModelOperationBinding
	for index := range request.ModelBindings {
		binding := &request.ModelBindings[index]
		if binding.Slot == "text" {
			textBinding = binding
			break
		}
	}
	if textBinding == nil || len(textBinding.Content) != 1 {
		if providerRequestContainsTextInput(request, wantText) {
			return
		}
		t.Fatalf("provider text binding = %#v, want one resolved content part or an exact dispatched text input; all model bindings = %#v", textBinding, request.ModelBindings)
	}
	part := textBinding.Content[0]
	if part.Type.Normalized() != work.WorkContentPartTypeText || part.Text != wantText {
		t.Fatalf("provider text binding content = %#v, want exact text %q", part, wantText)
	}
}

func providerRequestContainsTextInput(
	request *workerexecution.ProviderInferenceRequest,
	wantText string,
) bool {
	if request == nil {
		return false
	}
	for _, raw := range request.InputTokens {
		var content []work.WorkContentPart
		switch token := raw.(type) {
		case workerexecution.Token:
			content = token.Color.Content
		case *workerexecution.Token:
			if token != nil {
				content = token.Color.Content
			}
		}
		for _, part := range content {
			if part.Type.Normalized() == work.WorkContentPartTypeText && part.Text == wantText {
				return true
			}
		}
	}
	return false
}

func assertPackagedTTSResponseCorrelatesWithEvents(
	t *testing.T,
	response factoryapi.InvocationResponse,
	events []factoryapi.FactoryEvent,
) {
	t.Helper()
	assertPackagedTTSResponseCorrelatesWithEventsForSession(t, response, events, factorysessions.DefaultSessionID)
}

func assertPackagedTTSResponseCorrelatesWithEventsForSession(
	t *testing.T,
	response factoryapi.InvocationResponse,
	events []factoryapi.FactoryEvent,
	sessionID string,
) {
	t.Helper()
	observed := collectFactoryTTSDispatchEvents(t, events, sessionID)
	requestID := factoryTTSRequiredContextID(t, observed.workRequest, "request")
	traceID := factoryTTSRequiredTraceID(t, observed.workRequest)
	if response.RequestId != requestID {
		t.Fatalf("invocation response requestId = %q, want WORK_REQUEST request %q", response.RequestId, requestID)
	}
	if response.TraceId != traceID {
		t.Fatalf("invocation response traceId = %q, want WORK_REQUEST trace %q", response.TraceId, traceID)
	}
	if response.DispatchId != nil {
		dispatchID := factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch")
		if *response.DispatchId != dispatchID {
			t.Fatalf("invocation response dispatchId = %q, want DISPATCH_REQUEST dispatch %q", *response.DispatchId, dispatchID)
		}
	}
}

func packagedTTSCompletedMetadataWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wantArtifactPath, wantTraceID string,
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
		t.Fatalf("packaged TTS completed Work = %#v, want one task:complete item", completed)
	}
	assertPackagedTTSMetadataWork(t, completed[0], wantArtifactPath, wantTraceID, "listed Work")
	return completed[0]
}

func assertPackagedTTSMetadataWork(
	t *testing.T,
	item factoryapi.Work,
	wantArtifactPath, wantTraceID, label string,
) {
	t.Helper()
	if item.WorkId == nil || strings.TrimSpace(*item.WorkId) == "" {
		t.Fatalf("%s Work id = %#v, want non-empty identity", label, item.WorkId)
	}
	if item.State == nil || item.State.Name != "complete" {
		t.Fatalf("%s state = %#v, want complete", label, item.State)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("%s content = %#v, want one packaged metadata text part", label, item.Content)
	}
	textPart, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("%s content as metadata text: %v", label, err)
	}
	var metadata factorydefinitions.TTSInvocationMetadata
	if err := json.Unmarshal([]byte(textPart.Text), &metadata); err != nil {
		t.Fatalf("%s metadata JSON: %v; text = %q", label, err, textPart.Text)
	}
	if metadata.ArtifactPath != wantArtifactPath || metadata.MediaType != "audio/wav" ||
		metadata.Backend != factorydefinitions.DefaultTTSModelName+"/"+factorydefinitions.DefaultTTSBackendName ||
		metadata.TraceID != wantTraceID {
		t.Fatalf("%s metadata = %#v, want artifact %q, audio/wav, backend %q, trace %q", label, metadata, wantArtifactPath, factorydefinitions.DefaultTTSModelName+"/"+factorydefinitions.DefaultTTSBackendName, wantTraceID)
	}
}

func packagedTTSExpectedAudioPart(t *testing.T, artifactPath string) factoryapi.WorkAudioContentPart {
	t.Helper()
	if strings.TrimSpace(artifactPath) == "" {
		t.Fatal("packaged TTS artifact path is empty, want audio witness")
	}
	contentType := "audio/wav"
	file := artifactPath
	slot := "audio"
	return factoryapi.WorkAudioContentPart{
		Type:        factoryapi.WorkContentPartTypeAudio,
		File:        &file,
		ContentType: &contentType,
		Slot:        &slot,
	}
}

func assertPackagedTTSSuccessEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	outputWork factoryapi.Work,
	wantText string,
	wantAudio factoryapi.WorkAudioContentPart,
	wantArtifactPath, wantTraceID string,
) {
	t.Helper()
	assertPackagedTTSSuccessEventsForSession(
		t, events, factorysessions.DefaultSessionID, outputWork, wantText, wantAudio, wantArtifactPath, wantTraceID,
	)
}

func assertPackagedTTSSuccessEventsForSession(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	outputWork factoryapi.Work,
	wantText string,
	wantAudio factoryapi.WorkAudioContentPart,
	wantArtifactPath, wantTraceID string,
) {
	t.Helper()
	observed := collectFactoryTTSDispatchEvents(t, events, sessionID)
	workID := *outputWork.WorkId
	requestID := factoryTTSRequiredContextID(t, observed.workRequest, "request")
	traceID := factoryTTSRequiredTraceID(t, observed.workRequest)
	dispatchID := factoryTTSRequiredContextID(t, observed.dispatchRequest, "dispatch")
	if traceID != wantTraceID {
		t.Fatalf("packaged TTS event trace = %q, want response trace %q", traceID, wantTraceID)
	}
	assertFactoryTTSContextCorrelation(t, observed, workID, requestID, traceID, dispatchID)
	assertPackagedTTSWorkRequest(t, observed.workRequest, workID, wantText)
	assertPackagedTTSDispatchRequest(t, observed.dispatchRequest, workID)
	assertFactoryTTSModelEvents(t, observed, wantAudio)
	assertPackagedTTSDispatchResponse(t, observed.dispatchResponse, workID, wantAudio, wantArtifactPath, wantTraceID)
}

func assertPackagedTTSWorkRequest(t *testing.T, event *factoryapi.FactoryEvent, workID, wantText string) {
	t.Helper()
	payload, err := event.Payload.AsWorkRequestEventPayload()
	if err != nil {
		t.Fatalf("decode packaged WORK_REQUEST %q: %v", event.Id, err)
	}
	if payload.Type != factoryapi.WorkRequestTypeFactoryRequestBatch || payload.Works == nil || len(*payload.Works) != 1 {
		t.Fatalf("packaged WORK_REQUEST payload = %#v, want one factory request work", payload)
	}
	requestedWork := (*payload.Works)[0]
	if requestedWork.WorkId == nil || *requestedWork.WorkId != workID || requestedWork.WorkTypeName == nil || *requestedWork.WorkTypeName != "task" {
		t.Fatalf("packaged WORK_REQUEST work = %#v, want %q task", requestedWork, workID)
	}
	if requestedWork.Content == nil || len(*requestedWork.Content) != 1 {
		t.Fatalf("packaged WORK_REQUEST content = %#v, want one text part", requestedWork.Content)
	}
	requestedText, err := (*requestedWork.Content)[0].AsWorkTextContentPart()
	if err != nil || requestedText.Text != wantText {
		t.Fatalf("packaged WORK_REQUEST text = %#v, want exact text %q", requestedText, wantText)
	}
	if requestedText.Slot != nil && *requestedText.Slot != "text" {
		t.Fatalf("packaged WORK_REQUEST text slot = %q, want text when present", *requestedText.Slot)
	}
}

func assertPackagedTTSDispatchRequest(t *testing.T, event *factoryapi.FactoryEvent, workID string) {
	t.Helper()
	payload, err := event.Payload.AsDispatchRequestEventPayload()
	if err != nil {
		t.Fatalf("decode packaged DISPATCH_REQUEST %q: %v", event.Id, err)
	}
	if payload.TransitionId != "execute-tts" || len(payload.Inputs) != 1 || payload.Inputs[0].WorkId != workID {
		t.Fatalf("packaged DISPATCH_REQUEST payload = %#v, want execute-tts with %q input", payload, workID)
	}
}

func assertPackagedTTSDispatchResponse(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	workID string,
	wantAudio factoryapi.WorkAudioContentPart,
	wantArtifactPath, wantTraceID string,
) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode packaged DISPATCH_RESPONSE %q: %v", event.Id, err)
	}
	if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != "execute-tts" {
		t.Fatalf("packaged DISPATCH_RESPONSE payload = %#v, want accepted execute-tts response", payload)
	}
	if payload.Output == nil {
		t.Fatal("packaged DISPATCH_RESPONSE output is nil, want serialized AUDIO payload")
	}
	assertFactoryTTSRawAudioOutput(t, "packaged DISPATCH_RESPONSE", *payload.Output, wantAudio)
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("packaged DISPATCH_RESPONSE outputWork = %#v, want one metadata Work", payload.OutputWork)
	}
	responseWork := (*payload.OutputWork)[0]
	if responseWork.WorkId == nil || *responseWork.WorkId != workID {
		t.Fatalf("packaged DISPATCH_RESPONSE output Work = %#v, want work %q", responseWork, workID)
	}
	assertPackagedTTSMetadataWork(t, responseWork, wantArtifactPath, wantTraceID, "DISPATCH_RESPONSE output Work")
}

func assertPackagedTTSFailureDispatchResponse(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	workID, wantText, wantMessage string,
) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode packaged failed DISPATCH_RESPONSE %q: %v", event.Id, err)
	}
	if payload.Outcome != factoryapi.WorkOutcomeFailed || payload.TransitionId != "execute-tts" {
		t.Fatalf("packaged failed DISPATCH_RESPONSE payload = %#v, want failed execute-tts response", payload)
	}
	if payload.Error == nil || *payload.Error != wantMessage || payload.FailureDetail == nil ||
		payload.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown || payload.FailureDetail.Message != wantMessage {
		t.Fatalf("packaged failed DISPATCH_RESPONSE failure = error %#v detail %#v, want unknown/%q", payload.Error, payload.FailureDetail, wantMessage)
	}
	if payload.Output != nil {
		t.Fatalf("packaged failed DISPATCH_RESPONSE output = %q, want no serialized AUDIO output", *payload.Output)
	}
	if payload.OutputResources != nil && len(*payload.OutputResources) != 0 {
		t.Fatalf("packaged failed DISPATCH_RESPONSE outputResources = %#v, want no success artifact resources", payload.OutputResources)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("packaged failed DISPATCH_RESPONSE outputWork = %#v, want one onFailure Work", payload.OutputWork)
	}
	responseWork := (*payload.OutputWork)[0]
	if responseWork.WorkId == nil || *responseWork.WorkId != workID {
		t.Fatalf("packaged failed DISPATCH_RESPONSE output Work = %#v, want work %q", responseWork, workID)
	}
	assertPackagedTTSFailedWork(t, responseWork, wantText, "packaged DISPATCH_RESPONSE output Work")
}

func assertPackagedTTSFailedWork(t *testing.T, item factoryapi.Work, wantText, label string) {
	t.Helper()
	if item.State == nil || item.State.Name != "failed" || item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("%s state = %#v, want failed/FAILED", label, item.State)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("%s content = %#v, want one preserved text part and no AUDIO part", label, item.Content)
	}
	textPart, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil || textPart.Text != wantText {
		t.Fatalf("%s content = %#v, want text %q", label, textPart, wantText)
	}
	if textPart.Slot != nil && *textPart.Slot != "text" {
		t.Fatalf("%s text slot = %q, want text when present", label, *textPart.Slot)
	}
}
