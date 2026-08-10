package agy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
	t.Run("cold-watch-video-and-audio", func(t *testing.T) {
		for _, test := range []struct {
			name         string
			trace        string
			asset        string
			wantResponse []string
		}{
			{
				name:  "video-watch",
				trace: "agy-trace-video-watch.stream.jsonl",
				asset: "clip-fixture.mp4",
				wantResponse: []string{
					"silver-haired woman",
					"ambient atmospheric drone",
					"clock ticking",
					"speech",
				},
			},
			{
				name:  "groundtruth-video",
				trace: "agy-trace-groundtruth-verbose.stream.jsonl",
				asset: "groundtruth-fixture.mp4",
				wantResponse: []string{
					"PHASE 1",
					"PHASE 2",
					"00:02.000",
					"440.00 Hz",
					"speech",
				},
			},
		} {
			test := test
			t.Run(test.name, func(t *testing.T) {
				response, events, runner, assetPath := runAgyColdWatchInvocation(
					t,
					test.trace,
					test.asset,
					false,
				)
				if response.Status != factoryapi.InvocationTerminalStatusCompleted {
					t.Fatalf("invocation status = %q, want COMPLETED; response=%#v", response.Status, response)
				}
				if response.PrimaryResult == nil {
					t.Fatal("primaryResult is nil, want cold-watch report")
				}
				result := invocationPrimaryText(t, *response.PrimaryResult)
				for _, want := range test.wantResponse {
					if !strings.Contains(result, want) {
						t.Fatalf("cold-watch primary result missing %q: %s", want, result)
					}
				}
				if runner.CallCount() != 1 {
					t.Fatalf("AGY provider command calls = %d, want exactly one", runner.CallCount())
				}
				assertAgyColdWatchCommand(t, runner.LastRequest(), assetPath)
				assertAgyColdWatchEvents(t, events, factoryapi.WorkOutcomeAccepted, result)
			})
		}
	})

	t.Run("missing-file-fails-work-after-provider-success", func(t *testing.T) {
		response, events, runner, assetPath := runAgyColdWatchInvocation(
			t,
			"agy-trace-missing-file.stream.jsonl",
			"does-not-exist-xyz.mp4",
			true,
		)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
		}
		if response.PrimaryResult != nil {
			t.Fatalf("primaryResult = %#v, want no recommendation for missing media", response.PrimaryResult)
		}
		if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
			t.Fatalf("failure message = %#v, want actionable non-empty diagnostic", response.Message)
		}
		if runner.CallCount() != 1 {
			t.Fatalf("AGY provider command calls = %d, want exactly one", runner.CallCount())
		}
		assertAgyColdWatchCommand(t, runner.LastRequest(), assetPath)
		providerResponse := agyGoldenInferenceResponse(t, events, factoryapi.InferenceOutcomeSucceeded)
		if providerResponse.Response == nil ||
			!strings.Contains(*providerResponse.Response, "does-not-exist-xyz.mp4") {
			t.Fatalf("provider refusal response = %#v, want missing-file explanation", providerResponse.Response)
		}
		assertAgyColdWatchEventsFailure(t, events, "does-not-exist-xyz.mp4")
	})

	t.Run("clip-qa-structured-pass-with-audio-evidence", func(t *testing.T) {
		response, events, runner, assetPath := runAgyClipQAInvocation(
			t,
			"agy-trace-clipqa-schema.stream.jsonl",
			"clip-fixture.mp4",
			agyClipQAShotSpec,
			platformprocess.CommandResult{ExitCode: 0},
			false,
		)
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
		if runner.CallCount() != 1 {
			t.Fatalf("AGY provider command calls = %d, want exactly one", runner.CallCount())
		}
		assertAgyClipQACommand(t, runner.LastRequest(), assetPath, agyClipQAShotSpec)
		assertAgyClipQAEvents(t, events, result)
	})

	t.Run("clip-qa-missing-file-fails-work", func(t *testing.T) {
		response, events, runner, assetPath := runAgyClipQAInvocation(
			t,
			"agy-trace-missing-file.stream.jsonl",
			"does-not-exist-xyz.mp4",
			agyClipQAShotSpec,
			platformprocess.CommandResult{ExitCode: 0},
			true,
		)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
		}
		if response.PrimaryResult != nil {
			t.Fatalf("primaryResult = %#v, want no production verdict for missing media", response.PrimaryResult)
		}
		if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
			t.Fatalf("failure message = %#v, want actionable non-empty diagnostic", response.Message)
		}
		if runner.CallCount() == 0 {
			t.Fatal("AGY provider command calls = 0, want at least one failure attempt")
		}
		assertAgyClipQACommand(t, runner.LastRequest(), assetPath, agyClipQAShotSpec)
		assertAgyGoldenDispatchFailure(t, events)
	})

	t.Run("clip-qa-schema-invalid-result-fails-work", func(t *testing.T) {
		response, events, runner, assetPath := runAgyClipQAInvocation(
			t,
			"agy-trace-structured.json",
			"clip-fixture.mp4",
			agyClipQAShotSpec,
			platformprocess.CommandResult{ExitCode: 0},
			true,
		)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
		}
		if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
			t.Fatalf("failure message = %#v, want schema diagnostic", response.Message)
		}
		if runner.CallCount() == 0 {
			t.Fatal("AGY provider command calls = 0, want at least one failure attempt")
		}
		assertAgyClipQACommand(t, runner.LastRequest(), assetPath, agyClipQAShotSpec)
		assertAgyGoldenDispatchFailure(t, events)
	})

	t.Run("clip-qa-provider-failure-fails-work", func(t *testing.T) {
		response, events, runner, assetPath := runAgyClipQAInvocation(
			t,
			"",
			"clip-fixture.mp4",
			agyClipQAShotSpec,
			platformprocess.CommandResult{Stderr: []byte("agy unavailable"), ExitCode: 17},
			true,
		)
		if response.Status != factoryapi.InvocationTerminalStatusFailed {
			t.Fatalf("invocation status = %q, want FAILED; response=%#v", response.Status, response)
		}
		if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
			t.Fatalf("failure message = %#v, want provider diagnostic", response.Message)
		}
		if runner.CallCount() == 0 {
			t.Fatal("AGY provider command calls = 0, want at least one failure attempt")
		}
		assertAgyClipQACommand(t, runner.LastRequest(), assetPath, agyClipQAShotSpec)
		assertAgyGoldenDispatchFailure(t, events)
	})
}

func runAgyColdWatchInvocation(
	t *testing.T,
	trace string,
	asset string,
	expectFailure bool,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *testutil.ProviderCommandRunner, string) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, agyColdWatchFactoryName)

	assetPath := filepath.Join(workingDirectory, asset)
	if !strings.Contains(asset, "does-not-exist") {
		if err := os.WriteFile(assetPath, readAgyGoldenAsset(t, asset), 0o644); err != nil {
			t.Fatalf("write AGY media fixture %q: %v", assetPath, err)
		}
	}

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   readAgyGoldenAsset(t, trace),
		ExitCode: 0,
	})
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	recordingPath := filepath.Join(t.TempDir(), "agy-cold-watch.replay.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", agyColdWatchFactoryName,
		"--cut-path", assetPath,
		"--record", recordingPath,
		"--output", "primary",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
	if !expectFailure && err != nil {
		t.Fatalf("Process.Execute(cold-watch) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if expectFailure && err == nil {
		t.Fatal("Process.Execute(cold-watch missing-file) error = nil, want terminal failure")
	}

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	events := readAgyColdWatchRecording(t, recordingPath)
	return response, events, runner, assetPath
}

func runAgyClipQAInvocation(
	t *testing.T,
	trace string,
	asset string,
	shotSpecification string,
	result platformprocess.CommandResult,
	expectFailure bool,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *testutil.ProviderCommandRunner, string) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, agyClipQAFactoryName)

	assetPath := filepath.Join(workingDirectory, asset)
	if !strings.Contains(asset, "does-not-exist") {
		if err := os.WriteFile(assetPath, readAgyGoldenAsset(t, asset), 0o644); err != nil {
			t.Fatalf("write AGY media fixture %q: %v", assetPath, err)
		}
	}
	if strings.TrimSpace(trace) != "" {
		result.Stdout = readAgyGoldenAsset(t, trace)
	}

	runner := testutil.NewProviderCommandRunner(result)
	process := support.BuildProcess(t, serviceedges.Edges{ProviderCommandRunner: runner})
	support.CleanupProcess(t, process)
	recordingPath := filepath.Join(t.TempDir(), "agy-clip-qa.replay.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", assetPath,
		"--shot-specification", shotSpecification,
		"--record", recordingPath,
		"--output", "primary",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = workingDirectory
	err := process.Execute(inputs.Input)
	if !expectFailure && err != nil {
		t.Fatalf("Process.Execute(clip-QA) error = %v\nstdout=%s\nstderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if expectFailure && err == nil {
		t.Fatal("Process.Execute(clip-QA failure) error = nil, want terminal failure")
	}

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	events := readAgyRecording(t, recordingPath, "AGY clip-QA")
	return response, events, runner, assetPath
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
		if observation.Response == nil || observation.Response.Output == nil {
			continue
		}
		if observation.Response.Outcome != factoryapi.WorkOutcomeFailed &&
			observation.Response.Outcome != factoryapi.WorkOutcomeRejected {
			continue
		}
		if !strings.Contains(*observation.Response.Output, wantDiagnostic) {
			t.Fatalf("failure output = %q, want actionable diagnostic containing %q", *observation.Response.Output, wantDiagnostic)
		}
		return
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
