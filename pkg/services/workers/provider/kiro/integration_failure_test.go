package kiro

import (
	"context"
	"errors"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type integrationFailureCase struct {
	name        string
	result      workerprocess.CommandResult
	err         error
	wantKind    inference.FailureKind
	wantMessage string
	wantRetry   bool
	wantSession string
	rejected    []string
}

func TestIntegrationNormalizesKnownKiroFailures(t *testing.T) {
	t.Parallel()

	runIntegrationFailureCases(t, []integrationFailureCase{
		{
			name: "structured authentication",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"error":{"type":"authentication_error","message":"Bearer private-token"}}`),
			},
			wantKind:    inference.FailureAuthentication,
			wantMessage: kiroAuthFailureMessage,
			rejected:    []string{"Bearer", "private-token"},
		},
		{
			name: "structured invalid request",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr:   []byte(`{"status":422,"message":"private customer request"}`),
			},
			wantKind:    inference.FailureInvalidRequest,
			wantMessage: kiroBadRequestFailureMessage,
			rejected:    []string{"private customer request"},
		},
		{
			name: "structured throttle outranks text",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stdout:   []byte(`{"error":{"code":"ThrottlingException","message":"capacity secret"}}`),
				Stderr:   []byte("ERROR: authentication required"),
			},
			wantKind:    inference.FailureThrottled,
			wantMessage: kiroThrottleFailureMessage,
			wantRetry:   true,
			rejected:    []string{"authentication required", "capacity secret"},
		},
		{
			name: "temporary service failure",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr: []byte(
					`{"event":"session.created","session_id":"` + resumedSessionID + `"}` + "\n" +
						`{"type":"error","errorType":"ServiceUnavailableException","message":"host /tmp/private"}`,
				),
			},
			wantKind:    inference.FailureUnknown,
			wantMessage: kiroServerFailureMessage,
			wantRetry:   true,
			wantSession: resumedSessionID,
			rejected:    []string{"/tmp/private"},
		},
	})
}

func TestIntegrationNormalizesMalformedUnknownAndDeadlineFailures(t *testing.T) {
	t.Parallel()

	runIntegrationFailureCases(t, []integrationFailureCase{
		{
			name: "malformed structured record falls back to text",
			result: workerprocess.CommandResult{
				ExitCode: 1,
				Stderr: []byte(
					"ERROR: {\"type\":\"error\", malformed\nERROR: request timed out while waiting for Kiro",
				),
			},
			wantKind:    inference.FailureTimeout,
			wantMessage: TimeoutFailureMessage,
			wantRetry:   true,
			rejected:    []string{"malformed", "while waiting"},
		},
		{
			name: "unsafe unknown detail falls back to exit code",
			result: workerprocess.CommandResult{
				ExitCode: 17,
				Stderr:   []byte(`Error: failed reading C:\Users\alice\private\project`),
			},
			wantKind:    inference.FailureUnknown,
			wantMessage: "kiro-cli exited with code 17",
			rejected:    []string{`C:\Users\alice`, "private"},
		},
		{
			name:        "command deadline",
			err:         context.DeadlineExceeded,
			wantKind:    inference.FailureTimeout,
			wantMessage: TimeoutFailureMessage,
			wantRetry:   true,
		},
	})
}

func runIntegrationFailureCases(t *testing.T, testCases []integrationFailureCase) {
	t.Helper()
	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			runner := &recordingRunner{result: testCase.result, err: testCase.err}
			writer := &recordingWriter{}
			err := inference.ExecuteInvocation(
				context.Background(),
				NewIntegration(IntegrationDependencies{CommandRunner: runner}),
				failureInvocationRequest("inv-kiro-"+testCase.name),
				writer,
			)
			if err != nil {
				t.Fatalf("ExecuteInvocation() error = %v", err)
			}
			assertKiroFailureCompletion(t, writer, testCase)
		})
	}
}

func assertKiroFailureCompletion(
	t *testing.T,
	writer *recordingWriter,
	want integrationFailureCase,
) {
	t.Helper()
	if writer.closes != 1 || writer.completion.Response() != nil || len(writer.events) != 1 {
		t.Fatalf(
			"completion = %#v, closes = %d, events = %#v; want one failed close with one error event",
			writer.completion, writer.closes, writer.events,
		)
	}
	draft := writer.events[0].Draft()
	if draft.Kind != workerexecution.KindError || draft.Phase != workerexecution.PhaseFailed {
		t.Fatalf("failure event = %#v, want synthesized error.failed", draft)
	}
	failure := writer.completion.Failure()
	if failure == nil ||
		failure.Kind() != want.wantKind ||
		failure.Message() != want.wantMessage ||
		failure.Retryable() != want.wantRetry {
		t.Fatalf(
			"failure = %#v, want kind=%q message=%q retryable=%v",
			failure, want.wantKind, want.wantMessage, want.wantRetry,
		)
	}
	assertKiroFailureSession(t, failure.ProviderSession(), want.wantSession)
	for _, rejected := range want.rejected {
		if strings.Contains(failure.Message(), rejected) {
			t.Fatalf("failure message leaked %q: %q", rejected, failure.Message())
		}
	}
}

func assertKiroFailureSession(
	t *testing.T,
	session *inference.ProviderSession,
	wantSession string,
) {
	t.Helper()
	if wantSession == "" {
		if session != nil {
			t.Fatalf("failure Provider Session = %#v, want nil", session)
		}
		return
	}
	if session == nil || session.Provider() != providerIdentity || session.ID() != wantSession {
		t.Fatalf(
			"failure Provider Session = %#v, want provider=%q id=%q",
			session,
			providerIdentity,
			wantSession,
		)
	}
}

func TestIntegrationPropagatesCancellationForProtocolNormalization(t *testing.T) {
	t.Parallel()

	runner := &recordingRunner{err: context.Canceled}
	writer := &recordingWriter{}
	err := inference.ExecuteInvocation(
		context.Background(),
		NewIntegration(IntegrationDependencies{CommandRunner: runner}),
		failureInvocationRequest("inv-kiro-canceled"),
		writer,
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteInvocation() error = %v, want context.Canceled", err)
	}
	if writer.closes != 1 || writer.completion.Response() != nil || len(writer.events) != 0 {
		t.Fatalf(
			"completion = %#v, closes = %d, events = %#v; want one failed close",
			writer.completion, writer.closes, writer.events,
		)
	}
	failure := writer.completion.Failure()
	if failure == nil ||
		failure.Kind() != inference.FailureCanceled ||
		failure.Message() != "provider invocation was canceled" ||
		failure.Retryable() {
		t.Fatalf("failure = %#v, want canonical non-retryable cancellation", failure)
	}
}

func failureInvocationRequest(id string) inference.InvocationRequest {
	return inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: id,
		UserMessage:  "private customer request",
	})
}
