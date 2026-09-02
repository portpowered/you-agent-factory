package agy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	agyGoldenModel             = "gemini-3.6-flash-high"
	agyGoldenTimeout           = "8m"
	agyGoldenVideoPrompt       = "Watch the video file clip-fixture.mp4 in the workspace. Describe the visual content and state whether the audio track contains speech, music, noise, or silence."
	agyGoldenGroundtruthPrompt = "Watch groundtruth-fixture.mp4 in the workspace and give an exhaustive review with exact duration, resolution, frame count and rate, PHASE text, the red-to-blue cut time, and the 440 Hz tone ending in silence."
	agyGoldenMissingPrompt     = "Watch does-not-exist-xyz.mp4 in the workspace and return the structured clip-QA verdict."
	agyGoldenStructuredSchema  = `{"type":"object","properties":{"sentiment":{"type":"string","enum":["positive","negative"]},"confidence":{"type":"number"}},"required":["sentiment","confidence"]}`
)

type agyGoldenCase struct {
	name             string
	trace            string
	asset            string
	prompt           string
	wantResponseText []string
	wantUsage        map[string]string
}

// TestAgyMultimodalGoldenThroughRootBuildProcess replays real AGY media
// recordings through root.BuildProcess and proves the selected provider,
// command effect, provider result, Worker result, Factory Events, and terminal
// Work projection remain aligned.
func TestAgyMultimodalGoldenThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	tests := []agyGoldenCase{
		{
			name:   "video-watch",
			trace:  "agy-trace-video-watch.stream.jsonl",
			asset:  "clip-fixture.mp4",
			prompt: agyGoldenVideoPrompt,
			wantResponseText: []string{
				"silver-haired woman",
				"clock face head",
				"ambient atmospheric drone",
				"clock ticking",
				"no speech or music",
			},
			wantUsage: map[string]string{
				"input_tokens":      "89393",
				"output_tokens":     "4622",
				"thinking_tokens":   "2312",
				"cache_read_tokens": "252517",
				"total_tokens":      "94015",
			},
		},
		{
			name:   "groundtruth-video",
			trace:  "agy-trace-groundtruth-verbose.stream.jsonl",
			asset:  "groundtruth-fixture.mp4",
			prompt: agyGoldenGroundtruthPrompt,
			wantResponseText: []string{
				"4.000000 seconds",
				"320 × 240 pixels",
				"25.00 FPS",
				"100 Frames",
				"PHASE 1",
				"PHASE 2",
				"00:02.000",
				"440.00 Hz",
				"Absolute Digital Silence",
			},
			wantUsage: map[string]string{
				"input_tokens":      "95455",
				"output_tokens":     "14178",
				"thinking_tokens":   "4738",
				"cache_read_tokens": "534149",
				"total_tokens":      "109633",
			},
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertAgyMultimodalGoldenCase(t, test)
		})
	}
}

func assertAgyMultimodalGoldenCase(t *testing.T, test agyGoldenCase) {
	t.Helper()

	fixture := agySharedProcess(t)
	_, listed, events, route, callStart := fixture.runGolden(t, "golden-"+test.name)

	assertAgyGoldenWorkCompleted(t, listed)
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("provider command runner calls for %q = %d, want exactly one", test.name, got)
	}
	assertAgyGoldenCommand(t, route.lastRequest(), route.workDir, test.prompt, "")

	providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
	if providerResponse.ProviderSession == nil ||
		providerResponse.ProviderSession.Provider == nil ||
		support.StringPointerValue(providerResponse.ProviderSession.Provider) != string(modelprovider.ProviderAntigravity) {
		t.Fatalf("provider session = %#v, want Antigravity metadata", providerResponse.ProviderSession)
	}
	if providerResponse.ProviderSession.Id == nil ||
		strings.TrimSpace(support.StringPointerValue(providerResponse.ProviderSession.Id)) == "" {
		t.Fatalf("provider session = %#v, want real trace session id", providerResponse.ProviderSession)
	}
	assertAgyGoldenUsage(t, providerResponse, test.wantUsage)
	if providerResponse.Response == nil {
		t.Fatal("provider response is nil")
	}
	response := *providerResponse.Response
	for _, want := range test.wantResponseText {
		if !strings.Contains(response, want) {
			t.Fatalf("provider response missing %q: %s", want, response)
		}
	}
	assertAgyGoldenDispatch(t, events, factoryapi.WorkOutcomeAccepted, response)
}

// TestAgyClipQAGoldenPassThroughRootBuildProcess proves the real clip-QA
// review survives the Provider and Worker boundaries.
func TestAgyClipQAGoldenPassThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, route, callStart := fixture.runGolden(t, "golden-clipqa")

	assertAgyGoldenWorkCompleted(t, listed)
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("provider command runner calls for clip-QA = %d, want exactly one", got)
	}
	assertAgyGoldenCommand(t, route.lastRequest(), route.workDir, agyGoldenVideoPrompt, "")

	providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
	if providerResponse.Response == nil {
		t.Fatal("clip-QA provider response is nil")
	}
	for _, want := range []string{
		"silver-haired woman",
		"clock-headed figure",
		"ambient drone",
		"clock-ticking",
		"Zero vocal formants",
	} {
		if !strings.Contains(*providerResponse.Response, want) {
			t.Fatalf("clip-QA provider response missing %q: %s", want, *providerResponse.Response)
		}
	}
	assertAgyGoldenDispatch(t, events, factoryapi.WorkOutcomeAccepted, *providerResponse.Response)
}

// TestAgyStructuredJSONGoldenThroughRootBuildProcess proves a recorded JSON
// envelope satisfies an authored schema at the Worker boundary instead of
// being accepted solely because the Provider returned exit-zero output.
func TestAgyStructuredJSONGoldenThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, route, callStart := fixture.runGolden(t, "golden-structured")

	assertAgyGoldenWorkCompleted(t, listed)
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("provider command runner calls for structured JSON = %d, want exactly one", got)
	}
	assertAgyGoldenCommand(t, route.lastRequest(), route.workDir,
		"Classify the statement as positive or negative and provide confidence.",
		agyGoldenStructuredSchema,
	)

	providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
	if providerResponse.Response == nil {
		t.Fatal("structured provider response is nil")
	}
	var output map[string]any
	if err := json.Unmarshal([]byte(*providerResponse.Response), &output); err != nil {
		t.Fatalf("decode structured provider response: %v", err)
	}
	if output["sentiment"] != "positive" || output["confidence"] != 0.98 {
		t.Fatalf("structured provider response = %#v, want positive/0.98", output)
	}
	assertAgyGoldenDispatch(t, events, factoryapi.WorkOutcomeAccepted, *providerResponse.Response)
}

// TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess proves a real AGY
// refusal with exit code zero cannot become successful Work merely because the
// native process reported status SUCCESS.
func TestAgyMissingFileRefusalFailsWorkThroughRootBuildProcess(t *testing.T) {
	t.Parallel()
	fixture := agySharedProcess(t)
	_, listed, events, route, callStart := fixture.runGolden(t, "golden-missing-file")

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := route.callCount() - callStart; got != 1 {
		t.Fatalf("provider command runner calls = %d, want exactly one", got)
	}
	assertAgyGoldenCommand(t, route.lastRequest(), route.workDir, agyGoldenMissingPrompt, "")

	providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
	if providerResponse.Response == nil ||
		!strings.Contains(*providerResponse.Response, "does-not-exist-xyz.mp4") {
		t.Fatalf("provider refusal response = %#v, want missing-file explanation", providerResponse.Response)
	}
	assertAgyGoldenDispatchFailure(t, events)
}

func agyGoldenWorkerConfig() string {
	return fmt.Sprintf(
		"---\n"+
			"type: MODEL_WORKER\n"+
			"model: %s\n"+
			"modelProvider: %s\n"+
			"skipPermissions: true\n"+
			"timeout: %s\n"+
			"---\n"+
			"Inspect the requested media and return the result.\n",
		agyGoldenModel,
		modelprovider.ProviderAntigravity,
		agyGoldenTimeout,
	)
}

func agyGoldenWorkerConfigWithStopToken(stopToken string) string {
	return strings.Replace(
		agyGoldenWorkerConfig(),
		"skipPermissions: true\n",
		"skipPermissions: true\nstopToken: "+stopToken+"\n",
		1,
	)
}

func agyGoldenWorkstationConfig(prompt, schema string) string {
	config := "---\n" +
		"type: MODEL_WORKSTATION\n"
	if strings.TrimSpace(schema) != "" {
		config += "outputSchema: '" + strings.ReplaceAll(schema, "'", "''") + "'\n"
	}
	return config + "---\n" + prompt + "\n"
}

func readAgyGoldenAsset(t *testing.T, name string) []byte {
	t.Helper()

	path := testutil.MustRepoPath(t, filepath.Join("tests", "functional", "providers", "agy", "testdata", name))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read AGY golden asset %s: %v", name, err)
	}
	return data
}

func copyAgyGoldenAsset(t *testing.T, dir, name string) {
	t.Helper()

	target := filepath.Join(dir, name)
	if err := os.WriteFile(target, readAgyGoldenAsset(t, name), 0o644); err != nil {
		t.Fatalf("copy AGY golden asset %s: %v", name, err)
	}
}

func assertAgyGoldenCommand(
	t *testing.T,
	request platformprocess.CommandRequest,
	workDir, prompt, schema string,
) {
	t.Helper()

	format := "stream-json"
	if strings.TrimSpace(schema) != "" {
		format = "json"
	}
	want := []string{
		"-p", prompt,
		"--output-format", format,
		"--add-dir", workDir,
		"--disable-slash-commands",
	}
	if strings.TrimSpace(schema) != "" {
		want = append(want, "--json-schema", schema)
	}
	want = append(
		want,
		"--model", agyGoldenModel,
		"--dangerously-skip-permissions",
		"--print-timeout", agyGoldenTimeout,
	)
	if request.Command != "agy" {
		t.Fatalf("provider command = %q, want agy", request.Command)
	}
	if request.WorkDir != workDir {
		t.Fatalf("provider workdir = %q, want factory dir %q", request.WorkDir, workDir)
	}
	if !reflect.DeepEqual(request.Args, want) {
		t.Fatalf("provider argv = %#v, want %#v", request.Args, want)
	}
}

func assertAgyGoldenWorkCompleted(t *testing.T, listed factoryapi.ListWorkResponse) {
	t.Helper()

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0; listed=%#v", got, listed)
	}
}

func agyGoldenInferenceResponse(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	want factoryapi.InferenceOutcome,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse &&
			event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode AGY provider response: %v", err)
		}
		if payload.Outcome == want {
			return payload
		}
	}
	t.Fatalf("missing AGY provider response with outcome %q", want)
	return factoryapi.InferenceResponseEventPayload{}
}

func assertAgyGoldenUsage(
	t *testing.T,
	response factoryapi.InferenceResponseEventPayload,
	want map[string]string,
) {
	t.Helper()

	if response.Diagnostics == nil || response.Diagnostics.Provider == nil ||
		response.Diagnostics.Provider.ResponseMetadata == nil {
		t.Fatalf("provider diagnostics = %#v, want authoritative usage metadata", response.Diagnostics)
	}
	metadata := *response.Diagnostics.Provider.ResponseMetadata
	for key, value := range want {
		if metadata[key] != value {
			t.Fatalf("provider usage %s = %q, want %q; metadata=%#v", key, metadata[key], value, metadata)
		}
	}
}

func assertAgyGoldenDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.WorkOutcome,
	wantOutput string,
) {
	t.Helper()

	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil {
			continue
		}
		if observation.Response.Outcome != wantOutcome {
			continue
		}
		if observation.Response.Output == nil || *observation.Response.Output != wantOutput {
			t.Fatalf("dispatch output = %#v, want provider output %q", observation.Response.Output, wantOutput)
		}
		return
	}
	t.Fatalf("missing dispatch response with outcome %q", wantOutcome)
}

func assertAgyGoldenDispatchFailure(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()

	for _, observation := range support.ObserveDispatchEvents(t, events) {
		if observation.Response == nil ||
			(observation.Response.Outcome != factoryapi.WorkOutcomeFailed &&
				observation.Response.Outcome != factoryapi.WorkOutcomeRejected) {
			continue
		}
		if observation.Response.FailureDetail != nil &&
			strings.TrimSpace(observation.Response.FailureDetail.Message) != "" {
			return
		}
		if observation.Response.Output != nil && strings.TrimSpace(*observation.Response.Output) != "" {
			return
		}
		t.Fatalf("dispatch %s has no actionable failure output: %#v", observation.Response.Outcome, observation.Response)
	}
	t.Fatal("missing failed or rejected dispatch response")
}
