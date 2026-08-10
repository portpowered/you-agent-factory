package run

import (
	"bytes"
	"context"
	"errors"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
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
		_ factorysessions.FactoryEventConsumer,
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
		_ factorysessions.FactoryEventConsumer,
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
		factorysessions.FactoryEventConsumer,
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
		context.Background(), cfg, invocationTarget(cfg, nil, nil),
		factoryapi.InvocationRequest{}, operation, testResponsePresentation(),
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
		factorysessions.FactoryEventConsumer,
	) (factorysessions.FactoryInvocationOutcome, error) {
		return factorysessions.FactoryInvocationOutcome{}, nil
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	err := runFactoryInvocation(
		context.Background(), cfg, invocationTarget(cfg, nil, nil),
		factoryapi.InvocationRequest{}, operation, testResponsePresentation(),
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
