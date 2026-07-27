package gemini_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	geminiGoldenTextSuccessCase         = "text-success"
	geminiGoldenRateLimitCase           = "rate-limit"
	geminiGoldenStructuredFailureCase   = "structured-failure"
	geminiGoldenTimeoutCase             = "timeout"
	geminiThrottleFailureReplayAttempts = 12
)

// TestGeminiGoldenTextSuccess replays a sanitized Gemini text-success transcript
// through the customer process boundary and proves successful text output with
// matching public Provider Session, response-event, and invocation-result metadata.
// golden: docs/temp/functional/provider-sessions/gemini/text-success/manifest.json
func TestGeminiGoldenTextSuccess(t *testing.T) {
	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(
		repoRoot,
		filepath.FromSlash(support.ProviderSessionFixturePath("gemini", geminiGoldenTextSuccessCase)),
	)

	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != "gemini-text-success" {
		t.Fatalf("manifest.ID = %q, want gemini-text-success", loaded.Manifest.ID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityFinalOnly,
		)
	}

	var request struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatalf("request.json = %#v, want model", request)
	}

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderGemini, request.Model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini golden text success"}`))

	exitCode := 0
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		30*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload, dispatchOutput := geminiGoldenInferenceObservation(t, events)
	if inferencePayload.Outcome != factoryapi.InferenceOutcomeSucceeded {
		t.Fatalf("inference outcome = %q, want SUCCEEDED", inferencePayload.Outcome)
	}
	if inferencePayload.Response == nil || *inferencePayload.Response != dispatchOutput {
		t.Fatalf("inference response text = %#v, want dispatch output %q", inferencePayload.Response, dispatchOutput)
	}
	if dispatchOutput == "" || !strings.Contains(dispatchOutput, "COMPLETE") {
		t.Fatalf("dispatch output = %q, want terminal COMPLETE-bearing success text", dispatchOutput)
	}

	assertGeminiFinalOnlyPublicResponseEvents(t, responseEvents)

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeGeminiProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeGeminiResponseEventGoldens(responseEvents),
		InvocationResult: observeGeminiInvocationResultGolden(inferencePayload, dispatchOutput),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

// TestGeminiGoldenRateLimitAndStructuredFailure replays sanitized Gemini rate-limit
// and structured-failure transcripts through the customer process boundary and
// proves those public failure classes remain distinct from each other and timeout.
// golden: docs/temp/functional/provider-sessions/gemini/rate-limit/manifest.json
// golden: docs/temp/functional/provider-sessions/gemini/structured-failure/manifest.json
func TestGeminiGoldenRateLimitAndStructuredFailure(t *testing.T) {
	t.Run("rate-limit", func(t *testing.T) {
		runGeminiFailureGoldenCase(
			t,
			geminiGoldenRateLimitCase,
			"gemini-rate-limit",
			factoryapi.WorkFailureTypeThrottled,
			[]factoryapi.WorkFailureType{factoryapi.WorkFailureTypePermanentBadRequest},
			true,
		)
	})
	t.Run("structured-failure", func(t *testing.T) {
		runGeminiFailureGoldenCase(
			t,
			geminiGoldenStructuredFailureCase,
			"gemini-structured-failure",
			factoryapi.WorkFailureTypePermanentBadRequest,
			[]factoryapi.WorkFailureType{factoryapi.WorkFailureTypeThrottled},
			false,
		)
	})
}

// TestGeminiGoldenTimeout replays a sanitized Gemini timeout transcript through
// the customer process boundary and proves a public timeout outcome distinct from
// rate-limit throttle and structured non-throttle failure classes.
// golden: docs/temp/functional/provider-sessions/gemini/timeout/manifest.json
func TestGeminiGoldenTimeout(t *testing.T) {
	runGeminiFailureGoldenCase(
		t,
		geminiGoldenTimeoutCase,
		"gemini-timeout",
		factoryapi.WorkFailureTypeTimeout,
		[]factoryapi.WorkFailureType{
			factoryapi.WorkFailureTypeThrottled,
			factoryapi.WorkFailureTypePermanentBadRequest,
		},
		true,
	)
}

// TestRootBuiltProcessExecutesThroughSharedSupport proves the shared harness executes a root-built process.
func TestRootBuiltProcessExecutesThroughSharedSupport(t *testing.T) {
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{"you", "--help"})

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(--help) error = %v\nstderr:\n%s", err, inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), "Run and manage") {
		t.Fatalf("Process.Execute(--help) stdout = %q, want root command help", inputs.Stdout())
	}
}

// TestGeminiConductorSuccessThroughRootBuildProcess proves successful Gemini execution through the product graph.
func TestGeminiConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini functional answer COMPLETE"),
	})

	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed place tokens = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1 through conductor path", runner.CallCount())
	}
	request := runner.LastRequest()
	if request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini (conductor-selected built-in)", request.Command)
	}
	if !containsArgPair(request.Args, "--model", "gemini-2.5-flash") {
		t.Fatalf("args = %#v, want --model gemini-2.5-flash", request.Args)
	}
}

// TestGeminiClassifierRejectsStructuredLabelThroughRootBuildProcess proves unsupported structured labels fail at the boundary.
func TestGeminiClassifierRejectsStructuredLabelThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	configureClassifierFixture(t, dir)
	support.WriteWorkstationConfig(t, dir, "process", `---
type: CLASSIFIER_WORKSTATION
---
Classify the work.
`)
	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderGemini, "gemini-2.5-flash"),
		"stopToken: COMPLETE\n",
		"",
		1,
	)
	support.WriteAgentConfig(t, dir, "worker", workerConfig)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini classifier"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: []byte(`{"label":"approved"}`)})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1 invalid classifier result; listed=%#v", got, listed)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.CallCount())
	}
}

// TestGeminiConductorPreservesConfiguredEnvironment proves configured environment reaches Gemini execution.
func TestGeminiConductorPreservesConfiguredEnvironment(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
env:
  GEMINI_CONTEXT_FIXTURE: configured
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini conductor context"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini context answer COMPLETE"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	request := runner.LastRequest()
	if !containsEnv(request.Env, "GEMINI_CONTEXT_FIXTURE=configured") {
		t.Fatalf("command environment omitted configured Gemini context")
	}
}

// TestGeminiRejectsUnsupportedWorkingDirectoryBeforeProviderIO proves invalid working directories fail before provider IO.
func TestGeminiRejectsUnsupportedWorkingDirectoryBeforeProviderIO(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
workingDirectory: .
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini rejects working directory"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("provider must not run"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1 unsupported-capability failure; listed=%#v", got, listed)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("gemini command runner calls = %d, want no provider I/O", runner.CallCount())
	}
}

// TestGeminiRejectsUnsupportedStructuredOutputBeforeProviderIO proves unsupported output fails before provider IO.
func TestGeminiRejectsUnsupportedStructuredOutputBeforeProviderIO(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	support.WriteWorkstationConfig(t, dir, "process", `---
type: MODEL_WORKSTATION
outputSchema: '{}'
worktree: .
---
Test workstation.
`)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini rejects structured output"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("provider must not run"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1 unsupported-capability failure; listed=%#v", got, listed)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("gemini command runner calls = %d, want no provider I/O", runner.CallCount())
	}
}

// TestGeminiConductorPreservesConfiguredSkipPermissions proves permission policy reaches Gemini execution.
func TestGeminiConductorPreservesConfiguredSkipPermissions(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	workerConfig := strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderGemini, "gemini-2.5-flash"),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	)
	support.WriteAgentConfig(t, dir, "worker", workerConfig)
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini skip permissions"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte("gemini policy answer COMPLETE"),
	})
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("terminal place tokens = %d, want 1 completed work item; listed=%#v", got, listed)
	}
	args := runner.LastRequest().Args
	if !containsArgPair(args, "--approval-mode", "yolo") ||
		!containsArgPair(args, "--sandbox", "false") {
		t.Fatalf("args = %#v, want configured Gemini skip-permissions flags", args)
	}
}

// TestGeminiNativeFailureThroughRootBuildProcessIsSafe proves native provider failures return safely.
func TestGeminiNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini native failure"}`))

	const leaked = "/tmp/secret-key"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte(`{"error":{"status":"UNAUTHENTICATED","message":"token path ` + leaked + ` leaked"}}`),
	})

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("done place tokens = %d, want 0 after native failure", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.CallCount())
	}
	if request := runner.LastRequest(); request.Command != "gemini" {
		t.Fatalf("command = %q, want gemini", request.Command)
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) ||
		strings.Contains(payload, "secret-key") {
		t.Fatalf("factory events leaked unsafe Gemini failure detail: %s", payload)
	}
}

// TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical proves cancellation returns the canonical outcome.
func TestGeminiCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderGemini,
		"gemini-2.5-flash",
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini command error"}`))

	runner := &commandCancellationRunner{}
	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(t, dir, serviceedges.Edges{
		ProviderCommandRunner: runner,
	}, 20*time.Second)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed place tokens = %d, want 1; listed=%#v", got, listed)
	}
	if runner.calls != 1 {
		t.Fatalf("gemini command runner calls = %d, want 1", runner.calls)
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal factory events: %v", err)
	}
	if strings.Contains(string(encoded), "Gemini command did not complete successfully") {
		t.Fatalf("factory events used Gemini-local cancellation fallback: %s", encoded)
	}
}

func runGeminiFailureGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
	wantReason factoryapi.WorkFailureType,
	notReasons []factoryapi.WorkFailureType,
	replayThrottleExhaustion bool,
) {
	t.Helper()

	loaded, model := loadGeminiFailureGoldenCase(t, caseName, manifestID)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderGemini, model))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"gemini golden `+caseName+`"}`))

	exitCode := 1
	if loaded.Process.ExitCode != nil {
		exitCode = *loaded.Process.ExitCode
	}
	transcript := platformprocess.CommandResult{
		Stdout:   append([]byte(nil), loaded.Stdout.Raw...),
		Stderr:   []byte(loaded.Stderr),
		ExitCode: exitCode,
	}
	var runner *testutil.ProviderCommandRunner
	if replayThrottleExhaustion {
		runner = testutil.NewProviderCommandRunner(
			repeatedGeminiCommandResults(transcript, geminiThrottleFailureReplayAttempts)...,
		)
	} else {
		runner = testutil.NewProviderCommandRunner(transcript)
	}

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		factoryRunTimeoutForGeminiFailureCase(replayThrottleExhaustion),
	)
	// Gemini failure classification closes without publishing provider response events.
	responseEvents := []factoryapi.FactoryResponseEvent{}

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if replayThrottleExhaustion {
		if runner.CallCount() < 2 {
			t.Fatalf("provider command runner calls = %d, want throttle retry exhaustion", runner.CallCount())
		}
	} else if runner.CallCount() != 1 {
		t.Fatalf("provider command runner calls = %d, want 1", runner.CallCount())
	}

	inferencePayload := geminiGoldenFailedInferenceObservation(t, events)
	if inferencePayload.FailureDetail == nil {
		t.Fatal("inference response missing failure detail")
	}
	if got := inferencePayload.FailureDetail.Reason; got != wantReason {
		t.Fatalf("failure reason = %q, want %q", got, wantReason)
	}
	for _, notReason := range notReasons {
		if inferencePayload.FailureDetail.Reason == notReason {
			t.Fatalf("failure reason = %q, must remain distinct from %q", inferencePayload.FailureDetail.Reason, notReason)
		}
	}
	if wantReason != factoryapi.WorkFailureTypeTimeout &&
		inferencePayload.FailureDetail.Reason == factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want non-timeout failure class", inferencePayload.FailureDetail.Reason)
	}

	observed := support.ProviderSessionObservedGoldens{
		ProviderSession:  observeGeminiProviderSessionGolden(inferencePayload, loaded.Manifest),
		ResponseEvents:   observeGeminiResponseEventGoldens(responseEvents),
		InvocationResult: observeGeminiFailedInvocationResultGolden(inferencePayload),
	}
	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		var updated *support.ProviderSessionGoldensUpdatedError
		if errors.As(err, &updated) {
			t.Fatalf("%v", err)
		}
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}

func loadGeminiFailureGoldenCase(
	t *testing.T,
	caseName string,
	manifestID string,
) (support.ProviderSessionCase, string) {
	t.Helper()
	caseDir := filepath.Join(
		testutil.MustRepoRoot(t),
		filepath.FromSlash(support.ProviderSessionFixturePath("gemini", caseName)),
	)
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase: %v", err)
	}
	if loaded.Manifest.ID != manifestID {
		t.Fatalf("manifest.ID = %q, want %s", loaded.Manifest.ID, manifestID)
	}
	if loaded.Manifest.FidelityClass != support.ProviderSessionFidelityFinalOnly {
		t.Fatalf(
			"manifest.fidelityClass = %q, want %q",
			loaded.Manifest.FidelityClass,
			support.ProviderSessionFidelityFinalOnly,
		)
	}
	var request struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(loaded.Request, &request); err != nil {
		t.Fatalf("decode request.json: %v", err)
	}
	if request.Model == "" {
		t.Fatalf("request.json = %#v, want model", request)
	}
	return loaded, request.Model
}

func repeatedGeminiCommandResults(
	transcript platformprocess.CommandResult,
	count int,
) []platformprocess.CommandResult {
	results := make([]platformprocess.CommandResult, count)
	for i := range results {
		results[i] = transcript
	}
	return results
}

func factoryRunTimeoutForGeminiFailureCase(replayThrottleExhaustion bool) time.Duration {
	if replayThrottleExhaustion {
		return 90 * time.Second
	}
	return 30 * time.Second
}

func assertGeminiFinalOnlyPublicResponseEvents(t *testing.T, events []factoryapi.FactoryResponseEvent) {
	t.Helper()

	var completedMessages int
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool:
			t.Fatalf("final-only replay fabricated tool lifecycle: %#v", event)
		case factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only replay fabricated usage lifecycle: %#v", event)
		}
	}
	if completedMessages == 0 {
		t.Fatal("final-only replay missing authoritative completed message")
	}
}

func geminiGoldenInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) (factoryapi.InferenceResponseEventPayload, string) {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
		dispatchOutput   string
	)
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeInferenceResponse:
			payload, err := event.Payload.AsInferenceResponseEventPayload()
			if err != nil {
				t.Fatalf("decode inference response: %v", err)
			}
			if payload.Outcome != factoryapi.InferenceOutcomeSucceeded {
				continue
			}
			inferencePayload = payload
			foundInference = true
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				t.Fatalf("decode dispatch response: %v", err)
			}
			if payload.Output != nil && *payload.Output != "" {
				dispatchOutput = *payload.Output
			}
		}
	}
	if !foundInference {
		t.Fatal("missing succeeded INFERENCE_RESPONSE in factory events")
	}
	if dispatchOutput == "" {
		t.Fatal("missing dispatch output in factory events")
	}
	return inferencePayload, dispatchOutput
}

func geminiGoldenFailedInferenceObservation(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) factoryapi.InferenceResponseEventPayload {
	t.Helper()

	var (
		inferencePayload factoryapi.InferenceResponseEventPayload
		foundInference   bool
	)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != factoryapi.InferenceOutcomeFailed {
			continue
		}
		inferencePayload = payload
		foundInference = true
	}
	if !foundInference {
		t.Fatal("missing failed INFERENCE_RESPONSE in factory events")
	}
	return inferencePayload
}

func observeGeminiProviderSessionGolden(
	payload factoryapi.InferenceResponseEventPayload,
	manifest support.ProviderSessionGoldenManifest,
) json.RawMessage {
	status := "failed"
	if payload.Outcome == factoryapi.InferenceOutcomeSucceeded {
		status = "completed"
	}
	provider := string(modelprovider.ProviderGemini)
	sessionID := ""
	if payload.ProviderSession != nil {
		if payload.ProviderSession.Provider != nil {
			provider = support.StringPointerValue(payload.ProviderSession.Provider)
		}
		if payload.ProviderSession.Id != nil {
			sessionID = support.StringPointerValue(payload.ProviderSession.Id)
		}
	}
	record := map[string]string{
		"provider":          provider,
		"providerSessionId": sessionID,
		"fidelityClass":     manifest.FidelityClass,
		"status":            status,
	}
	return mustMarshalJSON(record)
}

func observeGeminiResponseEventGoldens(events []factoryapi.FactoryResponseEvent) []json.RawMessage {
	records := make([]json.RawMessage, 0, len(events))
	for _, event := range events {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindError:
			if event.Phase != factoryapi.FactoryResponseEventPhaseFailed {
				continue
			}
			errorPayload, err := event.Payload.AsFactoryResponseEventErrorPayload()
			if err != nil {
				continue
			}
			record := map[string]any{
				"type":             "error.failed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           geminiGoldenItemID(event),
				"code":             errorPayload.Code,
				"message":          errorPayload.Message,
			}
			if errorPayload.Retryable != nil {
				record["retryable"] = *errorPayload.Retryable
			}
			records = append(records, mustMarshalJSON(record))
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
				continue
			}
			message, err := event.Payload.AsFactoryResponseEventMessagePayload()
			if err != nil {
				continue
			}
			text := geminiGoldenMessageText(message)
			if text == "" {
				continue
			}
			record := map[string]any{
				"type":             "message.completed",
				"eventId":          event.EventId,
				"factorySessionId": event.FactorySessionId,
				"runId":            event.RunId,
				"itemId":           geminiGoldenItemID(event),
				"text":             text,
				"finishReason":     "stop",
			}
			records = append(records, mustMarshalJSON(record))
		}
	}
	return records
}

func observeGeminiInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
	dispatchOutput string,
) json.RawMessage {
	ok := payload.Outcome == factoryapi.InferenceOutcomeSucceeded
	content := dispatchOutput
	if payload.Response != nil && *payload.Response != "" {
		content = *payload.Response
	}
	record := map[string]any{
		"ok":           ok,
		"content":      content,
		"finishReason": "stop",
	}
	return mustMarshalJSON(record)
}

func observeGeminiFailedInvocationResultGolden(
	payload factoryapi.InferenceResponseEventPayload,
) json.RawMessage {
	record := map[string]any{
		"ok": false,
	}
	if payload.FailureDetail != nil {
		record["failureReason"] = payload.FailureDetail.Reason
		record["message"] = payload.FailureDetail.Message
	}
	return mustMarshalJSON(record)
}

func geminiGoldenItemID(event factoryapi.FactoryResponseEvent) string {
	if event.ItemId != nil && *event.ItemId != "" {
		return *event.ItemId
	}
	return ""
}

func geminiGoldenMessageText(message factoryapi.FactoryResponseEventMessagePayload) string {
	for _, block := range message.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err != nil {
			continue
		}
		if text.Text != "" {
			return text.Text
		}
	}
	return ""
}

func mustMarshalJSON(value any) json.RawMessage {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return encoded
}

type commandCancellationRunner struct {
	calls int
}

func configureClassifierFixture(t *testing.T, dir string) {
	t.Helper()
	path := filepath.Join(dir, "factory.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read classifier fixture: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal(raw, &config); err != nil {
		t.Fatalf("decode classifier fixture: %v", err)
	}
	workstations := config["workstations"].([]any)
	workstation := workstations[0].(map[string]any)
	workstation["type"] = "CLASSIFIER_WORKSTATION"
	delete(workstation, "outputs")
	workstation["classificationRoutes"] = []map[string]any{{
		"label": "approved",
		"outputs": []map[string]any{{
			"state": "done", "workType": "task",
		}},
	}}
	updated, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("encode classifier fixture: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatalf("write classifier fixture: %v", err)
	}
}

func (r *commandCancellationRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls++
	return platformprocess.CommandResult{}, context.Canceled
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func containsEnv(env []string, expected string) bool {
	for _, value := range env {
		if value == expected {
			return true
		}
	}
	return false
}
