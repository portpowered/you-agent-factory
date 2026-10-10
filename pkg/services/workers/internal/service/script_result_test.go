package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	executeservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/service"
)

type scriptEnvelopeParser struct {
	factorydefinitions.DecisionEnvelopeService
	calls  int
	result workers.WorkResult
}

func (parser *scriptEnvelopeParser) WorkResultFromDecisionEnvelopeJSONOrFailed(_, _, raw string) workers.WorkResult {
	parser.calls++
	if raw != "script stdout" {
		panic("parser did not receive untouched script stdout")
	}
	return parser.result
}

func TestExecuteScriptEnvelopeUsesInjectedParserOnce(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		outcome workers.WorkOutcome
		want    workers.ExecutionOutcome
		reason  string
	}{
		{"accepted", workers.OutcomeAccepted, workers.ExecutionOutcomeAccepted, ""},
		{"rejected", workers.OutcomeRejected, workers.ExecutionOutcomeRejected, ""},
		{"declared failure", workers.OutcomeFailed, workers.ExecutionOutcomeFailed, "required read unavailable"},
		{"failed feedback", workers.OutcomeFailed, workers.ExecutionOutcomeFailed, ""},
		{"malformed", workers.OutcomeFailed, workers.ExecutionOutcomeFailed, "invalid decision envelope"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parser := &scriptEnvelopeParser{result: workers.WorkResult{
				Outcome: tc.outcome, Feedback: "shape reason", Output: `{"measurements":[{"name":"zero","value":0,"source":"fixture"}]}`,
				Error: tc.reason,
			}}
			service := scriptEnvelopeService(t, &stubRunner{content: "script stdout"}, parser)
			request := validExecuteRequest("dispatch", "attempt")
			request.Target.Output.Format = factorydefinitions.DecisionEnvelopeOutcomeFormat
			result, err := service.Execute(context.Background(), request)
			if err != nil || result.Outcome != tc.want || parser.calls != 1 {
				t.Fatalf("result=%#v err=%v parser calls=%d", result, err, parser.calls)
			}
			if result.Output.Feedback != "shape reason" || len(result.Output.Primary) != 1 ||
				!strings.Contains(result.Output.Primary[0].Text, `"value":0`) {
				t.Fatalf("lost output/feedback: %#v", result.Output)
			}
			reason := tc.reason
			if tc.outcome == workers.OutcomeFailed && reason == "" {
				reason = "shape reason"
			}
			if reason != "" && (result.Failure == nil || result.Failure.Message != reason || result.Failure.RetryHint) {
				t.Fatalf("failure=%#v, want exact terminal reason %q", result.Failure, reason)
			}
		})
	}
}

func TestExecuteScriptEnvelopeDoesNotParseProcessFailuresOrRawScripts(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		format string
		runErr error
		want   workers.ExecutionOutcome
	}{
		{"raw", "", nil, workers.ExecutionOutcomeAccepted},
		{"nonzero", "decision-envelope", errors.New("exit status 1"), workers.ExecutionOutcomeFailed},
		{"timeout", "decision-envelope", context.DeadlineExceeded, workers.ExecutionOutcomeFailed},
		{"cancelled", "decision-envelope", context.Canceled, workers.ExecutionOutcomeFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			parser := &scriptEnvelopeParser{}
			runner := &stubRunner{execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
				return workers.RunnerExecutionResult{Content: "script stdout"}, tc.runErr
			}}
			service := scriptEnvelopeService(t, runner, parser)
			request := validExecuteRequest("dispatch", "attempt")
			request.Target.Output.Format = tc.format
			result, err := service.Execute(context.Background(), request)
			if err != nil || result.Outcome != tc.want || parser.calls != 0 {
				t.Fatalf("result=%#v err=%v parser calls=%d", result, err, parser.calls)
			}
			if tc.runErr == nil && result.Output.Primary[0].Text != "script stdout" {
				t.Fatalf("raw output = %#v", result.Output)
			}
		})
	}
}

func scriptEnvelopeService(t *testing.T, runner *stubRunner, parser factorydefinitions.DecisionEnvelopeService) *executeservice.Service {
	t.Helper()
	service, err := executeservice.NewWithProviderOverride(
		&staticRunners{runner: runner}, nil, nil, logging.NoopLogger{}, func() time.Time { return time.Unix(10, 0) }, platformclock.Real{},
		nil, nil, nil, nil, nil, parser,
	)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
