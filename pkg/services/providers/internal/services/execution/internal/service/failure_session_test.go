package service_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
)

func TestExecuteNormalizesAndDetachesFailureSession(t *testing.T) {
	t.Parallel()

	nativeSession := &providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "failure-session-1",
	}
	nativeFailure := providers.ExecuteFailure{
		Kind:       providers.ExecuteFailureKindAuthentication,
		Message:    "authentication failed",
		SessionRef: nativeSession,
	}
	executionService := mustExecutionService(t, func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, nativeFailure
	})

	_, executeErr := executionService.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindAuthentication ||
		failure.SessionRef == nil ||
		failure.SessionRef.ID != nativeSession.ID {
		t.Fatalf("Execute() error = %#v, want detached failure session", executeErr)
	}

	failure.SessionRef.ID = "caller-mutated"
	if nativeSession.ID != "failure-session-1" {
		t.Fatalf("adapter failure session = %#v, want unchanged", nativeSession)
	}
}

func TestExecuteRejectsInvalidOrCrossProviderFailureSession(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		session providers.SessionRef
	}{
		{
			name: "invalid",
			session: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
			},
		},
		{
			name: "cross provider",
			session: providers.SessionRef{
				Provider: providers.IDClaude,
				Kind:     providers.SessionIDKind,
				ID:       "failure-session-1",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executionService := mustExecutionService(t, func(
				context.Context,
				providers.ExecuteRequest,
			) (providers.ExecuteResult, error) {
				return providers.ExecuteResult{}, providers.ExecuteFailure{
					Kind:       providers.ExecuteFailureKindDependency,
					SessionRef: &test.session,
				}
			})

			result, executeErr := executionService.Execute(
				context.Background(),
				providers.ExecuteRequest{
					Provider:  providers.IDCodex,
					AttemptID: "attempt-1",
				},
			)
			if !errors.Is(executeErr, providers.ErrExecuteFailed) {
				t.Fatalf("Execute() error = %v, want ErrExecuteFailed", executeErr)
			}
			if !reflect.DeepEqual(result, providers.ExecuteResult{}) {
				t.Fatalf("Execute() result = %#v, want zero result", result)
			}
			var failure providers.ExecuteFailure
			if errors.As(executeErr, &failure) {
				t.Fatalf("Execute() error = %#v, want malformed failure reference rejected", executeErr)
			}
		})
	}
}

func TestExecuteAppliesDefaultMessageForSessionNotFound(t *testing.T) {
	t.Parallel()

	executionService := mustExecutionService(t, func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind: providers.ExecuteFailureKindSessionNotFound,
		}
	})

	_, executeErr := executionService.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.Kind != providers.ExecuteFailureKindSessionNotFound ||
		failure.Message != "provider does not recognize the referenced Provider Session as live" {
		t.Fatalf("Execute() error = %#v, want default SessionNotFound message", executeErr)
	}
}

func TestExecuteAppliesDefaultMessageForCapabilityMismatch(t *testing.T) {
	t.Parallel()
	service := mustExecutionService(t, func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCapabilityMismatch}
	})
	_, err := service.Execute(t.Context(), providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "capability-failure"})
	var failure providers.ExecuteFailure
	if !errors.Is(err, providers.ErrCapabilityMismatch) || !errors.As(err, &failure) ||
		failure.Kind != providers.ExecuteFailureKindCapabilityMismatch ||
		failure.Message != "provider does not support the requested capability" {
		t.Fatalf("Execute error = %#v, want classified capability failure with default message", err)
	}
}

func TestExecutePreservesSafeFailurePathAndRedactsRequestSecrets(t *testing.T) {
	t.Parallel()
	const path = "C:/provider/work"
	const secret = "private-request-secret"
	service := mustExecutionService(t, func(context.Context, providers.ExecuteRequest) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindDependency,
			Message: "cannot open " + path + " with " + secret,
			Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{
				providers.ExecuteDiagnosticMetadataSafeFailureMessage: "true",
			}},
		}
	})
	_, err := service.Execute(t.Context(), providers.ExecuteRequest{
		Provider: providers.IDCodex, AttemptID: "safe-path", WorkingDirectory: path, SystemPrompt: secret,
	})
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ExecuteFailureKindDependency ||
		failure.Message != "cannot open "+path+" with <redacted>" {
		t.Fatalf("Execute failure = %#v, want actionable path with request secret redacted", err)
	}
}

func TestExecuteCarriesLifecycleFailureSession(t *testing.T) {
	t.Parallel()

	nativeSession := &providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "lifecycle-failure-session",
	}
	executionService := mustExecutionService(t, func(
		context.Context,
		providers.ExecuteRequest,
	) (providers.ExecuteResult, error) {
		return providers.ExecuteResult{}, execution.AttemptFailure{
			Declared: &providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindDependency,
			},
			SessionRef:  nativeSession,
			NativeError: errors.New("native failure detail"),
		}
	})

	_, executeErr := executionService.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
	})
	var failure providers.ExecuteFailure
	if !errors.As(executeErr, &failure) ||
		failure.SessionRef == nil ||
		failure.SessionRef.ID != nativeSession.ID {
		t.Fatalf("Execute() error = %#v, want lifecycle failure session", executeErr)
	}
	failure.SessionRef.ID = "caller-mutated"
	if nativeSession.ID != "lifecycle-failure-session" {
		t.Fatalf("adapter lifecycle session = %#v, want unchanged", nativeSession)
	}
}
