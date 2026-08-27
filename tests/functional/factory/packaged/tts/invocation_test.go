package tts

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedTTSFakeAudioFixture = "RIFF....WAVEpayload"

// TestPackagedTTSNoServerPromptUsesCanonicalInputContract proves that the
// customer-facing named invocation renders the installed workstation prompt
// with the canonical per-input Work data while preserving the complete text
// binding that the TTS operation consumes.
func TestPackagedTTSNoServerPromptUsesCanonicalInputContract(t *testing.T) {
	text := "The release is ready, with the complete bound sentence preserved."
	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(
		t,
		homeDir,
		factorydefinitions.PackagedTTSFactoryName,
	)
	factoryDir = support.CopyFactoryAsNamed(t, factoryDir, homeDir, "@test/tts")
	overwritePackagedTTSFactoryWithCommandRunnerTopology(t, factoryDir)
	factoryName := "@test/tts"
	audioPath := filepath.Join(t.TempDir(), "packaged-tts-command-runner.wav")
	if err := os.WriteFile(audioPath, []byte(packagedTTSFakeAudioFixture), 0o644); err != nil {
		t.Fatalf("write command-runner audio fixture: %v", err)
	}
	audioContent, err := json.Marshal([]work.WorkContentPart{{
		Type:        work.WorkContentPartTypeAudio,
		File:        audioPath,
		ContentType: "audio/wav",
		Slot:        "audio",
	}})
	if err != nil {
		t.Fatalf("marshal command-runner audio content: %v", err)
	}
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{Stdout: audioContent})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", factoryName,
		"--no-record",
		"--output", "primary",
		"--to", text,
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()

	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(packaged TTS) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	assertPackagedTTSInvocationResponseIdentity(t, response)

	if runner.CallCount() != 1 {
		t.Fatalf("provider command call count = %d, want one named packaged invocation", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "codex" {
		t.Fatalf("provider command = %q, want codex", request.Command)
	}
	wantArgs := []string{"exec", "--json", "--model", factorydefinitions.DefaultTTSModelName, "-"}
	if !reflect.DeepEqual(request.Args, wantArgs) {
		t.Fatalf("provider command args = %#v, want %#v", request.Args, wantArgs)
	}
	prompt := string(request.Stdin)
	const promptPrefix = "For Work "
	if !strings.HasPrefix(prompt, promptPrefix) {
		t.Fatalf("rendered provider prompt = %q, want installed prompt with consumed WorkID", prompt)
	}
	comma := strings.Index(prompt[len(promptPrefix):], ",")
	if comma <= 0 {
		t.Fatalf("rendered provider prompt = %q, want non-empty consumed WorkID", prompt)
	}
	workID := strings.TrimSpace(prompt[len(promptPrefix) : len(promptPrefix)+comma])
	if workID == "" || !strings.Contains(prompt, "read the complete bound text input") {
		t.Fatalf("rendered provider prompt = %q, want consumed WorkID and complete authored prompt", prompt)
	}
	if !primaryResultContainsTTSArtifactMetadata(t, response.PrimaryResult) {
		t.Fatalf("primary result = %#v, want command-runner audio metadata", response.PrimaryResult)
	}
	if got, err := os.ReadFile(audioPath); err != nil || string(got) != packagedTTSFakeAudioFixture {
		t.Fatalf("command-runner audio artifact = %q, %v; want fixture", got, err)
	}
}

func factoryTTSFailedWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
) factoryapi.Work {
	t.Helper()
	var failed []factoryapi.Work
	for _, candidate := range listed.Results {
		if candidate.WorkTypeName == nil || *candidate.WorkTypeName != "task" ||
			candidate.State == nil || candidate.State.Name != "failed" {
			continue
		}
		failed = append(failed, candidate)
	}
	if len(failed) != 1 {
		t.Fatalf("failed TTS Work = %#v, want one task:failed item", failed)
	}
	if failed[0].WorkId == nil || strings.TrimSpace(*failed[0].WorkId) == "" {
		t.Fatalf("failed TTS Work id = %#v, want non-empty identity", failed[0].WorkId)
	}
	return failed[0]
}

func assertFactoryTTSFailedWork(
	t *testing.T,
	item factoryapi.Work,
	wantText, label string,
) {
	t.Helper()
	if item.State == nil || item.State.Name != "failed" || item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("%s state = %#v, want failed/FAILED", label, item.State)
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("%s content = %#v, want one preserved text part and no AUDIO part", label, item.Content)
	}
	textPart, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil || textPart.Text != wantText {
		t.Fatalf("%s content = %#v, want exact text %q", label, textPart, wantText)
	}
	if textPart.Slot != nil && *textPart.Slot != "text" {
		t.Fatalf("%s content slot = %q, want text when present", label, *textPart.Slot)
	}
}

func assertFactoryTTSFailureModelEvents(
	t *testing.T,
	events factoryTTSDispatchEvents,
	wantMessage string,
) {
	t.Helper()
	request, err := events.modelRequest.Payload.AsModelRequestEventPayload()
	if err != nil {
		t.Fatalf("decode failed MODEL_REQUEST %q: %v", events.modelRequest.Id, err)
	}
	if request.Operation != "TTS" || request.Worker != "tts-executor" ||
		request.Model != factorydefinitions.DefaultTTSModelName || request.ModelRequestId == "" {
		t.Fatalf("failed MODEL_REQUEST payload = %#v, want TTS/%s/%s and request identity", request, factorydefinitions.DefaultTTSModelName, "tts-executor")
	}
	response, err := events.modelResponse.Payload.AsModelResponseEventPayload()
	if err != nil {
		t.Fatalf("decode failed MODEL_RESPONSE %q: %v", events.modelResponse.Id, err)
	}
	if response.Operation != "TTS" || response.Outcome != factoryapi.InferenceOutcomeFailed ||
		response.ModelRequestId != request.ModelRequestId {
		t.Fatalf("failed MODEL_RESPONSE payload = %#v, want failed TTS response correlated to %q", response, request.ModelRequestId)
	}
	if response.OutputContent != nil || response.OutputPreview != nil {
		t.Fatalf("failed MODEL_RESPONSE output = content %#v preview %#v, want no successful output", response.OutputContent, response.OutputPreview)
	}
	if response.FailureDetail == nil || response.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown ||
		response.FailureDetail.Message != wantMessage {
		t.Fatalf("failed MODEL_RESPONSE failureDetail = %#v, want unknown/%q", response.FailureDetail, wantMessage)
	}
}

func assertFactoryTTSFailureDispatchResponse(
	t *testing.T,
	event *factoryapi.FactoryEvent,
	workID, wantText, wantMessage string,
) {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode failed DISPATCH_RESPONSE %q: %v", event.Id, err)
	}
	if payload.Outcome != factoryapi.WorkOutcomeFailed || payload.TransitionId != "tts-dispatch" {
		t.Fatalf("failed DISPATCH_RESPONSE payload = %#v, want failed tts-dispatch response", payload)
	}
	if payload.Error == nil || *payload.Error != wantMessage || payload.FailureDetail == nil ||
		payload.FailureDetail.Reason != factoryapi.WorkFailureTypeUnknown ||
		payload.FailureDetail.Message != wantMessage {
		t.Fatalf("failed DISPATCH_RESPONSE failure = error %#v detail %#v, want unknown/%q", payload.Error, payload.FailureDetail, wantMessage)
	}
	if payload.Output != nil {
		t.Fatalf("failed DISPATCH_RESPONSE output = %q, want no serialized AUDIO output", *payload.Output)
	}
	if payload.OutputResources != nil && len(*payload.OutputResources) != 0 {
		t.Fatalf("failed DISPATCH_RESPONSE outputResources = %#v, want no success artifact resources", payload.OutputResources)
	}
	if payload.OutputWork == nil || len(*payload.OutputWork) != 1 {
		t.Fatalf("failed DISPATCH_RESPONSE outputWork = %#v, want one onFailure Work", payload.OutputWork)
	}
	responseWork := (*payload.OutputWork)[0]
	if responseWork.WorkId == nil || *responseWork.WorkId != workID {
		t.Fatalf("failed DISPATCH_RESPONSE output Work = %#v, want work %q", responseWork, workID)
	}
	assertFactoryTTSFailedWork(t, responseWork, wantText, "DISPATCH_RESPONSE output Work")
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
	if audio.Slot == nil || *audio.Slot != "audio" {
		t.Fatalf("completed TTS Work AUDIO slot = %#v, want audio", audio.Slot)
	}
	if audio.ContentType == nil || *audio.ContentType != "audio/wav" {
		t.Fatalf("completed TTS Work AUDIO contentType = %#v, want audio/wav", audio.ContentType)
	}
	return audio
}

type factoryTTSDispatchEvents struct {
	workRequest      *factoryapi.FactoryEvent
	dispatchRequest  *factoryapi.FactoryEvent
	association      *factoryapi.FactoryEvent
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
	observed := collectFactoryTTSDispatchEvents(t, events, sessionID)
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

func collectFactoryTTSDispatchEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
) factoryTTSDispatchEvents {
	t.Helper()
	var observed factoryTTSDispatchEvents
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
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			factoryTTSRequireSessionID(t, event, sessionID)
			observed.association = event
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
	if observed.workRequest == nil || observed.dispatchRequest == nil || observed.association == nil || observed.modelRequest == nil ||
		observed.modelResponse == nil || observed.dispatchResponse == nil {
		t.Fatalf("TTS Factory Events missing required request/dispatch/model/response records: %#v", observed)
	}
	return observed
}

func assertFactoryTTSContextCorrelation(
	t *testing.T,
	events factoryTTSDispatchEvents,
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
	if err != nil || requestedText.Text != wantText {
		t.Fatalf("WORK_REQUEST text = %#v, want exact text %q", requestedText, wantText)
	}
	if requestedText.Slot != nil && *requestedText.Slot != "text" {
		t.Fatalf("WORK_REQUEST text slot = %q, want text when present", *requestedText.Slot)
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
	events factoryTTSDispatchEvents,
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
