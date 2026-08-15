//go:build functionallong

package runtime_api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	realOmniVoiceLongTestEnabledEnv  = "INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS"
	realOmniVoiceLongTestCommandEnv  = "INFINITE_YOU_OMNIVOICE_COMMAND"
	realOmniVoiceLongTestCacheDirEnv = "INFINITE_YOU_OMNIVOICE_CACHE_DIR"
	realOmniVoiceDefaultCommand      = "omnivoice-llamacpp"
	realOmniVoiceFactoryWaitTimeout  = 90 * time.Second
	realOmniVoiceFactoryWaitMacOS    = 5 * time.Minute
	realOmniVoiceFactoryWaitWindows  = 3 * time.Minute
)

func TestRealLocalInference_OMNIVOICEModelInvokeAndDirectAPIProduceAudio(t *testing.T) {
	support.SkipLongFunctional(t, "slow real OMNIVOICE local inference sweep")
	if os.Getenv(realOmniVoiceLongTestEnabledEnv) != "1" {
		t.Skip("set INFINITE_YOU_RUN_OMNIVOICE_LONG_TESTS=1 to run real OMNIVOICE local inference coverage")
	}

	command := resolveRealOmniVoiceCommand(t)
	cacheDir := stringsTrimSpaceOrDefault(os.Getenv(realOmniVoiceLongTestCacheDirEnv), filepath.Join(t.TempDir(), "managed-model-cache"))
	t.Logf("real local inference diagnostics: platform=%s/%s backend=%q cachePath=%q", runtime.GOOS, runtime.GOARCH, command, cacheDir)
	dir := support.ScaffoldFactory(t, realLocalInferenceFactoryConfig(command))
	writeRealLocalInferenceWorkstationConfig(t, dir)

	environment := append(
		os.Environ(),
		runcli.ModelCacheDirEnvironment+"="+cacheDir,
	)
	unexpectedProvider := support.NewRecordingCommandRunner("unexpected cloud-provider invocation")
	server := startFunctionalServer(
		t,
		dir,
		false,
		withEnvironment(environment),
		withWorkerCommands(unexpectedProvider, nil),
	)

	pull := postJSON[factoryapi.ModelPullResponse](t, server.URL()+"/models/OMNIVOICE_Q4_K_M/pull", nil, "asset pull failure")
	if pull.ModelName != "OMNIVOICE_Q4_K_M" || pull.CachePath == "" || pull.Revision == "" || len(pull.DownloadedFiles) == 0 {
		t.Fatalf("asset pull failure: response = %#v, want model identity, cache path, revision, and files", pull)
	}
	if pull.ManagedRuntimePull.Identity != "OMNIVOICE_Q4_K_M" ||
		pull.ManagedRuntimePull.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY ||
		pull.ManagedRuntimePull.PullOutcome == "" {
		t.Fatalf("asset pull failure: managed runtime pull = %#v, want ready managed pull metadata", pull.ManagedRuntimePull)
	}
	t.Logf("asset pull diagnostics: model=%s revision=%s cachePath=%s files=%d", pull.ModelName, pull.Revision, pull.CachePath, len(pull.DownloadedFiles))

	jsonInvocation := postJSON[factoryapi.ModelInvocationResponse](t, server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Bindings:  realLocalInferenceDirectBindings(),
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from direct local model invocation"),
		},
	}, "invocation failure")
	if jsonInvocation.ModelName != "OMNIVOICE_Q4_K_M" || jsonInvocation.Operation != "TTS" || jsonInvocation.ProviderLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("output validation failure: invocation identity = %#v, want OMNIVOICE local TTS metadata", jsonInvocation)
	}
	t.Logf("invocation diagnostics: model=%s operation=%s locality=%s outputParts=%d", jsonInvocation.ModelName, jsonInvocation.Operation, jsonInvocation.ProviderLocality, len(jsonInvocation.Content))
	if len(jsonInvocation.Content) != 1 {
		t.Fatalf("output validation failure: invocation content count = %d, want 1", len(jsonInvocation.Content))
	}
	audioPart, err := jsonInvocation.Content[0].AsWorkAudioContentPart()
	if err != nil {
		t.Fatalf("output validation failure: decode invocation audio content: %v", err)
	}
	audioPath := generatedAudioPath(audioPart)
	if stringPointerValue(audioPart.ContentType) != "audio/wav" || audioPath == "" {
		t.Fatalf("output validation failure: invocation audio part = %#v, want audio/wav file output", audioPart)
	}
	assertWAVFile(t, audioPath, "output validation failure")

	streamBytes, streamType := postAudioInvocation(t, server.URL()+"/models/OMNIVOICE_Q4_K_M/invocations", factoryapi.ModelInvocationRequest{
		Operation: "TTS",
		Bindings:  realLocalInferenceDirectBindings(),
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from streamed local model invocation"),
		},
		Options: &factoryapi.ModelInvocationOptions{
			ResponseMode: func() *factoryapi.ModelInvocationResponseMode {
				mode := factoryapi.AUDIOSTREAM
				return &mode
			}(),
		},
	})
	if streamType != "audio/wav" {
		t.Fatalf("output validation failure: stream content-type = %q, want audio/wav", streamType)
	}
	assertWAVBytes(t, streamBytes, "output validation failure")

	traceID := submitGeneratedWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         stringPtr("real-local-model-inference"),
		WorkTypeName: "speech",
		Content: &factoryapi.WorkContent{
			mustGeneratedFunctionalTextPart(t, "hello from factory-level model invoke"),
		},
	})
	work := waitForGeneratedWorkAtPlace(t, server.URL(), traceID, "speech:complete", realLocalInferenceFactoryWaitTimeout())
	findGeneratedWorkByTraceIDAndPlace(t, work.Results, traceID, "speech:complete")
	eventAudioPath := assertRecordedRealLocalModelEvents(t, server.GetFactoryEvents(t))
	assertWAVFile(t, eventAudioPath, "output validation failure")
}

func realLocalInferenceFactoryWaitTimeout() time.Duration {
	if runtime.GOOS == "windows" {
		return realOmniVoiceFactoryWaitWindows
	}
	if runtime.GOOS == "darwin" {
		return realOmniVoiceFactoryWaitMacOS
	}
	return realOmniVoiceFactoryWaitTimeout
}

func resolveRealOmniVoiceCommand(t *testing.T) string {
	t.Helper()
	command := stringsTrimSpaceOrDefault(os.Getenv(realOmniVoiceLongTestCommandEnv), realOmniVoiceDefaultCommand)
	resolved, err := exec.LookPath(command)
	if err != nil {
		t.Fatalf("platform or backend failure: platform=%s/%s command=%q path=%q: %v", runtime.GOOS, runtime.GOARCH, command, os.Getenv("PATH"), err)
	}
	return resolved
}

func realLocalInferenceFactoryConfig(command string) map[string]any {
	return map[string]any{
		"name": "real-local-model-inference",
		"workTypes": []map[string]any{{
			"name": "speech",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"resources": []map[string]any{{
			"name":       "omnivoice-cache",
			"type":       "MODEL",
			"capacity":   1,
			"model":      "OMNIVOICE_Q4_K_M",
			"backend":    "LLAMACPP",
			"loadPolicy": "ON_DEMAND",
		}},
		"workers": []map[string]any{{
			"name":          "tts-worker",
			"type":          "MODEL_WORKER",
			"model":         "OMNIVOICE_Q4_K_M",
			"modelProvider": "CODEX",
			"modelLocality": "LOCAL",
			"command":       command,
			"resources": []map[string]any{{
				"name":     "omnivoice-cache",
				"capacity": 1,
			}},
			"operations": []map[string]any{{
				"name": "TTS",
				"inputs": []map[string]any{{
					"name":         "text",
					"required":     true,
					"contentTypes": []string{"TEXT"},
				}},
				"outputs": []map[string]any{{
					"name":         "audio",
					"contentTypes": []string{"AUDIO"},
				}},
			}},
		}},
		"workstations": []map[string]any{{
			"name":      "speak",
			"type":      "MODEL_INVOKE",
			"operation": "TTS",
			"worker":    "tts-worker",
			"operationBindings": []map[string]any{{
				"slot": "text",
				"selector": map[string]any{
					"type": "TEXT",
				},
			}},
			"inputs":    []map[string]string{{"workType": "speech", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "speech", "state": "complete"}},
			"onFailure": []map[string]string{{"workType": "speech", "state": "failed"}},
		}},
	}
}

func writeRealLocalInferenceWorkstationConfig(t *testing.T, dir string) {
	t.Helper()
	support.WriteWorkstationConfig(t, dir, "speak", `---
type: MODEL_INVOKE
---
Generate speech output.
`)
}

func realLocalInferenceDirectBindings() *[]factoryapi.WorkstationOperationBinding {
	return &[]factoryapi.WorkstationOperationBinding{{
		Slot: "text",
		Selector: &factoryapi.WorkstationOperationBindingSelector{
			Type: func() *factoryapi.ModelOperationContentType {
				value := factoryapi.ModelOperationContentTypeText
				return &value
			}(),
		},
	}}
}

func postAudioInvocation(t *testing.T, endpoint string, request factoryapi.ModelInvocationRequest) ([]byte, string) {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal audio invocation request: %v", err)
	}
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("invocation failure: POST %s: %v", endpoint, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("invocation failure: POST %s status = %d, want 200: %s", endpoint, resp.StatusCode, string(payload))
	}
	audio, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("output validation failure: read streamed audio body: %v", err)
	}
	return audio, resp.Header.Get("Content-Type")
}

func assertWAVFile(t *testing.T, path string, failurePrefix string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: read audio file %q: %v", failurePrefix, path, err)
	}
	assertWAVBytes(t, data, failurePrefix)
}

func assertWAVBytes(t *testing.T, data []byte, failurePrefix string) {
	t.Helper()
	if len(data) <= 44 {
		t.Fatalf("%s: audio byte length = %d, want header plus non-empty sample data", failurePrefix, len(data))
	}
	if !bytes.Equal(data[:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		t.Fatalf("%s: audio header = %q/%q, want RIFF/WAVE", failurePrefix, string(data[:4]), string(data[8:12]))
	}
}

func findGeneratedWorkByTraceIDAndPlace(t *testing.T, works []factoryapi.Work, traceID string, placeID string) factoryapi.Work {
	t.Helper()
	for _, item := range works {
		if stringPointerValue(item.TraceId) == traceID && generatedWorkPlaceID(item) == placeID {
			return item
		}
	}
	t.Fatalf("output validation failure: no work item found for trace %q in %s", traceID, placeID)
	return factoryapi.Work{}
}

type realLocalModelEventCheck struct {
	sawTTSRequest bool
	audioPath     string
	responses     []realLocalModelResponseObservation
	failures      []string
}

type realLocalModelResponseObservation struct {
	eventIndex      int
	eventType       factoryapi.FactoryEventType
	payloadDecoded  bool
	payloadError    string
	operation       string
	outcome         factoryapi.InferenceOutcome
	outputPresent   bool
	outputPartCount int
	parts           []realLocalModelContentObservation
}

type realLocalModelContentObservation struct {
	partIndex   int
	decodedType string
	contentType string
	file        string
	url         string
	decodeError string
}

func assertRecordedRealLocalModelEvents(t *testing.T, events []factoryapi.FactoryEvent) string {
	t.Helper()
	check := inspectRecordedRealLocalModelEvents(events)
	if !check.sawTTSRequest || strings.TrimSpace(check.audioPath) == "" {
		t.Fatalf("output validation failure: %s", formatRealLocalModelEventCheck(check))
	}
	return check.audioPath
}

func inspectRecordedRealLocalModelEvents(events []factoryapi.FactoryEvent) realLocalModelEventCheck {
	var check realLocalModelEventCheck
	for eventIndex, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeModelRequest:
			payload, err := event.Payload.AsModelRequestEventPayload()
			if err == nil && payload.Operation == "TTS" {
				check.sawTTSRequest = true
			}
		case factoryapi.FactoryEventTypeModelResponse:
			observation := realLocalModelResponseObservation{
				eventIndex: eventIndex,
				eventType:  event.Type,
			}
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err != nil {
				observation.payloadError = err.Error()
				check.responses = append(check.responses, observation)
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE payload decode failure: %v",
					eventIndex,
					err,
				))
				continue
			}

			observation.payloadDecoded = true
			observation.operation = payload.Operation
			observation.outcome = payload.Outcome
			if payload.OutputContent != nil {
				observation.outputPresent = true
				observation.outputPartCount = len(*payload.OutputContent)
				for partIndex, part := range *payload.OutputContent {
					observation.parts = append(observation.parts, inspectRealLocalModelContentPart(partIndex, part))
				}
			}
			check.responses = append(check.responses, observation)

			if payload.Operation != "TTS" {
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE wrong operation: got %q, want %q",
					eventIndex,
					payload.Operation,
					"TTS",
				))
				continue
			}
			if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE non-succeeded outcome: got %q, want %q",
					eventIndex,
					payload.Outcome,
					factoryapi.InferenceOutcomeSucceeded,
				))
				continue
			}
			if payload.OutputContent == nil || len(*payload.OutputContent) != 1 {
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE output-part count: got %s, want exactly 1",
					eventIndex,
					formatRealLocalModelOutputPartCount(observation),
				))
				continue
			}

			audio, audioErr := (*payload.OutputContent)[0].AsWorkAudioContentPart()
			if audioErr != nil {
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE audio-part decode failure: %v",
					eventIndex,
					audioErr,
				))
				continue
			}
			if stringPointerValue(audio.ContentType) != "audio/wav" {
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE audio content-type mismatch: got %q, want %q",
					eventIndex,
					stringPointerValue(audio.ContentType),
					"audio/wav",
				))
				continue
			}
			responseAudioPath := generatedAudioPath(audio)
			if strings.TrimSpace(responseAudioPath) == "" {
				check.failures = append(check.failures, fmt.Sprintf(
					"response[%d] MODEL_RESPONSE empty file or URL: file=%s url=%s",
					eventIndex,
					formatRealLocalModelValue(stringPointerValue(audio.File)),
					formatRealLocalModelValue(string(audio.Url)),
				))
				continue
			}
			check.audioPath = responseAudioPath
		}
	}

	if !check.sawTTSRequest {
		check.failures = append(check.failures, "no TTS MODEL_REQUEST event observed")
	}
	if len(check.responses) == 0 {
		check.failures = append(check.failures, "no MODEL_RESPONSE event observed")
	}
	return check
}

func inspectRealLocalModelContentPart(partIndex int, part factoryapi.WorkContentPart) realLocalModelContentObservation {
	observation := realLocalModelContentObservation{partIndex: partIndex}
	raw, err := json.Marshal(part)
	if err != nil {
		observation.decodeError = err.Error()
		return observation
	}
	if strings.TrimSpace(string(raw)) == "" || strings.TrimSpace(string(raw)) == "null" {
		observation.decodeError = "content part payload is empty"
		return observation
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		observation.decodeError = err.Error()
		return observation
	}
	observation.decodedType = realLocalModelRawStringField(fields, "type")
	observation.contentType = realLocalModelRawStringField(fields, "contentType")
	observation.file = realLocalModelRawStringField(fields, "file")
	observation.url = realLocalModelRawStringField(fields, "url")
	return observation
}

func realLocalModelRawStringField(fields map[string]json.RawMessage, name string) string {
	raw, ok := fields[name]
	if !ok || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "<decode-failed>"
	}
	return value
}

func formatRealLocalModelEventCheck(check realLocalModelEventCheck) string {
	failures := append([]string(nil), check.failures...)
	if len(failures) == 0 {
		failures = append(failures, "no successful TTS audio response matched the required contract")
	}
	responseSummary := "none"
	if len(check.responses) > 0 {
		formatted := make([]string, 0, len(check.responses))
		for _, response := range check.responses {
			formatted = append(formatted, formatRealLocalModelResponseObservation(response))
		}
		responseSummary = strings.Join(formatted, "; ")
	}
	return fmt.Sprintf(
		"TTS request=%t audioPath=%s; violated conditions=%s; observed MODEL_RESPONSE events: %s",
		check.sawTTSRequest,
		formatRealLocalModelValue(check.audioPath),
		strings.Join(failures, "; "),
		responseSummary,
	)
}

func formatRealLocalModelResponseObservation(observation realLocalModelResponseObservation) string {
	if !observation.payloadDecoded {
		return fmt.Sprintf(
			"eventType=%s payload=decode-failure operation=<undecoded> outcome=<undecoded> outputParts=<undecoded> error=%q",
			observation.eventType,
			observation.payloadError,
		)
	}

	parts := "none"
	if len(observation.parts) > 0 {
		formatted := make([]string, 0, len(observation.parts))
		for _, part := range observation.parts {
			partText := fmt.Sprintf(
				"part[%d]={type=%s contentType=%s file=%s url=%s",
				part.partIndex,
				formatRealLocalModelValue(part.decodedType),
				formatRealLocalModelValue(part.contentType),
				formatRealLocalModelValue(part.file),
				formatRealLocalModelValue(part.url),
			)
			if part.decodeError != "" {
				partText += fmt.Sprintf(" decodeError=%q", part.decodeError)
			}
			formatted = append(formatted, partText+"}")
		}
		parts = strings.Join(formatted, ", ")
	}
	return fmt.Sprintf(
		"eventType=%s operation=%s outcome=%s outputParts=%s parts=[%s]",
		observation.eventType,
		formatRealLocalModelValue(observation.operation),
		formatRealLocalModelValue(string(observation.outcome)),
		formatRealLocalModelOutputPartCount(observation),
		parts,
	)
}

func formatRealLocalModelOutputPartCount(observation realLocalModelResponseObservation) string {
	if !observation.outputPresent {
		return "absent(0)"
	}
	return fmt.Sprintf("%d", observation.outputPartCount)
}

func formatRealLocalModelValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "<empty>"
	}
	return fmt.Sprintf("%q", value)
}

func TestRecordedRealLocalModelEventDiagnostics(t *testing.T) {
	validFile := realLocalModelAudioPart(t, "C:/tmp/factory-work.wav", "", "audio/wav")
	validURL := realLocalModelAudioPart(t, "", "file:///tmp/factory-work.wav", "audio/wav")
	wrongContentType := realLocalModelAudioPart(t, "C:/tmp/factory-work.mp3", "", "audio/mpeg")
	malformedAudio := realLocalModelMalformedAudioPart(t)

	outputWithValidFile := factoryapi.WorkContent{validFile}
	outputWithValidURL := factoryapi.WorkContent{validURL}
	outputWithTwoParts := factoryapi.WorkContent{validFile, validFile}
	outputWithMalformedAudio := factoryapi.WorkContent{malformedAudio}
	outputWithWrongContentType := factoryapi.WorkContent{wrongContentType}
	outputWithEmptyLocation := factoryapi.WorkContent{realLocalModelAudioPart(t, "", "", "audio/wav")}

	cases := []struct {
		name     string
		response *factoryapi.ModelResponseEventPayload
		want     string
		wantPath string
	}{
		{
			name: "no model response",
			want: "no MODEL_RESPONSE event observed",
		},
		{
			name:     "payload decode failure",
			response: nil,
			want:     "MODEL_RESPONSE payload decode failure",
		},
		{
			name: "wrong operation",
			response: &factoryapi.ModelResponseEventPayload{
				Operation:     "ASR",
				Outcome:       factoryapi.InferenceOutcomeSucceeded,
				OutputContent: &outputWithValidFile,
			},
			want: "wrong operation",
		},
		{
			name: "wrong outcome",
			response: &factoryapi.ModelResponseEventPayload{
				Operation:     "TTS",
				Outcome:       factoryapi.InferenceOutcomeFailed,
				OutputContent: &outputWithValidFile,
			},
			want: "non-succeeded outcome",
		},
		{
			name: "absent output parts",
			response: &factoryapi.ModelResponseEventPayload{
				Operation: "TTS",
				Outcome:   factoryapi.InferenceOutcomeSucceeded,
			},
			want: "output-part count: got absent(0)",
		},
		{
			name: "wrong output part count",
			response: &factoryapi.ModelResponseEventPayload{
				Operation:     "TTS",
				Outcome:       factoryapi.InferenceOutcomeSucceeded,
				OutputContent: &outputWithTwoParts,
			},
			want: "output-part count: got 2",
		},
		{
			name: "audio part decode failure",
			response: &factoryapi.ModelResponseEventPayload{
				Operation:     "TTS",
				Outcome:       factoryapi.InferenceOutcomeSucceeded,
				OutputContent: &outputWithMalformedAudio,
			},
			want: "audio-part decode failure",
		},
		{
			name: "wrong audio content type",
			response: &factoryapi.ModelResponseEventPayload{
				Operation:     "TTS",
				Outcome:       factoryapi.InferenceOutcomeSucceeded,
				OutputContent: &outputWithWrongContentType,
			},
			want: "audio content-type mismatch",
		},
		{
			name: "empty file and URL",
			response: &factoryapi.ModelResponseEventPayload{
				Operation:     "TTS",
				Outcome:       factoryapi.InferenceOutcomeSucceeded,
				OutputContent: &outputWithEmptyLocation,
			},
			want: "empty file or URL",
		},
		{
			name:     "valid file",
			response: &factoryapi.ModelResponseEventPayload{Operation: "TTS", Outcome: factoryapi.InferenceOutcomeSucceeded, OutputContent: &outputWithValidFile},
			want:     `eventType=MODEL_RESPONSE operation="TTS" outcome="SUCCEEDED" outputParts=1 parts=[part[0]={type="AUDIO" contentType="audio/wav" file="C:/tmp/factory-work.wav" url=<empty>}`,
			wantPath: "C:/tmp/factory-work.wav",
		},
		{
			name:     "valid URL",
			response: &factoryapi.ModelResponseEventPayload{Operation: "TTS", Outcome: factoryapi.InferenceOutcomeSucceeded, OutputContent: &outputWithValidURL},
			want:     `eventType=MODEL_RESPONSE operation="TTS" outcome="SUCCEEDED" outputParts=1 parts=[part[0]={type="AUDIO" contentType="audio/wav" file=<empty> url="file:///tmp/factory-work.wav"}`,
			wantPath: "/tmp/factory-work.wav",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			events := []factoryapi.FactoryEvent{realLocalModelRequestEvent(t)}
			if testCase.name != "no model response" {
				events = append(events, realLocalModelResponseEvent(t, testCase.response))
			}

			check := inspectRecordedRealLocalModelEvents(events)
			formatted := formatRealLocalModelEventCheck(check)
			if testCase.want != "" && !strings.Contains(formatted, testCase.want) {
				t.Fatalf("diagnostics = %s, want substring %q", formatted, testCase.want)
			}
			if testCase.wantPath != "" {
				if check.audioPath != testCase.wantPath {
					t.Fatalf("audio path = %q, want %q; diagnostics: %s", check.audioPath, testCase.wantPath, formatted)
				}
				return
			}
			if !strings.Contains(formatted, testCase.want) {
				t.Fatalf("diagnostics = %s, want substring %q", formatted, testCase.want)
			}
		})
	}
}

func realLocalModelRequestEvent(t *testing.T) factoryapi.FactoryEvent {
	t.Helper()
	event := factoryapi.FactoryEvent{Type: factoryapi.FactoryEventTypeModelRequest}
	if err := event.Payload.FromModelRequestEventPayload(factoryapi.ModelRequestEventPayload{Operation: "TTS"}); err != nil {
		t.Fatalf("encode model request event: %v", err)
	}
	return event
}

func realLocalModelResponseEvent(t *testing.T, payload *factoryapi.ModelResponseEventPayload) factoryapi.FactoryEvent {
	t.Helper()
	event := factoryapi.FactoryEvent{Type: factoryapi.FactoryEventTypeModelResponse}
	if payload != nil {
		if err := event.Payload.FromModelResponseEventPayload(*payload); err != nil {
			t.Fatalf("encode model response event: %v", err)
		}
	}
	return event
}

func realLocalModelAudioPart(t *testing.T, file string, url string, contentType string) factoryapi.WorkContentPart {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	audio := factoryapi.WorkAudioContentPart{
		ContentType: &contentType,
		Type:        factoryapi.WorkContentPartTypeAudio,
		Url:         url,
	}
	if file != "" {
		fileValue := factoryapi.WorkContentDeprecatedFileProperty(file)
		audio.File = &fileValue
	}
	if err := part.FromWorkAudioContentPart(audio); err != nil {
		t.Fatalf("encode audio content part: %v", err)
	}
	return part
}

func realLocalModelMalformedAudioPart(t *testing.T) factoryapi.WorkContentPart {
	t.Helper()
	var part factoryapi.WorkContentPart
	if err := json.Unmarshal([]byte(`{"type":"AUDIO","contentType":42,"file":"C:/tmp/factory-work.wav","url":""}`), &part); err != nil {
		t.Fatalf("decode malformed audio content part fixture: %v", err)
	}
	return part
}

func stringsTrimSpaceOrDefault(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
