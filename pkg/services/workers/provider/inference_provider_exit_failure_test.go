package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

// pkgmaintcheck:ignore-cyclomatic-complexity service-ownership migration preserves this decision flow; simplify branches and remove this exemption.
func TestNewFromInputRejectsMissingRequiredEdges(t *testing.T) {
	t.Parallel()
	allocator := &agypty.MockAllocator{}
	sequence := []string{}
	runner := &preparedInvocationTestRunner{sequence: &sequence}
	resolveSymlinks := func(path string) (string, error) { return path, nil }
	clock := platformclock.Real{}
	if _, err := NewFactory(nil, clock, allocator, resolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "command runner is required") {
		t.Fatalf("missing command runner error = %v", err)
	}
	if _, err := NewFactory(runner, clock, nil, resolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "Agy PTY allocator is required") {
		t.Fatalf("missing Agy allocator error = %v", err)
	}
	if _, err := NewFactory(runner, clock, allocator, nil, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "symlink resolver is required") {
		t.Fatalf("missing symlink resolver error = %v", err)
	}
	if _, err := NewFactory(runner, clock, allocator, resolveSymlinks, nil, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "executable locator is required") {
		t.Fatalf("missing executable locator error = %v", err)
	}
	if _, err := NewFactory(runner, clock, allocator, resolveSymlinks, platformprocess.HostExecutableLocator{}, nil, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "executable path inspector is required") {
		t.Fatalf("missing executable path inspector error = %v", err)
	}
	if _, err := NewFactory(runner, clock, allocator, resolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, nil, "linux"); err == nil || !strings.Contains(err.Error(), "executable file reader is required") {
		t.Fatalf("missing executable file reader error = %v", err)
	}
	if _, err := NewFactory(runner, clock, allocator, resolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, ""); err == nil || !strings.Contains(err.Error(), "operating system is required") {
		t.Fatalf("missing operating system error = %v", err)
	}
	if _, err := NewFactory(runner, nil, allocator, resolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "command clock is required") {
		t.Fatalf("missing command clock error = %v", err)
	}
	if _, err := NewFactory(runner, clock, allocator, resolveSymlinks, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux"); err == nil || !strings.Contains(err.Error(), "temporary filesystem is required") {
		t.Fatalf("missing temporary filesystem error = %v", err)
	}
}

func TestNewFromInputKeepsProviderCommandAndAgyPTYEdgesDistinct(t *testing.T) {
	t.Parallel()
	allocator := &agypty.MockAllocator{}
	sequence := []string{}
	runner := &preparedInvocationTestRunner{sequence: &sequence}
	clock := platformclock.Real{}
	factory, err := NewFactory(runner, clock, allocator, func(path string) (string, error) { return path, nil }, platformprocess.HostExecutableLocator{}, platformfilesystem.Local{}, platformfilesystem.Local{}, "linux", platformfilesystem.Local{})
	if err != nil {
		t.Fatalf("NewFactory: %v", err)
	}
	built, err := factory.New(false, nil, nil, nil)
	if err != nil {
		t.Fatalf("NewFromInput() error = %v", err)
	}
	loggingRunner, ok := built.exec.(*LoggingCommandRunner)
	if !ok || loggingRunner.Runner != runner || loggingRunner.Clock != clock {
		t.Fatalf("constructed edges = (%#v, %T), want selected runner and clock", built.exec, built.exec)
	}
}

func TestFactoryNewRejectsMissingProviderTimingClock(t *testing.T) {
	t.Parallel()
	factory := &Factory{}
	if _, err := factory.New(false, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "command clock is required") {
		t.Fatalf("missing provider timing clock error = %v", err)
	}
}

func TestFactoryNewUsesInjectedClockForProviderDiagnostics(t *testing.T) {
	t.Parallel()
	base := time.Unix(1_700_000_000, 0)
	clock := &providerSequenceClock{times: []time.Time{
		base,
		base.Add(time.Second),
		base.Add(2 * time.Second),
		base.Add(9 * time.Second),
	}}
	sequence := []string{}
	runner := &preparedInvocationTestRunner{sequence: &sequence}
	factory, err := NewFactory(
		runner,
		clock,
		&agypty.MockAllocator{},
		func(path string) (string, error) { return path, nil },
		platformprocess.HostExecutableLocator{},
		platformfilesystem.Local{},
		platformfilesystem.Local{},
		"linux",
		platformfilesystem.Local{},
	)
	if err != nil {
		t.Fatalf("NewFactory() error = %v", err)
	}
	provider, err := factory.New(false, nil, nil, nil)
	if err != nil {
		t.Fatalf("Factory.New() error = %v", err)
	}
	response, err := provider.Infer(t.Context(), workerexecution.ProviderInferenceRequest{
		ModelProvider: nativeScriptWrapHarnessProvider,
		UserMessage:   "measure this provider invocation",
	})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if response.Diagnostics == nil || response.Diagnostics.Command == nil {
		t.Fatalf("diagnostics = %#v, want command timing", response.Diagnostics)
	}
	if got, want := response.Diagnostics.Command.Duration, 9*time.Second; got != want {
		t.Fatalf("command duration = %s, want %s", got, want)
	}
}

type providerSequenceClock struct {
	times []time.Time
	next  int
}

func (c *providerSequenceClock) Now() time.Time {
	if c.next >= len(c.times) {
		panic("provider sequence clock exhausted")
	}
	value := c.times[c.next]
	c.next++
	return value
}

func TestScriptWrapProvider_Infer_GenericNonCodexExitFailuresPreserveMessageAndClassification(t *testing.T) {
	t.Parallel()
	cases := genericNonCodexExitFailureTestCases()
	if len(cases) == 0 {
		t.Skip("no ScriptWrap-only non-Codex exit failure cases remain after conductor cutover")
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assertInferenceExitFailure(t, tc)
		})
	}
}

func TestScriptWrapProvider_Infer_CodexGPT56SolFailureUsesCanonicalResultAndDecision(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	entry := providerErrorCorpusEntryForTest(t, "codex_gpt_5_6_sol_requires_newer_cli")
	result := entry.CommandResult()
	result.Stderr = append([]byte("session_id: sess-codex-gpt-5-6-sol\n"), result.Stderr...)
	fakeExec := &recordingProviderExec{result: result}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5.6-sol",
		UserMessage:   "private prompt that must not appear in the failure",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	assertGPT56SolCanonicalFailure(t, providerErr)
	assertGPT56SolFailureMetadata(t, providerErr, entry, result)
}

func TestScriptWrapProvider_Infer_LogsCorrelatedNormalizedCodexFailureAfterParsing(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	const prompt = "synthetic prompt must not appear"
	const credential = "credential-value-must-not-appear"
	sequence := []string{}
	logger := &preparedInvocationTestLogger{sequence: &sequence}
	runner := &preparedInvocationTestRunner{
		sequence: &sequence,
		result: CommandResult{
			ExitCode: 17,
			Stderr:   []byte(`ERROR: {"type":"invalid_request_error","status":400,"message":"The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."}` + "\n" + credential),
		},
	}
	provider := NewScriptWrapProviderWithDependencies(false, logger, runner, nil, nil, nil, nil)
	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		Model:         "gpt-5.6-sol",
		UserMessage:   prompt,
		EnvVars:       map[string]string{"API_TOKEN": credential},
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-failure-1",
			Execution: work.ExecutionMetadata{
				RequestID: "request-failure-1", TraceID: "trace-failure-1", WorkIDs: []string{"work-failure-1", "work-failure-2"},
			},
		},
	})
	if err == nil {
		t.Fatal("Infer returned nil error")
	}
	if logger.failureCount != 1 {
		t.Fatalf("normalized failure records = %d, want 1", logger.failureCount)
	}
	if len(sequence) < 3 || sequence[0] != ProviderInvocationPrepared || sequence[1] != "runner" || sequence[2] != ProviderFailureNormalized {
		t.Fatalf("record sequence = %#v, want prepared, runner, normalized failure", sequence)
	}
	assertNormalizedFailureFields(t, logger.failureFields)
	if strings.Contains(logger.allValues, prompt) || strings.Contains(logger.allValues, credential) {
		t.Fatalf("provider logs contain prompt or credential: %s", logger.allValues)
	}
}

func assertNormalizedFailureFields(t *testing.T, fields map[string]any) {
	t.Helper()
	if fields["provider"] != "codex" || fields["model"] != "gpt-5.6-sol" {
		t.Fatalf("provider/model = %#v/%#v", fields["provider"], fields["model"])
	}
	if fields["failure_reason"] != workerexecution.WorkFailureTypePermanentBadRequest || fields["failure_message"] != codexGPT56SolUpgradeMessage {
		t.Fatalf("canonical failure = %#v", fields)
	}
	if fields["retryable"] != false || fields["exit_code"] != 17 {
		t.Fatalf("retry/exit fields = %#v", fields)
	}
	if fields["request_id"] != "request-failure-1" || fields["trace_id"] != "trace-failure-1" || fields["work_id"] != "work-failure-1" || fields["dispatch_id"] != "dispatch-failure-1" {
		t.Fatalf("correlation fields = %#v", fields)
	}
	if _, ok := fields["duration_ms"]; !ok {
		t.Fatalf("duration_ms absent: %#v", fields)
	}
}

func TestScriptWrapProvider_Infer_LogsNormalizedFailuresWithoutSyntheticExitCodes(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	for _, tc := range []struct {
		name          string
		err           error
		wantReason    workerexecution.WorkFailureType
		wantRetryable bool
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantReason: workerexecution.WorkFailureTypeTimeout, wantRetryable: true},
		{name: "command start", err: exec.ErrNotFound, wantReason: workerexecution.WorkFailureTypeMissingExecutable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sequence := []string{}
			logger := &preparedInvocationTestLogger{sequence: &sequence}
			runner := &preparedInvocationTestRunner{sequence: &sequence, err: tc.err}
			provider := NewScriptWrapProviderWithDependencies(false, logger, runner, nil, nil, nil, nil)
			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderClaude),
				UserMessage:   "private prompt",
				Dispatch:      work.WorkDispatch{DispatchID: "dispatch-no-exit"},
			})
			if err == nil {
				t.Fatal("Infer returned nil error")
			}
			if logger.failureCount != 1 || logger.failureFields["failure_reason"] != tc.wantReason || logger.failureFields["retryable"] != tc.wantRetryable {
				t.Fatalf("normalized failure fields = %#v", logger.failureFields)
			}
			if _, ok := logger.failureFields["exit_code"]; ok {
				t.Fatalf("failure record invented exit code: %#v", logger.failureFields)
			}
			if logger.failureFields["model"] != ProviderDefaultModel {
				t.Fatalf("model = %#v, want provider default", logger.failureFields["model"])
			}
		})
	}
}

func TestScriptWrapProvider_Infer_CodexExecutionFailureJSONLogsExcludeCommandOutput(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	const prompt = "codex execution prompt must not appear"
	const stdoutSecret = "stdout-secret-must-not-appear"
	const stderrSecret = "stderr-secret-must-not-appear"
	for _, tc := range []struct {
		name        string
		err         error
		wantReason  workerexecution.WorkFailureType
		wantMessage string
	}{
		{name: "timeout", err: context.DeadlineExceeded, wantReason: workerexecution.WorkFailureTypeTimeout, wantMessage: "Provider request timed out."},
		{name: "command start", err: exec.ErrNotFound, wantReason: workerexecution.WorkFailureTypeMissingExecutable, wantMessage: "Provider executable could not be found."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			encoderConfig := zap.NewProductionEncoderConfig()
			core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(&output), zapcore.DebugLevel)
			provider := NewScriptWrapProviderWithDependencies(false, logging.NewZapLogger(zap.New(core), false), &recordingProviderExec{
				result: CommandResult{Stdout: []byte(stdoutSecret), Stderr: []byte(stderrSecret)},
				err:    tc.err,
			}, nil, nil, nil, nil)

			_, _ = provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderCodex),
				UserMessage:   prompt,
			})

			record := normalizedFailureJSONRecord(t, output.String())
			if record["failure_reason"] != string(tc.wantReason) || record["failure_message"] != tc.wantMessage {
				t.Fatalf("normalized JSON failure = %#v", record)
			}
			encoded, err := json.Marshal(record)
			if err != nil {
				t.Fatalf("encode normalized failure: %v", err)
			}
			for _, secret := range []string{prompt, stdoutSecret, stderrSecret} {
				if strings.Contains(string(encoded), secret) {
					t.Fatalf("normalized JSON failure leaked %q: %s", secret, encoded)
				}
			}
		})
	}
}

func normalizedFailureJSONRecord(t *testing.T, logs string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON log: %v", err)
		}
		if record["event_name"] == ProviderFailureNormalized {
			return record
		}
	}
	t.Fatalf("normalized failure absent from JSON logs: %s", logs)
	return nil
}

func TestScriptWrapProvider_Infer_CodexUpgradeFailureIsSearchableInJSONLogs(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	var output bytes.Buffer
	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(&output), zapcore.DebugLevel)
	provider := NewScriptWrapProviderWithDependencies(false, logging.NewZapLogger(zap.New(core), false), &recordingProviderExec{result: CommandResult{
		ExitCode: 2,
		Stderr:   []byte(`ERROR: {"type":"invalid_request_error","status":400,"message":"The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."}`),
	}}, nil, nil, nil, nil)

	_, _ = provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCodex),
		UserMessage:   "json log prompt fixture",
	})

	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode JSON log: %v", err)
		}
		if record["event_name"] != ProviderFailureNormalized {
			continue
		}
		if record["failure_reason"] != string(workerexecution.WorkFailureTypePermanentBadRequest) || record["failure_message"] != codexGPT56SolUpgradeMessage {
			t.Fatalf("normalized JSON failure = %#v", record)
		}
		if strings.Contains(line, "json log prompt fixture") || strings.Contains(line, `\"status\":400`) {
			t.Fatalf("normalized JSON failure leaked prompt or envelope: %s", line)
		}
		return
	}
	t.Fatalf("normalized failure absent from JSON logs: %s", output.String())
}

func assertGPT56SolCanonicalFailure(t *testing.T, providerErr *ProviderError) {
	t.Helper()

	if providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, workerexecution.WorkFailureTypePermanentBadRequest)
	}
	const wantMessage = "The 'gpt-5.6-sol' model requires a newer version of Codex. Please upgrade to the latest app or CLI and try again."
	if providerErr.Message != wantMessage {
		t.Fatalf("provider error message = %q, want %q", providerErr.Message, wantMessage)
	}
	if providerErr.Family != workerexecution.WorkFailureFamilyTerminal {
		t.Fatalf("provider error family = %q, want %q", providerErr.Family, workerexecution.WorkFailureFamilyTerminal)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if decision.Retryable || !decision.Terminal || decision.TriggersThrottlePause {
		t.Fatalf("provider failure decision = %#v, want terminal without retry or throttle pause", decision)
	}
}

func assertGPT56SolFailureMetadata(t *testing.T, providerErr *ProviderError, entry ProviderErrorCorpusEntry, result CommandResult) {
	t.Helper()

	if providerErr.ProviderSession == nil || providerErr.ProviderSession.ID != "sess-codex-gpt-5-6-sol" {
		t.Fatalf("provider session = %#v, want captured Codex session", providerErr.ProviderSession)
	}
	if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on provider error")
	}
	if providerErr.Diagnostics.Command.ExitCode != entry.ExitCode || providerErr.Diagnostics.Command.Stderr != string(result.Stderr) {
		t.Fatalf("command diagnostics = %#v, want captured exit code and stderr", providerErr.Diagnostics.Command)
	}
	if providerErr.Diagnostics.Command.TimedOut {
		t.Fatal("expected non-timeout Codex failure diagnostics")
	}
	if providerErr.Cause == nil {
		t.Fatal("expected bounded internal cause on Codex exit failure")
	}
	if !strings.Contains(providerErr.Cause.Error(), "gpt-5.6-sol") {
		t.Fatalf("provider error cause = %v, want audited upgrade diagnostic", providerErr.Cause)
	}
}

type exitFailureInferenceTestCase struct {
	name        string
	provider    string
	result      CommandResult
	wantMessage string
	wantType    workerexecution.WorkFailureType
}

func genericNonCodexExitFailureTestCases() []exitFailureInferenceTestCase {
	return nil
}

func assertInferenceExitFailure(t *testing.T, tc exitFailureInferenceTestCase) {
	t.Helper()

	fakeExec := &recordingProviderExec{result: tc.result}
	provider := newScriptWrapProviderForTest(t, fakeExec, tc.provider)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: tc.provider,
		UserMessage:   "run the task",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != tc.wantType {
		t.Fatalf("provider error type = %q, want %q", providerErr.Type, tc.wantType)
	}
	if providerErr.Message != tc.wantMessage {
		t.Fatalf("provider error message = %q, want %q", providerErr.Message, tc.wantMessage)
	}
}

func TestProgressStreamIdentity_SelectsProviderOwnedObservers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		command  string
		identity adapter.Identity
	}{
		{command: "claude", identity: adapter.Identity(modelprovider.ProviderClaude)},
		{command: "opencode", identity: adapter.Identity(modelprovider.ProviderOpenCode)},
	}
	for _, tc := range tests {
		if got := progressStreamIdentity(tc.command); got != tc.identity {
			t.Fatalf("progressStreamIdentity(%q) = %q, want %q", tc.command, got, tc.identity)
		}
	}
}

func TestIsCodexCommand_AcceptsNativeExecutableShapes(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	for _, command := range []string{"codex", "codex.exe", `C:\tools\codex.cmd`, "/usr/local/bin/codex"} {
		if !isCodexCommand(command) {
			t.Fatalf("isCodexCommand(%q) = false, want true", command)
		}
	}
}

func TestParseOpenCodeProviderFailure_KnownCorpusShapesUseCanonicalContract(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		wantMessage string
	}{
		{name: "opencode_provider_auth_error", wantMessage: "Authentication required for openai. Run opencode auth login."},
		{name: "opencode_invalid_request_api_error", wantMessage: "The selected model does not support this request."},
		{name: "opencode_rate_limit_text", wantMessage: opencodeThrottleFailureMessage},
		{name: "opencode_timeout_error", wantMessage: opencodeTimeoutFailureMessage},
		{name: "opencode_server_api_error", wantMessage: opencodeServerFailureMessage},
	}

	for _, tc := range testCases {
		entry := providerErrorCorpusEntryForTest(t, tc.name)
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOpenCodeProviderFailure(entry.CommandResult())
			if got.Reason != entry.ExpectedType || got.Message != tc.wantMessage {
				t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want reason=%q message=%q", got, entry.ExpectedType, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestParseOpenCodeProviderFailure_StructuredFailurePrecedesText(t *testing.T) {
	t.Parallel()
	result := CommandResult{
		ExitCode: 1,
		Stderr:   []byte("Error: rate limit exceeded"),
		Stdout:   []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"Choose a supported model."}}}`),
	}

	got := ParseOpenCodeProviderFailure(result)
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != "Choose a supported model." {
		t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want structured bad request", got)
	}
}

func TestParseOpenCodeProviderFailure_SanitizesKnownActionableDetails(t *testing.T) {
	t.Parallel()
	result := CommandResult{
		ExitCode: 1,
		Stdout: []byte(`{"type":"error","error":{"name":"APIError","data":{"statusCode":400,"message":"prompt: ` +
			strings.Repeat("private ", 100) + ` Authorization: Bearer secret-token"}}}`),
	}

	got := ParseOpenCodeProviderFailure(result)
	if got.Reason != workerexecution.WorkFailureTypePermanentBadRequest || got.Message != opencodeBadRequestFailureMessage {
		t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want sanitized fixed bad-request message", got)
	}
	if len(got.Message) > opencodeFailureMessageBytes || strings.Contains(got.Message, "secret-token") || strings.Contains(got.Message, "private") {
		t.Fatalf("message = %q, want bounded message without sensitive detail", got.Message)
	}
}

func TestParseOpenCodeProviderFailure_UnknownFailuresUseSafeBoundedExcerptOrExitCode(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name        string
		result      CommandResult
		wantMessage string
	}{
		{
			name:        "safe error line",
			result:      CommandResult{ExitCode: 17, Stderr: []byte("loading project\nError: plugin initialization failed\nrendering prompt")},
			wantMessage: "Error: plugin initialization failed",
		},
		{
			name:        "unrecognized structured error",
			result:      CommandResult{ExitCode: 18, Stdout: []byte(`{"type":"error","error":{"name":"PluginError","data":{"message":"Plugin initialization failed."}}}`)},
			wantMessage: "Plugin initialization failed.",
		},
		{
			name:        "oversized safe error",
			result:      CommandResult{ExitCode: 19, Stderr: []byte("Error: " + strings.Repeat("x", opencodeFailureMessageBytes))},
			wantMessage: ("Error: " + strings.Repeat("x", opencodeFailureMessageBytes))[:opencodeFailureMessageBytes],
		},
		{
			name:        "empty output",
			result:      CommandResult{ExitCode: 20},
			wantMessage: "opencode exited with code 20",
		},
		{
			name:        "malformed structured output",
			result:      CommandResult{ExitCode: 21, Stdout: []byte(`{"type":"error","message":"unfinished"`)},
			wantMessage: "opencode exited with code 21",
		},
		{
			name:        "transcript noise",
			result:      CommandResult{ExitCode: 22, Stderr: []byte("user: show credentials\nassistant: working\nprompt: private request")},
			wantMessage: "opencode exited with code 22",
		},
		{
			name:        "secret bearing error",
			result:      CommandResult{ExitCode: 23, Stderr: []byte("Error: Authorization: Bearer secret-token")},
			wantMessage: "opencode exited with code 23",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseOpenCodeProviderFailure(tc.result)
			if got.Reason != workerexecution.WorkFailureTypeUnknown || got.Message != tc.wantMessage {
				t.Fatalf("ParseOpenCodeProviderFailure() = %#v, want unknown message %q", got, tc.wantMessage)
			}
			if len(got.Message) > opencodeFailureMessageBytes {
				t.Fatalf("message length = %d, want at most %d", len(got.Message), opencodeFailureMessageBytes)
			}
		})
	}
}

func TestNormalizeProviderExitFailure_SelectsOpenCodeParserFromNormalizedIdentity(t *testing.T) {
	t.Parallel()
	entry := providerErrorCorpusEntryForTest(t, "opencode_rate_limit_text")
	providerErr := normalizeProviderExitFailure("  OPENCODE  ", entry.CommandResult(), nil, nil)

	if providerErr.Type != workerexecution.WorkFailureTypeThrottled || providerErr.Message != opencodeThrottleFailureMessage {
		t.Fatalf("normalizeProviderExitFailure() = %#v, want OpenCode throttle failure", providerErr)
	}
	decision := WorkFailureDecisionFromProviderError(providerErr)
	if !decision.Retryable || decision.Terminal || !decision.TriggersThrottlePause {
		t.Fatalf("decision = %#v, want central throttle policy", decision)
	}
}

func TestNormalizeProviderExitFailure_OpenCodeCorpusUsesCentralPolicy(t *testing.T) {
	t.Parallel()
	for _, name := range []string{
		"opencode_provider_auth_error",
		"opencode_invalid_request_api_error",
		"opencode_rate_limit_text",
		"opencode_timeout_error",
		"opencode_server_api_error",
	} {
		entry := providerErrorCorpusEntryForTest(t, name)
		t.Run(name, func(t *testing.T) {
			providerErr := normalizeProviderExitFailure(string(entry.Provider), entry.CommandResult(), nil, nil)
			if providerErr.Type != entry.ExpectedType || providerErr.Family != entry.ExpectedFamily {
				t.Fatalf("normalized failure = %#v, want type=%q family=%q", providerErr, entry.ExpectedType, entry.ExpectedFamily)
			}
			decision := WorkFailureDecisionFromProviderError(providerErr)
			if decision.Retryable != entry.Retryable || decision.Terminal == entry.Retryable || decision.TriggersThrottlePause != entry.TriggersThrottlePause {
				t.Fatalf("decision = %#v, want retryable=%t terminal=%t throttlePause=%t", decision, entry.Retryable, !entry.Retryable, entry.TriggersThrottlePause)
			}
		})
	}
}

func TestNewProviderError_AssignsDeterministicFamilyFromType(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name       string
		errorType  workerexecution.WorkFailureType
		wantFamily workerexecution.WorkFailureFamily
	}{
		{name: "AuthFailure_IsTerminal", errorType: workerexecution.WorkFailureTypeAuthFailure, wantFamily: workerexecution.WorkFailureFamilyTerminal},
		{name: "PermanentBadRequest_IsTerminal", errorType: workerexecution.WorkFailureTypePermanentBadRequest, wantFamily: workerexecution.WorkFailureFamilyTerminal},
		{name: "Throttled_IsThrottle", errorType: workerexecution.WorkFailureTypeThrottled, wantFamily: workerexecution.WorkFailureFamilyThrottle},
		{name: "InternalServerError_IsRetryable", errorType: workerexecution.WorkFailureTypeInternalServerError, wantFamily: workerexecution.WorkFailureFamilyRetryable},
		{name: "Timeout_IsRetryable", errorType: workerexecution.WorkFailureTypeTimeout, wantFamily: workerexecution.WorkFailureFamilyRetryable},
		{name: "Unknown_IsTerminal", errorType: workerexecution.WorkFailureTypeUnknown, wantFamily: workerexecution.WorkFailureFamilyTerminal},
		{name: "Misconfigured_IsTerminal", errorType: workerexecution.WorkFailureTypeMisconfigured, wantFamily: workerexecution.WorkFailureFamilyTerminal},
		{name: "MissingExecutable_IsTerminal", errorType: workerexecution.WorkFailureTypeMissingExecutable, wantFamily: workerexecution.WorkFailureFamilyTerminal},
		{name: "CommandLineTooLong_IsTerminal", errorType: workerexecution.WorkFailureTypeCommandLineTooLong, wantFamily: workerexecution.WorkFailureFamilyTerminal},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := NewProviderError(tc.errorType, "normalized failure", nil)
			if err.Type != tc.errorType {
				t.Fatalf("expected Type %q, got %q", tc.errorType, err.Type)
			}
			if err.Family != tc.wantFamily {
				t.Fatalf("expected Family %q, got %q", tc.wantFamily, err.Family)
			}
		})
	}
}

func TestNewProviderErrorFromResult_DerivesPolicyFromCanonicalReason(t *testing.T) {
	t.Parallel()
	result := ProviderFailureResult{
		Reason:  workerexecution.WorkFailureTypeThrottled,
		Message: "request capacity exceeded",
	}

	providerErr := NewProviderErrorFromResult(result, nil)
	if providerErr.Type != result.Reason || providerErr.Message != result.Message {
		t.Fatalf("NewProviderErrorFromResult() = %#v, want canonical reason and message", providerErr)
	}
	if providerErr.Family != workerexecution.WorkFailureFamilyThrottle {
		t.Fatalf("Family = %q, want %q", providerErr.Family, workerexecution.WorkFailureFamilyThrottle)
	}
}

func TestParseClaudeProviderFailure_CredentialFieldValuesNeverPassThrough(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
	t.Parallel()
	testCases := []struct {
		name   string
		stderr string
	}{
		{name: "AuthorizationWhitespaceProse", stderr: "Invalid request: authorization customer-private-value is invalid"},
		{name: "StructuredAuthorizationWhitespaceProse", stderr: `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Replace authorization customer-private-value"}}`},
		{name: "PrefixedAuthTokenWhitespaceProse", stderr: "Invalid request: x-auth-token customer-private-value is invalid"},
		{name: "AccessTokenWhitespaceProse", stderr: "Invalid request: access-token customer-private-value is invalid"},
		{name: "StructuredClientSecretWhitespaceProse", stderr: `API Error: 400 {"type":"error","error":{"type":"invalid_request_error","message":"Replace client-secret customer-private-value"}}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := CommandResult{ExitCode: 4, Stderr: []byte(tc.stderr)}
			assertClaudeFailureAndPolicy(t, result, claudeFailureExpectation{
				reason:    workerexecution.WorkFailureTypePermanentBadRequest,
				family:    workerexecution.WorkFailureFamilyTerminal,
				message:   claudeBadRequestFailureMessage,
				terminal:  true,
				retryable: false,
			})
			if parsed := ParseClaudeProviderFailure(result); strings.Contains(parsed.Message, "customer-private-value") {
				t.Fatalf("message %q must not contain the credential value", parsed.Message)
			}
		})
	}
}
