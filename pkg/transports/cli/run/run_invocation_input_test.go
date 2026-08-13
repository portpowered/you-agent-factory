package run

import (
	"bytes"
	"context"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clihttp"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"io"
	"reflect"
	"strings"
	"testing"
)

func TestResolveFactoryInvocationRequest_NamedFactoryDirPositionalText(t *testing.T) {
	text := "hi there"

	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir:                      "/tmp/builtin-tts",
		NamedFactoryName:         "@you/tts",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for named factory positional text")
	}
	if got := extractInvocationText(t, request); got != text {
		t.Fatalf("invocation text = %q, want %q", got, text)
	}
}

func TestResolveFactoryInvocationRequest_PositionalText(t *testing.T) {
	text := "Fix the lint issues"

	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath:        "/tmp/factory.json",
		InvocationPositionalText: &text,
		StdinIsTTY:               func() bool { return true },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for positional text")
	}
	if got := extractInvocationText(t, request); got != text {
		t.Fatalf("invocation text = %q, want %q", got, text)
	}
}

func TestResolveFactoryInvocationRequest_StdinText(t *testing.T) {
	prepared := preparedTextInvocationInput(work.InputSourceStdinText, "from stdin")
	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		FactoryConfigPath:       "/tmp/factory.json",
		PreparedInvocationInput: &prepared,
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for stdin text")
	}
	if got := extractInvocationText(t, request); got != "from stdin" {
		t.Fatalf("invocation text = %q, want stdin text", got)
	}
}

func TestResolveFactoryInvocationRequest_NamedFactoryStdinText(t *testing.T) {
	stdinText := "hi from stdin"

	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir:                 "/tmp/builtin-tts",
		NamedFactoryName:    "@you/tts",
		InvocationStdinText: &stdinText,
		StdinIsTTY:          func() bool { return true },
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for named factory stdin text")
	}
	if got := extractInvocationText(t, request); got != stdinText {
		t.Fatalf("invocation text = %q, want %q", got, stdinText)
	}
}

func TestResolveFactoryInvocationRequest_UsesNormalizedSignatureArgs(t *testing.T) {
	request, invocationMode, err := resolveFactoryInvocationRequest(RunConfig{
		Dir: "/tmp/signature-factory",
		InvocationNormalizedArguments: &work.NormalizedArguments{
			Arguments: map[string]work.NormalizedArgument{
				"input": {Values: []string{"draft"}},
				"mode":  {Values: []string{"fast", "review"}},
			},
		},
	})
	if err != nil {
		t.Fatalf("resolveFactoryInvocationRequest: %v", err)
	}
	if !invocationMode {
		t.Fatal("expected invocation mode for normalized signature args")
	}
	if request == nil || request.Args == nil {
		t.Fatalf("request = %#v, want args request", request)
	}
	if got := (*request.Args)["input"]; got != "draft" {
		t.Fatalf("args[input] = %#v, want %q", got, "draft")
	}
	values, ok := (*request.Args)["mode"].([]string)
	if !ok {
		t.Fatalf("args[mode] = %#v, want []string", (*request.Args)["mode"])
	}
	if len(values) != 2 || values[0] != "fast" || values[1] != "review" {
		t.Fatalf("args[mode] = %#v, want [fast review]", values)
	}
}

func TestRunFactoryInvocationCarriesPreparedCanonicalInputWithoutPlainArgs(t *testing.T) {
	prepared := work.PreparedInvocationInput{
		NormalizedArguments: &work.NormalizedArguments{Arguments: map[string]work.NormalizedArgument{
			"input": {
				Values:  []string{"draft"},
				Sources: []work.ArgumentSource{{Kind: work.ArgumentSourceKindPositional, Name: "1"}},
			},
		}},
	}
	apiRequest := invocationRequestFromNormalizedArguments(*prepared.NormalizedArguments)
	var captured factorysessions.InvocationRequest
	operation := testInvocationOperation{invokeFactory: func(
		_ context.Context,
		_ factorysessions.InvocationTarget,
		request factorysessions.InvocationRequest,
		_ func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		captured = request
		return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
			Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "done",
			}},
		}}, nil
	}}
	var output bytes.Buffer
	err := runFactoryInvocation(
		context.Background(),
		RunConfig{PreparedInvocationInput: &prepared, Output: &output},
		factorysessions.InvocationTarget{},
		*apiRequest,
		operation,
		testResponsePresentation(),
		nil,
	)
	if err != nil {
		t.Fatalf("runFactoryInvocation: %v", err)
	}
	if captured.Args != nil || captured.ContentProvided {
		t.Fatalf("execution request retained plain API carriers: %#v", captured)
	}
	if captured.PreparedInvocationInput == nil ||
		!reflect.DeepEqual(
			captured.PreparedInvocationInput.NormalizedArguments.Arguments,
			prepared.NormalizedArguments.Arguments,
		) {
		t.Fatalf("prepared execution input = %#v, want detached canonical input", captured.PreparedInvocationInput)
	}
	prepared.NormalizedArguments.Arguments["input"] = work.NormalizedArgument{Values: []string{"mutated"}}
	if got := captured.PreparedInvocationInput.NormalizedArguments.Arguments["input"].Values[0]; got != "draft" {
		t.Fatalf("captured canonical input aliased caller mutation: %q", got)
	}
}

func TestRunFactoryInvocationCarriesPreparedCompatibilityInputWithoutAPIContent(t *testing.T) {
	prepared := preparedTextInvocationInput(work.InputSourcePositionalText, "legacy input")
	apiRequest := invocationRequestFromResolvedInput(*prepared.ResolvedInput)
	var captured factorysessions.InvocationRequest
	operation := testInvocationOperation{invokeFactory: func(
		_ context.Context,
		_ factorysessions.InvocationTarget,
		request factorysessions.InvocationRequest,
		_ func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		captured = request
		return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
			Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "done",
			}},
		}}, nil
	}}
	var output bytes.Buffer
	err := runFactoryInvocation(
		context.Background(),
		RunConfig{PreparedInvocationInput: &prepared, Output: &output},
		factorysessions.InvocationTarget{},
		*apiRequest,
		operation,
		testResponsePresentation(),
		nil,
	)
	if err != nil {
		t.Fatalf("runFactoryInvocation: %v", err)
	}
	if captured.Args != nil || captured.ContentProvided || len(captured.Content) != 0 {
		t.Fatalf("execution request retained API compatibility carriers: %#v", captured)
	}
	if captured.PreparedInvocationInput == nil ||
		captured.PreparedInvocationInput.ResolvedInput == nil ||
		captured.PreparedInvocationInput.ResolvedInput.Text != "legacy input" {
		t.Fatalf("prepared execution input = %#v, want compatibility result", captured.PreparedInvocationInput)
	}
}

// TestRunFactoryInvocationWritesTerminalRecordAndPreservesCleanupErrorAfterResult
// proves that once InvokeFactory has determined a terminal result, a non-nil
// error returned alongside it (a post-result runtime teardown/cleanup
// failure, not a pre-terminal invocation failure) still causes the public
// terminal NDJSON record to be written for that determined outcome AND still
// causes runFactoryInvocation to report failure. Regression for both
// directions of the "invocation_result record count = 0" defect class: a
// cleanup failure must not silently swallow the terminal record (it is
// written), and it must not silently swallow itself either (the CLI must not
// report success).
func TestRunFactoryInvocationWritesTerminalRecordAndPreservesCleanupErrorAfterResult(t *testing.T) {
	cleanupErr := errors.New("close factory session: deterministic teardown failure")
	var output bytes.Buffer
	operation := testInvocationOperation{invokeFactory: func(
		context.Context,
		factorysessions.InvocationTarget,
		factorysessions.InvocationRequest,
		func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
			RequestID: "request-cleanup",
			Status:    interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "completed despite cleanup failure",
			}},
		}}, cleanupErr
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	err := runFactoryInvocation(
		context.Background(), cfg, invocationTarget(cfg, nil),
		factoryapi.InvocationRequest{}, operation, testResponsePresentation(), nil,
	)
	if err == nil || !errors.Is(err, cleanupErr) {
		t.Fatalf("runFactoryInvocation error = %v, want it to preserve the post-result cleanup failure %v", err, cleanupErr)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 {
		t.Fatalf("NDJSON output = %d lines, want exactly one terminal invocation_result record:\n%s", len(lines), output.String())
	}
	if !strings.Contains(lines[0], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal record = %q, want an invocation_result record", lines[0])
	}
	if !strings.Contains(lines[0], `"status":"`+string(interfaces.InvocationTerminalStatusCompleted)+`"`) {
		t.Fatalf("terminal record = %q, want the invocation's own determined completed status", lines[0])
	}
}

func TestRunFactoryInvocationPrefersCanonicalOutcomeOverContextCancellation(t *testing.T) {
	var output bytes.Buffer
	operation := testInvocationOperation{invokeFactory: func(
		context.Context,
		factorysessions.InvocationTarget,
		factorysessions.InvocationRequest,
		func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
			Status:    interfaces.InvocationTerminalStatusCanceled,
			ErrorCode: string(interfaces.InvocationErrorCodeCanceled),
			Message:   "invocation was canceled while waiting for primary result",
		}}, context.Canceled
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	err := runFactoryInvocation(
		context.Background(), cfg, invocationTarget(cfg, nil),
		factoryapi.InvocationRequest{}, operation, testResponsePresentation(), nil,
	)
	if err == nil || !strings.Contains(err.Error(), string(interfaces.InvocationErrorCodeCanceled)) {
		t.Fatalf("runFactoryInvocation error = %v, want canonical cancellation", err)
	}
	if strings.Contains(err.Error(), InvocationErrorCodeCancelled) {
		t.Fatalf("runFactoryInvocation error = %v, want no derived RUN cancellation wrapper", err)
	}
	if !strings.Contains(output.String(), `"status":"`+string(interfaces.InvocationTerminalStatusCanceled)+`"`) {
		t.Fatalf("output = %q, want canceled terminal record", output.String())
	}
}

func TestRunFactoryInvocationUsesRunCancellationWhenOutcomeIsUndetermined(t *testing.T) {
	var output bytes.Buffer
	operation := testInvocationOperation{invokeFactory: func(
		context.Context,
		factorysessions.InvocationTarget,
		factorysessions.InvocationRequest,
		func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		return factorysessions.FactoryInvocationOutcome{}, context.Canceled
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	err := runFactoryInvocation(
		context.Background(), cfg, invocationTarget(cfg, nil),
		factoryapi.InvocationRequest{}, operation, testResponsePresentation(), nil,
	)
	if err == nil || !strings.Contains(err.Error(), InvocationErrorCodeCancelled) {
		t.Fatalf("runFactoryInvocation error = %v, want derived RUN cancellation fallback", err)
	}
	if output.Len() != 0 {
		t.Fatalf("output = %q, want no terminal record for undetermined outcome", output.String())
	}
}

// TestRunFactoryInvocationRejectsUndeterminedResultWithNilError proves that
// runFactoryInvocation cannot silently report success when InvokeFactory
// returns neither a determined terminal result (empty Status) nor an error.
// MapInvocationFailure(nil) returns nil by design (a genuinely absent error
// means success), so without an explicit invariant check this zero-value
// outcome would flow straight through as a successful CLI exit with no
// terminal record written at all -- the exact "hidden false success" defect
// this guards against.

func TestRunFactoryInvocationRejectsUndeterminedResultWithNilError(t *testing.T) {
	var output bytes.Buffer
	operation := testInvocationOperation{invokeFactory: func(
		context.Context,
		factorysessions.InvocationTarget,
		factorysessions.InvocationRequest,
		func([]interfaces.FactoryEvent),
	) (factorysessions.FactoryInvocationOutcome, error) {
		return factorysessions.FactoryInvocationOutcome{}, nil
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	err := runFactoryInvocation(
		context.Background(), cfg, invocationTarget(cfg, nil),
		factoryapi.InvocationRequest{}, operation, testResponsePresentation(), nil,
	)
	if err == nil {
		t.Fatal("runFactoryInvocation error = nil, want a non-nil error when no terminal result was ever determined")
	}
	if output.Len() != 0 {
		t.Fatalf("NDJSON output = %q, want no output written for an undetermined outcome", output.String())
	}
}

func TestResolveFactoryInvocationRequest_NamedFactoryRejectsConflictingSources(t *testing.T) {
	err := scriptedInvocationConflictError()
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("error = %q, want stable conflict code", err.Error())
	}
}

func TestResolveFactoryInvocationRequest_RejectsWhitespaceOnlyPositional(t *testing.T) {
	err := MapInvocationInputError(&work.InputError{Code: work.InputErrorCodeEmpty, Message: "invocation input is empty", Source: work.InputSourcePositionalText})
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_EMPTY") {
		t.Fatalf("error = %q, want stable empty code", err.Error())
	}
}

func TestResolveFactoryInvocationRequest_RejectsConflictingSources(t *testing.T) {
	err := scriptedInvocationConflictError()
	if !strings.Contains(err.Error(), "INVOCATION_INPUT_SOURCE_CONFLICT") {
		t.Fatalf("error = %q, want stable conflict code", err.Error())
	}
}

func TestResolveFactoryInvocationRequest_ConflictLogsAndCountsSourceConflict(t *testing.T) {
	core, observedLogs := observer.New(zap.InfoLevel)
	recorder := &capturingInvocationMetricsRecorder{}
	inputErr := &work.InputError{
		Code: work.InputErrorCodeSourceConflict, Message: "invocation input sources conflict: positional_text, stdin_text",
		ConflictingSources: []work.InputSourceLabel{work.InputSourcePositionalText, work.InputSourceStdinText},
	}
	recordCLIInvocationFailure(RunConfig{
		Logger:                    zap.New(core),
		InvocationMetricsRecorder: recorder,
	}, inputErr)
	entries := observedLogs.FilterMessage("factory invocation input resolution failed").All()
	if len(entries) != 1 {
		t.Fatalf("conflict log count = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if got := fields["failure_class"]; got != "source_conflict" {
		t.Fatalf("failure_class = %#v, want source_conflict", got)
	}
	if got := fields["error_code"]; got != "INVOCATION_INPUT_SOURCE_CONFLICT" {
		t.Fatalf("error_code = %#v, want INVOCATION_INPUT_SOURCE_CONFLICT", got)
	}
	if _, ok := fields["conflicting_sources"]; !ok {
		t.Fatal("expected conflicting_sources field in conflict log")
	}

	recorder.assertContainsMetricNames(t, "invocation.failure", "invocation.source_conflict")
}

type capturingInvocationMetricsRecorder struct {
	metrics []factorysessions.InvocationMetric
}

func (r *capturingInvocationMetricsRecorder) RecordInvocationMetric(metric factorysessions.InvocationMetric) {
	r.metrics = append(r.metrics, metric)
}

func (r *capturingInvocationMetricsRecorder) assertContainsMetricNames(t *testing.T, want ...string) {
	t.Helper()

	if len(r.metrics) != len(want) {
		t.Fatalf("metric count = %d, want %d (%#v)", len(r.metrics), len(want), r.metrics)
	}
	got := make(map[string]int, len(r.metrics))
	for _, metric := range r.metrics {
		got[metric.Name]++
	}
	for _, name := range want {
		if got[name] == 0 {
			t.Fatalf("metrics = %#v, want to include %q", r.metrics, name)
		}
	}
}

func TestRemoteInvocationClientResultValidation(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{}}.GetFactorySessionResult(nil, RemoteInvocationResultRequest{})
	if err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("nil context error = %v, want required-context result error", err)
	}

	_, err = remoteInvocationClient{}.GetFactorySessionResult(context.Background(), RemoteInvocationResultRequest{})
	if err == nil || !strings.Contains(err.Error(), "CLI HTTP protocol is required") {
		t.Fatalf("nil protocol error = %v, want required-protocol result error", err)
	}

	stub := &remoteProtocolStub{}
	_, err = remoteInvocationClient{transport: stub}.GetFactorySessionResult(context.Background(), RemoteInvocationResultRequest{Server: "https://selected.test"})
	if err == nil || !strings.Contains(err.Error(), RemoteDurableResponseInvalidCode) || stub.called {
		t.Fatalf("missing result identity error = %v/called=%t, want response error without HTTP", err, stub.called)
	}

	stub = &remoteProtocolStub{}
	_, err = remoteInvocationClient{transport: stub}.GetFactorySessionResult(context.Background(), RemoteInvocationResultRequest{
		Server: "http://[::1", SessionID: "dur-sess-invalid-endpoint",
	})
	if err == nil || !strings.Contains(err.Error(), RemoteDurableResultCode) || stub.called {
		t.Fatalf("invalid result endpoint error = %v/called=%t, want result error without HTTP", err, stub.called)
	}
}

func TestRemoteInvocationClientResultWithoutHTTPResponse(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{response: clihttp.Response{}}}.GetFactorySessionResult(
		context.Background(), RemoteInvocationResultRequest{Server: "https://selected.test", SessionID: "dur-sess-result"},
	)
	if err == nil || !strings.Contains(err.Error(), "HTTP response is unavailable") {
		t.Fatalf("error = %v, want missing result response", err)
	}
}

func TestRemoteInvocationClientTransportErrors(t *testing.T) {
	_, err := remoteInvocationClient{transport: &remoteProtocolStub{err: errors.New("dial failed")}}.StartFactorySession(
		context.Background(), RemoteInvocationRequest{Server: "https://selected.test"},
	)
	assertInvocationErrorCode(t, err, RemoteDurableStartCode, "dial failed")

	_, err = remoteInvocationClient{transport: &remoteProtocolStub{err: errors.New("result dial failed")}}.GetFactorySessionResult(
		context.Background(), RemoteInvocationResultRequest{Server: "https://selected.test", SessionID: "dur-sess-result"},
	)
	assertInvocationErrorCode(t, err, RemoteDurableResultCode, "result dial failed")
}

func assertInvocationErrorCode(t *testing.T, err error, code, detail string) {
	t.Helper()
	var invocationErr *InvocationError
	if !errors.As(err, &invocationErr) || invocationErr.Code != code || !strings.Contains(err.Error(), detail) {
		t.Fatalf("error = %v, want %s with %s", err, code, detail)
	}
}

func TestRemoteDurableReasonClassificationCoversControlReasons(t *testing.T) {
	for _, test := range []struct {
		reason string
		code   string
		status interfaces.InvocationTerminalStatus
	}{
		{reason: "NEEDS_HUMAN", code: "INVOCATION_NEEDS_HUMAN", status: interfaces.InvocationTerminalStatusFailed},
		{reason: "approval_required", code: "INVOCATION_NEEDS_HUMAN", status: interfaces.InvocationTerminalStatusFailed},
		{reason: "PAUSED_BY_OPERATOR", code: "INVOCATION_PAUSED", status: interfaces.InvocationTerminalStatusFailed},
		{reason: "TIMEOUT", code: "INVOCATION_TIMED_OUT", status: interfaces.InvocationTerminalStatusTimedOut},
		{reason: "CANCELLED", code: "INVOCATION_CANCELED", status: interfaces.InvocationTerminalStatusCanceled},
		{reason: "TERMINATED", code: "INVOCATION_INTERRUPTED", status: interfaces.InvocationTerminalStatusFailed},
	} {
		t.Run(test.reason, func(t *testing.T) {
			status, code, _, ok := remoteDurableReasonClassification(test.reason)
			if !ok || code != test.code || status != test.status {
				t.Fatalf("classification = (%s, %q, %t), want (%s, %q, true)", status, code, ok, test.status, test.code)
			}
		})
	}
	if _, _, _, ok := remoteDurableReasonClassification("UNKNOWN"); ok {
		t.Fatal("unknown durable reason classified as terminal")
	}
}

func TestRemoteDurableSourceVariants(t *testing.T) {
	workflow := "review-workflow"
	got, _, err := remoteDurableSourceFromRunConfig(RunConfig{Workflow: workflow})
	if err != nil || got.Kind != factoryapi.FactorySessionExecutionSourceKindWorkflowName || got.WorkflowName == nil || *got.WorkflowName != workflow {
		t.Fatalf("workflow source = %#v/%v, want workflow name", got, err)
	}

	workflowFile := "workflow.mjs"
	got, _, err = remoteDurableSourceFromRunConfig(RunConfig{FactoryConfigPath: workflowFile})
	if err != nil || got.Kind != factoryapi.FactorySessionExecutionSourceKindWorkflowFile || got.WorkflowFile == nil || *got.WorkflowFile != workflowFile {
		t.Fatalf("workflow file source = %#v/%v, want workflow file", got, err)
	}
}

func TestRemoteDurableSourceRejectsUnrepresentableTargets(t *testing.T) {
	_, _, err := remoteDurableSourceFromRunConfig(RunConfig{})
	if err == nil || !strings.Contains(err.Error(), RemoteDurableRequestInvalidCode) {
		t.Fatalf("missing target error = %v, want %s", err, RemoteDurableRequestInvalidCode)
	}

	_, _, err = remoteDurableSourceFromRunConfig(RunConfig{
		NamedFactoryName: "@you/research",
		Dir:              "factory",
		LoadFactoryConfigFile: func(string) (*interfaces.FactoryConfig, error) {
			return nil, errors.New("config unavailable")
		},
	})
	if err == nil || !strings.Contains(err.Error(), RemoteDurableRequestInvalidCode) || !strings.Contains(err.Error(), "config unavailable") {
		t.Fatalf("named policy load error = %v, want stable remote request error", err)
	}

	_, _, err = remoteDurableSourceFromRunConfig(RunConfig{FactoryConfigPath: "factory.json"})
	if err == nil || !strings.Contains(err.Error(), "config loader") {
		t.Fatalf("missing inline loader error = %v, want config loader diagnostic", err)
	}
}

func TestRemoteInvocationWaitAndOutputModesPreserveTerminalBoundary(t *testing.T) {
	newOperation := func(result factoryapi.FactorySessionResult) remoteInvocationResultOperationFunc {
		return remoteInvocationResultOperationFunc{result: func(context.Context, RemoteInvocationResultRequest) (factoryapi.FactorySessionResult, error) {
			return result, nil
		}}
	}
	baseConfig := func(output io.Writer) RunConfig {
		return RunConfig{
			Dir:                     "factory",
			NamedFactoryName:        "@you/research",
			PreparedInvocationInput: preparedRemoteArguments("boundary input"),
			Output:                  output,
		}
	}

	statusSucceeded := factoryapi.FactorySessionDurableLifecycleStatusSucceeded
	output, err := runRemoteInvocationWithOperation(t, baseConfig(io.Discard), newOperation(factoryapi.FactorySessionResult{
		SessionId:     "other-session",
		ResultStatus:  factoryapi.FactorySessionResultStatusFinal,
		SessionStatus: &statusSucceeded,
	}))
	if err == nil || !strings.Contains(err.Error(), RemoteDurableResponseInvalidCode) || output != "" {
		t.Fatalf("identity mismatch = %v/output=%q, want stable response error and no output", err, output)
	}

	output, err = runRemoteInvocationWithOperation(t, baseConfig(io.Discard), newOperation(factoryapi.FactorySessionResult{
		SessionId:     "dur-sess-boundary",
		ResultStatus:  factoryapi.FactorySessionResultStatusNotReady,
		SessionStatus: &statusSucceeded,
	}))
	if err == nil || !strings.Contains(err.Error(), "INVOCATION_PRIMARY_RESULT_UNRESOLVED") || output != "" {
		t.Fatalf("non-terminal result = %v/output=%q, want unresolved-result error and no output", err, output)
	}

	statusFailed := factoryapi.FactorySessionDurableLifecycleStatusFailed
	var streamOutput bytes.Buffer
	streamConfig := baseConfig(&streamOutput)
	streamConfig.JSONOutput = true
	streamConfig.InvocationOutputMode = InvocationOutputResponseStream
	output, err = runRemoteInvocationWithOperation(t, streamConfig, newResponseStreamFailureOperation(factoryapi.FactorySessionResult{
		SessionId:     "dur-sess-boundary",
		ResultStatus:  factoryapi.FactorySessionResultStatusFailedWithPartial,
		SessionStatus: &statusFailed,
		FailureDetail: &factoryapi.FailureDetail{Message: "remote terminal failure"},
	}), testResponsePresentation())
	if err == nil || !strings.Contains(err.Error(), "INVOCATION_RUNTIME_FAILURE") || output == "" || !strings.Contains(output, `"recordType":"invocation_result"`) {
		t.Fatalf("JSON response stream = %v/output=%q, want terminal record and failure", err, output)
	}

	var humanOutput bytes.Buffer
	humanConfig := baseConfig(&humanOutput)
	humanConfig.InvocationOutputMode = InvocationOutputResponseStream
	output, err = runRemoteInvocationWithOperation(t, humanConfig, newResponseStreamFailureOperation(factoryapi.FactorySessionResult{
		SessionId:     "dur-sess-boundary",
		ResultStatus:  factoryapi.FactorySessionResultStatusFailedWithPartial,
		SessionStatus: &statusFailed,
		FailureDetail: &factoryapi.FailureDetail{Message: "remote terminal failure"},
	}), testResponsePresentation())
	if err == nil || !strings.Contains(err.Error(), "INVOCATION_RUNTIME_FAILURE") || output != humanOutput.String() || !strings.Contains(output, "--- invocation outcome ---") {
		t.Fatalf("human response stream = %v/output=%q, want human terminal outcome", err, output)
	}
}

func newResponseStreamFailureOperation(result factoryapi.FactorySessionResult) *scriptedRemoteResponseOperation {
	result.SessionId = "dur-sess-remote-stream"
	return &scriptedRemoteResponseOperation{
		streams: []scriptedRemoteEventStream{{}},
		result:  result,
	}
}

func TestRemoteInvocationWaitCancellationIsClassified(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	op := remoteInvocationResultOperationFunc{result: func(context.Context, RemoteInvocationResultRequest) (factoryapi.FactorySessionResult, error) {
		cancel()
		running := factoryapi.FactorySessionDurableLifecycleStatusRunning
		retryable := true
		return factoryapi.FactorySessionResult{
			SessionId:     "dur-sess-boundary",
			ResultStatus:  factoryapi.FactorySessionResultStatusNotReady,
			SessionStatus: &running,
			Availability:  &factoryapi.FactorySessionResultAvailabilityDetail{Retryable: &retryable},
		}, nil
	}}
	output, err := runRemoteInvocationWithContext(t, ctx, RunConfig{
		Dir:                     "factory",
		NamedFactoryName:        "@you/research",
		PreparedInvocationInput: preparedRemoteArguments("cancel wait"),
		Output:                  io.Discard,
	}, op)
	if err == nil || !strings.Contains(err.Error(), RemoteDurableResultCode) || !errors.Is(err, context.Canceled) || output != "" {
		t.Fatalf("canceled wait = %v/output=%q, want durable result cancellation and no output", err, output)
	}
}

type remoteInvocationResultOperationFunc struct {
	result func(context.Context, RemoteInvocationResultRequest) (factoryapi.FactorySessionResult, error)
}

func (fn remoteInvocationResultOperationFunc) StartFactorySession(context.Context, RemoteInvocationRequest) (factoryapi.FactorySessionExecutionResponse, error) {
	return factoryapi.FactorySessionExecutionResponse{
		SessionId: "dur-sess-boundary",
		Status:    factoryapi.FactorySessionDurableLifecycleStatusQueued,
	}, nil
}

func (fn remoteInvocationResultOperationFunc) GetFactorySessionResult(ctx context.Context, request RemoteInvocationResultRequest) (factoryapi.FactorySessionResult, error) {
	return fn.result(ctx, request)
}

func runRemoteInvocationWithOperation(
	t *testing.T,
	cfg RunConfig,
	operation RemoteInvocationOperation,
	presentations ...factoryvisualization.ResponsePresentation,
) (string, error) {
	t.Helper()
	err := RunRemoteInvocation(context.Background(), cfg, "http://selected.test", operation, presentations...)
	if output, ok := cfg.Output.(*bytes.Buffer); ok {
		return output.String(), err
	}
	return "", err
}

func runRemoteInvocationWithContext(t *testing.T, ctx context.Context, cfg RunConfig, operation remoteInvocationResultOperationFunc) (string, error) {
	t.Helper()
	err := RunRemoteInvocation(ctx, cfg, "http://selected.test", operation)
	if output, ok := cfg.Output.(*bytes.Buffer); ok {
		return output.String(), err
	}
	return "", err
}
