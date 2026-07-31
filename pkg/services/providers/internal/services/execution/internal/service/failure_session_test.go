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
