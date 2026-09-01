package agy

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agyColdWatchFactoryName = "@you/agy-cold-watch"
	agyClipQAFactoryName    = "@you/agy-clip-qa"
	agyClipQAShotSpec       = "A silver-haired woman points at a bright star; a clock-headed figure tilts toward it; red energy flares at the end; no speech is allowed."
)

type agyClipQAVerdict struct {
	ActionCompleted   bool     `json:"action_completed"`
	SpecDeviations    []string `json:"spec_deviations"`
	TemporalArtifacts []string `json:"temporal_artifacts"`
	AudioContent      string   `json:"audio_content"`
	UnexpectedSpeech  bool     `json:"unexpected_speech"`
	Verdict           string   `json:"verdict"`
	Confidence        float64  `json:"confidence"`
}

// TestAgyProductionReviewRolesThroughRootBuildProcess exercises the first
// production review role through the public named-Factory CLI and the exact
// root.BuildProcess + Process.Execute boundary. The provider edge replays the
// real AGY media traces, so the test never starts a live AGY process.
func TestAgyProductionReviewRolesThroughRootBuildProcess(t *testing.T) {
	fixture := agySharedProcess(t)
	fixture.startRoleHost(t)
	t.Cleanup(func() { fixture.stopRoleHost(t) })

	tests := []struct {
		name string
		run  func(*testing.T)
	}{
		{name: "cold-watch-complete-report-contract", run: testAgyColdWatchCompleteReportContract},
		{name: "cold-watch-incomplete-real-traces-fail", run: testAgyColdWatchIncompleteRealTracesFail},
		{name: "missing-file-fails-work-after-provider-success", run: testAgyColdWatchMissingFile},
		{name: "clip-qa-structured-pass-with-audio-evidence", run: testAgyClipQAStructuredPass},
		{name: "clip-qa-structured-reroll-is-accepted", run: testAgyClipQAStructuredReroll},
		{name: "clip-qa-semantic-invalid-results-fail", run: testAgyClipQASemanticInvalidResults},
		{name: "clip-qa-missing-file-fails-work", run: testAgyClipQAMissingFile},
		{name: "clip-qa-schema-invalid-result-fails-work", run: testAgyClipQASchemaInvalid},
		{name: "clip-qa-provider-failure-fails-work", run: testAgyClipQAProviderFailure},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			test.run(t)
		})
	}
}

func testAgyColdWatchCompleteReportContract(t *testing.T) {
	response, events, route, assetPath, callStart := runAgySharedColdWatchComplete(t)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response=%#v", response.Status, response)
	}
	if response.PrimaryResult == nil {
		t.Fatal("primaryResult is nil, want complete cold-watch report")
	}
	result := invocationPrimaryText(t, *response.PrimaryResult)
	for _, want := range []string{
		"## Inspection status",
		"## Chronological events",
		"## Temporal or transient defects",
		"## Audio content and defects",
		"## Observed speech",
		"## Overall recommendation",
		"Recommendation: pass",
	} {
		if !strings.Contains(result, want) {
			t.Fatalf("cold-watch primary result missing %q: %s", want, result)
		}
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("AGY provider command calls = %d, want exactly one", got)
	}
	assertAgyColdWatchCommand(t, route.lastRequest(), assetPath)
	assertAgyColdWatchEvents(t, events, factoryapi.WorkOutcomeAccepted, result)
}

func testAgyColdWatchIncompleteRealTracesFail(t *testing.T) {
	for _, test := range []struct {
		name         string
		trace        string
		asset        string
		wantEvidence []string
	}{
		{
			name:  "video-watch",
			trace: "agy-trace-video-watch.stream.jsonl",
			asset: "clip-fixture.mp4",
			wantEvidence: []string{
				"silver-haired woman",
				"ambient atmospheric drone",
				"clock ticking",
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := agySharedProcess(t)
			selector := "role-cold-watch-incomplete-" + test.name
			route := fixture.routes[selector]
			response, events, route, assetPath, callStart := fixture.runRoleFailure(t, selector, []string{
				"you", "--json", "run",
				"--named", agyColdWatchFactoryName,
				"--cut-path", route.assetPath,
			})
			if response.Status != factoryapi.InvocationTerminalStatusFailed {
				t.Fatalf("invocation status = %q, want FAILED for incomplete report; response=%#v", response.Status, response)
			}
			if response.PrimaryResult != nil {
				t.Fatalf("primaryResult = %#v, want no recommendation for incomplete report", response.PrimaryResult)
			}
			if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
				t.Fatalf("failure message = %#v, want actionable diagnostic", response.Message)
			}
			providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
			if providerResponse.Response == nil {
				t.Fatal("real AGY trace did not retain provider response evidence")
			}
			for _, want := range test.wantEvidence {
				if !strings.Contains(*providerResponse.Response, want) {
					t.Fatalf("provider response missing real-trace evidence %q: %s", want, *providerResponse.Response)
				}
			}
			if got := route.callCount() - callStart; got != 1 {
				t.Fatalf("AGY provider command calls = %d, want exactly one", got)
			}
			assertAgyColdWatchCommand(t, route.lastRequest(), assetPath)
			assertAgyColdWatchEventsFailure(t, events, "output contract failed")
		})
	}
}

func testAgyColdWatchMissingFile(t *testing.T) {
	fixture := agySharedProcess(t)
	selector := "role-cold-watch-missing-file"
	route := fixture.routes[selector]
	response, events, route, assetPath, callStart := fixture.runRoleFailure(t, selector, []string{
		"you", "--json", "run",
		"--named", agyColdWatchFactoryName,
		"--cut-path", route.assetPath,
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want no recommendation for missing media", response.PrimaryResult)
	}
	if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
		t.Fatalf("failure message = %#v, want actionable non-empty diagnostic", response.Message)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("AGY provider command calls = %d, want exactly one", got)
	}
	assertAgyColdWatchCommand(t, route.lastRequest(), assetPath)
	providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
	if providerResponse.Response == nil ||
		!strings.Contains(*providerResponse.Response, "does-not-exist-xyz.mp4") {
		t.Fatalf("provider refusal response = %#v, want missing-file explanation", providerResponse.Response)
	}
	assertAgyColdWatchEventsFailure(t, events, "does-not-exist-xyz.mp4")
}

func testAgyClipQAStructuredPass(t *testing.T) {
	response, events, route, assetPath, callStart := runAgySharedClipQAPass(t)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response=%#v", response.Status, response)
	}
	if response.PrimaryResult == nil {
		t.Fatal("primaryResult is nil, want clip-QA verdict")
	}
	result := invocationPrimaryText(t, *response.PrimaryResult)
	verdict := decodeAgyClipQAVerdict(t, result)
	if !verdict.ActionCompleted || verdict.Verdict != "pass" || verdict.AudioContent != "noise" || verdict.UnexpectedSpeech {
		t.Fatalf("clip-QA verdict = %#v, want completed/noise/no-speech/pass", verdict)
	}
	if verdict.SpecDeviations == nil || len(verdict.SpecDeviations) != 0 || verdict.TemporalArtifacts == nil || len(verdict.TemporalArtifacts) != 0 {
		t.Fatalf("clip-QA reason arrays = %#v/%#v, want present and empty", verdict.SpecDeviations, verdict.TemporalArtifacts)
	}
	if verdict.Confidence < 0 || verdict.Confidence > 1 {
		t.Fatalf("clip-QA confidence = %v, want [0,1]", verdict.Confidence)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("AGY provider command calls = %d, want exactly one", got)
	}
	assertAgyClipQACommand(t, route.lastRequest(), assetPath, agyClipQAShotSpec)
	assertAgyClipQAEvents(t, events, result)
}

func testAgyClipQAStructuredReroll(t *testing.T) {
	response, events, route, assetPath, callStart := runAgySharedClipQAReroll(t)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED for inspected reroll; response=%#v", response.Status, response)
	}
	if response.PrimaryResult == nil {
		t.Fatal("primaryResult is nil, want accepted reroll verdict")
	}
	result := decodeAgyClipQAVerdict(t, invocationPrimaryText(t, *response.PrimaryResult))
	if result.Verdict != "reroll" || result.ActionCompleted || len(result.SpecDeviations) == 0 {
		t.Fatalf("clip-QA reroll = %#v, want accepted inspected failure", result)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("AGY provider command calls = %d, want exactly one", got)
	}
	assertAgyClipQACommand(t, route.lastRequest(), assetPath, agyClipQAShotSpec)
	assertAgyClipQAEvents(t, events, invocationPrimaryText(t, *response.PrimaryResult))
}

func testAgyClipQASemanticInvalidResults(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{
			name: "pass with specification deviation",
			mutate: func(verdict map[string]any) {
				verdict["spec_deviations"] = []string{"wrong action"}
			},
		},
	} {
		test := test
		t.Run(test.name, func(t *testing.T) {
			fixture := agySharedProcess(t)
			verdict := validAgyClipQAVerdictPayload()
			test.mutate(verdict)
			selector := "role-clipqa-invalid-" + agyRouteSlug(test.name)
			route := fixture.routes[selector]
			response, events, route, assetPath, callStart := fixture.runRoleFailure(t, selector, []string{
				"you", "--json", "run",
				"--named", agyClipQAFactoryName,
				"--clip-path", route.assetPath,
				"--shot-specification", agyClipQAShotSpec,
			})
			assertAgyClipQAFailedSharedInvocation(t, response, events, route, assetPath, callStart)
		})
	}
}

func testAgyClipQAMissingFile(t *testing.T) {
	fixture := agySharedProcess(t)
	selector := "role-clipqa-missing-file"
	route := fixture.routes[selector]
	response, events, route, assetPath, callStart := fixture.runRoleFailure(t, selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
	assertAgyClipQAFailedSharedInvocation(t, response, events, route, assetPath, callStart)
}

func testAgyClipQASchemaInvalid(t *testing.T) {
	fixture := agySharedProcess(t)
	selector := "role-clipqa-schema-invalid"
	route := fixture.routes[selector]
	response, events, route, assetPath, callStart := fixture.runRoleFailure(t, selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
	assertAgyClipQAFailedSharedInvocation(t, response, events, route, assetPath, callStart)
}

func testAgyClipQAProviderFailure(t *testing.T) {
	fixture := agySharedProcess(t)
	selector := "role-clipqa-provider-failure"
	route := fixture.routes[selector]
	response, events, route, assetPath, callStart := fixture.runRoleFailure(t, selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
	assertAgyClipQAFailedSharedInvocation(t, response, events, route, assetPath, callStart)
}

func assertAgyClipQAFailedSharedInvocation(
	t *testing.T,
	response factoryapi.InvocationResponse,
	events []factoryapi.FactoryEvent,
	route *agySharedCommandRoute,
	assetPath string,
	callStart int,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want no production verdict for failure", response.PrimaryResult)
	}
	if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
		t.Fatalf("failure message = %#v, want actionable non-empty diagnostic", response.Message)
	}
	if got := route.callCount() - callStart; got == 0 {
		t.Fatal("AGY provider command calls = 0, want at least one failure attempt")
	}
	assertAgyClipQACommand(t, route.lastRequest(), assetPath, agyClipQAShotSpec)
	assertAgyGoldenDispatchFailure(t, events)
}

func alignAgyClipQAReplaySchema(t *testing.T, stdout []byte) []byte {
	t.Helper()
	if len(stdout) == 0 {
		return stdout
	}

	// The recorded trace predates the bounded confidence contract. Preserve its
	// real media-review response while aligning only the echoed schema metadata
	// with the current authored request.
	lines := strings.Split(strings.TrimSuffix(string(stdout), "\n"), "\n")
	var output strings.Builder
	changed := false
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return stdout
		}
		if alignAgyClipQAReplaySchemaParent(event["init"]) || alignAgyClipQAReplaySchemaParent(event["result"]) {
			changed = true
		}
		encoded, err := json.Marshal(event)
		if err != nil {
			t.Fatalf("marshal AGY clip-QA replay event: %v", err)
		}
		output.Write(encoded)
		output.WriteByte('\n')
	}
	if !changed {
		return stdout
	}
	return []byte(output.String())
}

func alignAgyClipQAReplaySchemaParent(value any) bool {
	parent, ok := value.(map[string]any)
	if !ok {
		return false
	}
	schema, ok := parent["json_schema"].(map[string]any)
	if !ok {
		return false
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	if _, ok := properties["action_completed"]; !ok {
		return false
	}
	confidence, ok := properties["confidence"].(map[string]any)
	if !ok {
		return false
	}
	confidence["minimum"] = float64(0)
	confidence["maximum"] = float64(1)
	return true
}

func readAgyColdWatchRecording(t *testing.T, path string) []factoryapi.FactoryEvent {
	return readAgyRecording(t, path, "AGY cold-watch")
}

func readAgyRecording(t *testing.T, path, label string) []factoryapi.FactoryEvent {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s recording %q: %v", label, path, err)
	}
	var artifact struct {
		Events []factoryapi.FactoryEvent `json:"events"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode %s recording: %v\n%s", label, err, data)
	}
	if len(artifact.Events) == 0 {
		t.Fatalf("%s recording has no Factory Events", label)
	}
	return artifact.Events
}

func decodeAgyClipQAVerdict(t *testing.T, content string) agyClipQAVerdict {
	t.Helper()
	var verdict agyClipQAVerdict
	if err := json.Unmarshal([]byte(content), &verdict); err != nil {
		t.Fatalf("decode clip-QA primary result: %v\n%s", err, content)
	}
	return verdict
}

func agyColdWatchCompleteReportTrace(t *testing.T) []byte {
	t.Helper()
	response := `## Inspection status
Inspected: yes

## Chronological events
- 00:00.000 — The subject enters frame.
- 00:02.000 — The subject turns toward the light.

## Temporal or transient defects
None observed.

## Audio content and defects
Audio content: noise
None observed.

## Observed speech
None observed.

## Overall recommendation
Recommendation: pass`
	line, err := json.Marshal(map[string]any{
		"event":           "result",
		"conversation_id": "agy-cold-watch-contract",
		"result": map[string]any{
			"status":           "SUCCESS",
			"response":         response,
			"duration_seconds": 1.25,
			"num_turns":        1,
			"usage":            map[string]int64{"input_tokens": 1, "output_tokens": 1, "thinking_tokens": 0, "cache_read_tokens": 0, "total_tokens": 2},
		},
	})
	if err != nil {
		t.Fatalf("marshal complete cold-watch trace: %v", err)
	}
	return append(line, '\n')
}

func validAgyClipQAVerdictPayload() map[string]any {
	return map[string]any{
		"action_completed":   true,
		"spec_deviations":    []string{},
		"temporal_artifacts": []string{},
		"audio_content":      "noise",
		"unexpected_speech":  false,
		"verdict":            "pass",
		"confidence":         0.95,
	}
}

func agyClipQAVerdictTrace(t *testing.T, verdict map[string]any) []byte {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(string(readAgyGoldenAsset(t, "agy-trace-clipqa-schema.stream.jsonl"))), "\n")
	var output strings.Builder
	foundResult := false
	for _, line := range lines {
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("decode AGY clip-QA trace line: %v", err)
		}
		if event["event"] == "result" {
			result, ok := event["result"].(map[string]any)
			if !ok {
				t.Fatalf("AGY clip-QA result = %#v, want object", event["result"])
			}
			encodedVerdict, err := json.Marshal(verdict)
			if err != nil {
				t.Fatalf("marshal AGY clip-QA verdict: %v", err)
			}
			result["response"] = string(encodedVerdict)
			result["structured_output"] = verdict
			encodedEvent, err := json.Marshal(event)
			if err != nil {
				t.Fatalf("marshal AGY clip-QA result event: %v", err)
			}
			line = string(encodedEvent)
			foundResult = true
		}
		output.WriteString(line)
		output.WriteByte('\n')
	}
	if !foundResult {
		t.Fatal("AGY clip-QA trace has no result event")
	}
	return []byte(output.String())
}

func assertAgyClipQACommand(
	t *testing.T,
	request platformprocess.CommandRequest,
	clipPath string,
	shotSpecification string,
) {
	t.Helper()

	if request.Command != "agy" {
		t.Fatalf("provider command = %q, want agy", request.Command)
	}
	if request.WorkDir == "" || !containsArgPair(request.Args, "--add-dir", request.WorkDir) {
		t.Fatalf("provider argv = %#v, want --add-dir matching workdir %q", request.Args, request.WorkDir)
	}
	if !containsArgPair(request.Args, "--output-format", "json") ||
		!containsArgPair(request.Args, "--model", agyGoldenModel) ||
		!containsArgPair(request.Args, "--print-timeout", agyGoldenTimeout) ||
		!containsArg(request.Args, "--disable-slash-commands") ||
		!containsArg(request.Args, "--dangerously-skip-permissions") {
		t.Fatalf("provider argv = %#v, want structured AGY workspace-safe invocation", request.Args)
	}
	prompt := commandArgValue(request.Args, "-p")
	if !strings.Contains(prompt, clipPath) || !strings.Contains(prompt, shotSpecification) ||
		!strings.Contains(prompt, "only intended-behavior context") {
		t.Fatalf("provider prompt = %q, want exact clip path/spec and bounded context", prompt)
	}

	var schema map[string]any
	if err := json.Unmarshal([]byte(commandArgValue(request.Args, "--json-schema")), &schema); err != nil {
		t.Fatalf("decode provider JSON schema: %v; argv=%#v", err, request.Args)
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("provider JSON schema properties = %#v, want object", schema["properties"])
	}
	for _, field := range []string{
		"action_completed",
		"spec_deviations",
		"temporal_artifacts",
		"audio_content",
		"unexpected_speech",
		"verdict",
		"confidence",
	} {
		if _, ok := properties[field]; !ok {
			t.Fatalf("provider JSON schema missing required property %q: %#v", field, properties)
		}
	}
	confidence, ok := properties["confidence"].(map[string]any)
	if !ok {
		t.Fatalf("provider JSON schema confidence = %#v, want bounded number schema", properties["confidence"])
	}
	for key, want := range map[string]float64{"minimum": 0, "maximum": 1} {
		if got, ok := confidence[key].(float64); !ok || got != want {
			t.Fatalf("provider JSON schema confidence[%q] = %#v, want %v", key, confidence[key], want)
		}
	}
}

func assertAgyClipQAEvents(t *testing.T, events []factoryapi.FactoryEvent, wantOutput string) {
	t.Helper()

	if support.CountFactoryEvents(events, factoryapi.FactoryEventTypeWorkRequest) == 0 ||
		support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest) == 0 ||
		support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("Factory Events missing clip-QA Work Request or dispatch request/response")
	}
	assertAgyGoldenDispatch(t, events, factoryapi.WorkOutcomeAccepted, wantOutput)
}

func assertAgyColdWatchCommand(t *testing.T, request platformprocess.CommandRequest, assetPath string) {
	t.Helper()

	if request.Command != "agy" {
		t.Fatalf("provider command = %q, want agy", request.Command)
	}
	if !containsArgPair(request.Args, "--model", agyGoldenModel) ||
		!containsArgPair(request.Args, "--print-timeout", agyGoldenTimeout) {
		t.Fatalf("provider argv = %#v, want AGY model %s and timeout %s", request.Args, agyGoldenModel, agyGoldenTimeout)
	}
	if !containsArgPair(request.Args, "--output-format", "stream-json") ||
		!containsArgPair(request.Args, "--add-dir", request.WorkDir) ||
		!containsArg(request.Args, "--disable-slash-commands") ||
		!containsArg(request.Args, "--dangerously-skip-permissions") {
		t.Fatalf("provider argv = %#v, want AGY workspace-safe print invocation", request.Args)
	}
	prompt := commandArgValue(request.Args, "-p")
	if !strings.Contains(prompt, assetPath) ||
		!strings.Contains(prompt, "only creative input") {
		t.Fatalf("provider prompt = %q, want exact cut path and context-free review instruction", prompt)
	}
	for _, heading := range []string{
		"## Inspection status",
		"## Chronological events",
		"## Temporal or transient defects",
		"## Audio content and defects",
		"## Observed speech",
		"## Overall recommendation",
	} {
		if !strings.Contains(prompt, heading) {
			t.Fatalf("provider prompt missing report-contract heading %q: %s", heading, prompt)
		}
	}
}

func assertAgyColdWatchEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.WorkOutcome,
	wantOutput string,
) {
	t.Helper()

	if support.CountFactoryEvents(events, factoryapi.FactoryEventTypeWorkRequest) == 0 {
		t.Fatal("Factory Events missing Work Request for named cold-watch invocation")
	}
	if support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchRequest) == 0 ||
		support.CountFactoryEvents(events, factoryapi.FactoryEventTypeDispatchResponse) == 0 {
		t.Fatal("Factory Events missing cold-watch dispatch request/response")
	}
	assertAgyGoldenDispatch(t, events, wantOutcome, wantOutput)
}

func assertAgyColdWatchEventsFailure(t *testing.T, events []factoryapi.FactoryEvent, wantDiagnostic string) {
	t.Helper()

	if support.CountFactoryEvents(events, factoryapi.FactoryEventTypeWorkRequest) == 0 {
		t.Fatal("Factory Events missing Work Request for missing-file invocation")
	}
	assertAgyGoldenDispatchFailure(t, events)
	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil {
			continue
		}
		if observation.Response.Outcome != factoryapi.WorkOutcomeFailed &&
			observation.Response.Outcome != factoryapi.WorkOutcomeRejected {
			continue
		}
		if observation.Response.FailureDetail != nil &&
			strings.Contains(observation.Response.FailureDetail.Message, wantDiagnostic) {
			return
		}
		if observation.Response.Output != nil && strings.Contains(*observation.Response.Output, wantDiagnostic) {
			return
		}
		continue
	}
	t.Fatalf("failed cold-watch dispatch has no output containing %q", wantDiagnostic)
}

func invocationPrimaryText(t *testing.T, content factoryapi.WorkContent) string {
	t.Helper()

	for _, part := range content {
		textPart, err := part.AsWorkTextContentPart()
		if err == nil {
			return textPart.Text
		}
	}
	t.Fatalf("primaryResult = %#v, want a text report", content)
	return ""
}

func commandArgValue(args []string, flag string) string {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag {
			return args[index+1]
		}
	}
	return ""
}
