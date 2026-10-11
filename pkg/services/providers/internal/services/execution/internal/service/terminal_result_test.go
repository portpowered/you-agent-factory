package service_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

const (
	terminalResultSecret = "terminal-result-secret"
	terminalResumeID     = "resume-session-secret"
)

func TestExecutionOnlyWorkerTokenRemainsSecretAfterAdapterMutation(t *testing.T) {
	t.Parallel()
	for _, route := range []string{"execute", "continue"} {
		for _, outcome := range []string{"success", "failure", "cancel"} {
			t.Run(route+"/"+outcome, func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				const token = "planted-execution-only-worker-token"
				var observed providers.ExecuteProgress
				attempt := func(_ context.Context, request providers.ExecuteRequest) (providers.ExecuteResult, error) {
					if request.ProcessEnvironment[0] != "YOU_WORKER_SESSION_TOKEN="+token {
						t.Fatal("execution did not receive the token")
					}
					request.ProcessEnvironment[0] = "YOU_WORKER_SESSION_TOKEN=changed-by-adapter"
					request.ObserveProgress(providers.ExecuteProgress{Phase: "delta", Detail: "visible " + token})
					result := terminalResultFixture("accepted "+token+" visible result", "visible "+token, token)
					result.Diagnostics.Metadata["safe"] = token
					if outcome == "success" {
						return result, nil
					}
					if outcome == "cancel" {
						cancel()
					}
					failure := terminalFailureFixture("failure "+token, token)
					failure.Diagnostics.Metadata["safe"] = token
					return result, failure
				}
				service, err := executionwire.NewService(mustCatalog(t), execution.Registration{
					Provider: providers.IDCodex, Attempt: attempt,
					Continue: func(ctx context.Context, request execution.ContinuationRequest) (providers.ExecuteResult, error) {
						return attempt(ctx, request.ExecuteRequest)
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				request := providers.ExecuteRequest{
					Provider: providers.IDCodex, AttemptID: "token-privacy",
					ProcessEnvironment: []string{"YOU_WORKER_SESSION_TOKEN=" + token},
					ProgressObserver:   func(progress providers.ExecuteProgress) { observed = progress },
				}
				var result providers.ExecuteResult
				if route == "continue" {
					result, err = service.Continue(ctx, execution.ContinuationRequest{
						ExecuteRequest: request,
						ResumeSession:  &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "resume-source"},
					})
				} else {
					result, err = service.Execute(ctx, request)
				}
				assertWorkerTokenDiagnostics(t, result, err, observed, outcome, token)
				if request.ProcessEnvironment[0] != "YOU_WORKER_SESSION_TOKEN="+token {
					t.Fatal("adapter mutated caller-owned environment")
				}
			})
		}
	}
}

func assertWorkerTokenDiagnostics(t *testing.T, result providers.ExecuteResult, err error, observed providers.ExecuteProgress, outcome, token string) {
	t.Helper()
	assertWorkerTokenResult(t, result, observed)
	if outcome == "success" {
		if err != nil {
			t.Fatal(err)
		}
		return
	}
	want := providers.ErrExecuteFailed
	if outcome == "cancel" {
		want = providers.ErrExecuteCancelled
	}
	var failure providers.ExecuteFailure
	if !errors.Is(err, want) || !errors.As(err, &failure) || strings.Contains(err.Error(), token) {
		t.Fatal("failure lost its typed outcome or leaked the worker token")
	}
	// Cancellation intentionally discards adapter failure diagnostics. A
	// declared failure preserves them, with the same secret classification.
	if outcome == "failure" && (failure.Diagnostics == nil || failure.Diagnostics.Metadata["safe"] != "<redacted>") {
		t.Fatal("declared failure diagnostics lost worker token redaction")
	}
	if failure.Diagnostics != nil && strings.Contains(failure.Diagnostics.Metadata["stderr"], token) {
		t.Fatal("failure diagnostics leaked the worker token")
	}
}

func assertWorkerTokenResult(t *testing.T, result providers.ExecuteResult, observed providers.ExecuteProgress) {
	t.Helper()
	if result.Content != "accepted <redacted> visible result" {
		t.Fatal("returned content leaked the worker token or lost neighboring text")
	}
	if observed.Detail != "visible <redacted>" || result.Diagnostics == nil || result.Diagnostics.Progress[0].Detail != "visible <redacted>" || result.Diagnostics.Metadata["safe"] != "<redacted>" {
		t.Fatal("live or returned progress leaked the worker token")
	}
}

func TestContinueRejectsInvalidAdapterResult(t *testing.T) {
	t.Parallel()
	for _, session := range []providers.SessionRef{
		{Provider: providers.IDCodex, Kind: providers.SessionIDKind},
		{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "foreign-session"},
	} {
		t.Run(string(session.Provider), func(t *testing.T) {
			t.Parallel()
			service, err := executionwire.NewService(mustCatalog(t), execution.Registration{
				Provider: providers.IDCodex,
				Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
					t.Fatal("continuation dispatched ordinary attempt")
					return providers.ExecuteResult{}, nil
				},
				Continue: func(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
					return providers.ExecuteResult{Content: "untrusted result", SessionRef: &session}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			result, err := service.Continue(t.Context(), execution.ContinuationRequest{
				ExecuteRequest: providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "invalid-result"},
				ResumeSession:  &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: terminalResumeID},
			})
			if !reflect.DeepEqual(result, providers.ExecuteResult{}) || !errors.Is(err, providers.ErrExecuteFailed) {
				t.Fatalf("Continue = (%#v, %v), want zero result and execution failure", result, err)
			}
			assertContinuationExecuteFailure(t, err, providers.ExecuteFailureKindUnknown)
			if strings.Contains(err.Error(), terminalResumeID) {
				t.Fatalf("failure leaked resume reference: %v", err)
			}
		})
	}
}

func TestExecutePreservesNormalizedResultAndError(t *testing.T) {
	t.Parallel()

	nativeResult := terminalResultFixture(
		"accepted "+terminalResultSecret,
		"accepted "+terminalResultSecret,
		terminalResultSecret,
	)
	nativeFailure := terminalFailureFixture(
		"failure "+terminalResultSecret,
		terminalResultSecret,
	)
	executionService := mustTerminalTupleService(t, nativeResult, nativeFailure)

	result, executeErr := executionService.Execute(context.Background(), providers.ExecuteRequest{
		Provider:     providers.IDCodex,
		AttemptID:    "attempt-terminal-result",
		SystemPrompt: terminalResultSecret,
		UserMessage:  "accept this result",
	})

	assertNormalizedResultAndError(t, result, executeErr, "accepted "+terminalResultSecret, "")
	nativeResult.SessionRef.ID = "mutated-after-return"
	nativeResult.Diagnostics.Metadata["safe"] = "mutated-after-return"
	if result.SessionRef.ID != "codex-session-terminal" ||
		result.Diagnostics.Metadata["safe"] != "result-fact" {
		t.Fatalf("normalized result retained adapter mutation: %#v", result)
	}
}

func TestContinuePreservesNormalizedResultAndError(t *testing.T) {
	t.Parallel()

	nativeResult := terminalResultFixture(
		"accepted "+terminalResultSecret+" "+terminalResumeID,
		"accepted "+terminalResultSecret+" "+terminalResumeID,
		terminalResultSecret+" "+terminalResumeID,
	)
	nativeFailure := terminalFailureFixture(
		"failure "+terminalResultSecret+" "+terminalResumeID,
		terminalResultSecret+" "+terminalResumeID,
	)
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, nil
			},
			Continue: func(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
				return nativeResult, nativeFailure
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}

	continuation, ok := executionService.(execution.ContinuationService)
	if !ok {
		t.Fatal("NewService() result does not implement ContinuationService")
	}
	result, continueErr := continuation.Continue(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:     providers.IDCodex,
			AttemptID:    "attempt-terminal-continuation",
			SystemPrompt: terminalResultSecret,
			UserMessage:  "resume this result",
		},
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       terminalResumeID,
		},
	})

	assertNormalizedResultAndError(t, result, continueErr,
		"accepted "+terminalResultSecret+" "+terminalResumeID,
		terminalResumeID,
	)
}

func TestContinuePreservesNormalizedResultWhenContextEndsDuringAttempt(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	nativeResult := terminalResultFixture("accepted before cancellation", "accepted before cancellation", "")
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, nil
			},
			Continue: func(context.Context, execution.ContinuationRequest) (providers.ExecuteResult, error) {
				cancel()
				return nativeResult, ctx.Err()
			},
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	defer cancel()

	continuation, ok := executionService.(execution.ContinuationService)
	if !ok {
		t.Fatal("NewService() result does not implement ContinuationService")
	}
	result, continueErr := continuation.Continue(ctx, execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-terminal-cancellation",
		},
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "resume-cancellation",
		},
	})

	if result.Content != "accepted before cancellation" {
		t.Fatalf("Continue() content = %q, want normalized candidate", result.Content)
	}
	if !errors.Is(continueErr, providers.ErrExecuteCancelled) {
		t.Fatalf("Continue() error = %v, want cancellation", continueErr)
	}
	var failure providers.ExecuteFailure
	if !errors.As(continueErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("Continue() error = %#v, want canceled ExecuteFailure", continueErr)
	}
}

func assertNormalizedResultAndError(
	t *testing.T,
	result providers.ExecuteResult,
	executeErr error,
	wantContent string,
	wantResumeID string,
) {
	t.Helper()
	assertNormalizedResult(t, result, wantContent, wantResumeID)
	assertNormalizedFailure(t, executeErr)
}

func assertNormalizedResult(
	t *testing.T,
	result providers.ExecuteResult,
	wantContent string,
	wantResumeID string,
) {
	t.Helper()
	if result.Content != wantContent || result.Outcome != providers.ExecuteOutcomeAccepted {
		t.Fatalf("normalized result = %#v, want accepted content/outcome", result)
	}
	if result.SessionRef == nil || result.SessionRef.ID != "codex-session-terminal" {
		t.Fatalf("normalized result session = %#v, want detached Codex session", result.SessionRef)
	}
	if result.Diagnostics == nil || result.Diagnostics.DurationMillis != 17 {
		t.Fatalf("normalized result diagnostics = %#v, want duration", result.Diagnostics)
	}
	if result.Diagnostics.Metadata["safe"] != "result-fact" ||
		result.Diagnostics.Metadata["stdout"] != "<redacted>" {
		t.Fatalf("normalized result metadata = %#v, want sanitized facts", result.Diagnostics.Metadata)
	}
	if strings.Contains(result.Diagnostics.Progress[0].Detail, terminalResultSecret) ||
		(wantResumeID != "" && strings.Contains(result.Diagnostics.Progress[0].Detail, wantResumeID)) {
		t.Fatalf("normalized result progress leaked secret: %#v", result.Diagnostics.Progress[0])
	}
}

func assertNormalizedFailure(
	t *testing.T,
	executeErr error,
) {
	t.Helper()
	if !errors.Is(executeErr, providers.ErrExecuteFailed) {
		t.Fatalf("execution error = %v, want failed sentinel", executeErr)
	}
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindDependency {
		t.Fatalf("execution error = %#v, want dependency ExecuteFailure", executeErr)
	}
	if failure.Diagnostics == nil || failure.Diagnostics.Metadata["stderr"] != "<redacted>" {
		t.Fatalf("failure diagnostics = %#v, want sanitized stderr", failure.Diagnostics)
	}
	if strings.Contains(executeErr.Error(), terminalResultSecret) ||
		strings.Contains(executeErr.Error(), terminalResumeID) {
		t.Fatalf("execution error leaked secret: %v", executeErr)
	}
}

func terminalResultFixture(content, progressDetail, secret string) providers.ExecuteResult {
	return providers.ExecuteResult{
		Content: content,
		Outcome: providers.ExecuteOutcomeAccepted,
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "codex-session-terminal",
		},
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 17,
			Progress: []providers.ExecuteProgress{{
				Phase:  "message.completed",
				Detail: progressDetail,
				Metadata: map[string]string{
					"safe":  "progress-fact",
					"token": secret,
				},
			}},
			Metadata: map[string]string{
				"safe":   "result-fact",
				"stdout": secret,
			},
		},
	}
}

func terminalFailureFixture(message, secret string) providers.ExecuteFailure {
	return providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindDependency,
		Message: message,
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 23,
			Metadata: map[string]string{
				"safe":   "failure-fact",
				"stderr": secret,
			},
		},
	}
}

func mustTerminalTupleService(
	t *testing.T,
	nativeResult providers.ExecuteResult,
	nativeFailure providers.ExecuteFailure,
) execution.Service {
	t.Helper()
	executionService, err := executionwire.NewService(
		mustCatalog(t),
		execution.Registration{
			Provider: providers.IDCodex,
			Attempt: func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
				return nativeResult, nativeFailure
			},
			Continue: executionwire.NewUnsupportedContinuation(),
		},
	)
	if err != nil {
		t.Fatalf("NewService() = %v", err)
	}
	return executionService
}
