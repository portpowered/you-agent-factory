package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestExecuteNamesDeclaredFailureAndPreservesOutput(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, feedback, want string
		skip                 bool
	}{
		{"feedback", "admission was not confirmed", "admission was not confirmed", false},
		{"skipped record", "admission was not confirmed", "admission was not confirmed", true},
		{"skipped empty feedback", "  ", "worker explicitly returned a FAILED decision", true},
		{"private source feedback", "admission private-source blocked", "admission <redacted> blocked", false},
		{"private environment feedback", "admission private-env blocked", "admission <redacted> blocked", false},
		{"empty feedback", "  ", "worker explicitly returned a FAILED decision", false},
		{"private feedback", "admission private-value blocked", "admission <redacted> blocked", false},
		{"bounded feedback", strings.Repeat("界", 600), strings.Repeat("界", 512), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := &stubRunner{execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
				diagnostics := &workers.WorkDiagnostics{Metadata: map[string]string{}}
				if tc.skip {
					diagnostics.Metadata["inspection_records_skipped"] = "1"
				}
				return workers.RunnerExecutionResult{Outcome: workers.OutcomeFailed, Content: "preserved output", Feedback: tc.feedback, Diagnostics: diagnostics}, nil
			}}
			request := validExecuteRequest("declared-dispatch", "declared-attempt")
			request.Input.Invocation.Arguments = map[string]work.InvocationArgument{
				"secret": {Sensitive: true, Values: []string{"private-value"}},
				"source": {Sources: []work.InvocationArgumentSource{{Redact: true}}, Values: []string{"private-source"}},
			}
			request.Target.Environment.ProcessEnvironment = []string{"OPENAI_API_KEY=private-env"}
			result, err := mustExecuteService(t, runner, nil).Execute(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			assertDeclaredFailure(t, result, tc.want)
			wantFeedback := tc.feedback
			if strings.HasPrefix(tc.name, "private") {
				wantFeedback = tc.want
			}
			if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "preserved output" || result.Output.Feedback != wantFeedback {
				t.Fatalf("output = %#v", result.Output)
			}
			if tc.skip && result.Diagnostics.Metadata["inspection_records_skipped"] != "1" {
				t.Fatal("lost record skip")
			}
		})
	}
}

func TestExecuteTypedFailurePrecedesDeclaredOutcome(t *testing.T) {
	t.Parallel()
	runner := &stubRunner{execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
		return workers.RunnerExecutionResult{Outcome: workers.OutcomeFailed, Feedback: "declared feedback"}, workers.NewProviderError(workers.WorkFailureTypeAuthFailure, "authentication failed", nil)
	}}
	result, err := mustExecuteService(t, runner, nil).Execute(t.Context(), validExecuteRequest("auth-dispatch", "auth-attempt"))
	if err != nil || result.Failure == nil || result.Failure.Type != workers.WorkFailureTypeAuthFailure || result.Failure.Message != "authentication failed" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestExecuteRedactsOverlappingPrivateValuesAcrossSources(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, sensitive, source, workstation, inherited string
	}{
		{"workstation prefix of inherited", "", "", "supervisor-", "supervisor-private-value"},
		{"argument prefix of workstation", "supervisor-", "", "supervisor-private-value", ""},
		{"source prefix of inherited", "", "supervisor-", "", "supervisor-private-value"},
		{"inherited prefix of argument", "supervisor-private-value", "", "", "supervisor-"},
		{"marker is also private", "", "", "supervisor-private-value", "redacted"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runner := &stubRunner{execute: func(context.Context, workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error) {
				return workers.RunnerExecutionResult{Outcome: workers.OutcomeFailed, Content: "preserved output", Feedback: "admission blocked by supervisor-private-value"}, nil
			}}
			request := validExecuteRequest("overlap-dispatch", "overlap-attempt")
			request.Input.Invocation.Arguments = map[string]work.InvocationArgument{
				"secret": {Sensitive: true, Values: []string{tc.sensitive}},
				"source": {Sources: []work.InvocationArgumentSource{{Redact: true}}, Values: []string{tc.source}},
			}
			request.Target.Environment.Vars = map[string]string{"SUPERVISOR_SECRET": tc.workstation}
			request.Target.Environment.ProcessEnvironment = []string{"OPENAI_API_KEY=" + tc.inherited}
			result, err := mustExecuteService(t, runner, nil).Execute(t.Context(), request)
			if err != nil {
				t.Fatal(err)
			}
			const want = "admission blocked by <redacted>"
			assertDeclaredFailure(t, result, want)
			if result.Failure.Message != want || result.Output.Feedback != want || len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "preserved output" {
				t.Fatalf("failure=%#v output=%#v", result.Failure, result.Output)
			}
		})
	}
}

func assertDeclaredFailure(t *testing.T, result workers.ExecuteResult, want string) {
	t.Helper()
	if result.Outcome != workers.ExecutionOutcomeFailed || result.Failure == nil || result.Failure.Type != workers.WorkFailureTypeWorkerDeclaredFailure || result.Failure.RetryHint {
		t.Fatalf("result = %#v", result)
	}
	if result.Failure.Detail.Reason != workers.WorkFailureTypeWorkerDeclaredFailure || result.Failure.Detail.Message != want {
		t.Fatalf("detail = %#v", result.Failure.Detail)
	}
}
