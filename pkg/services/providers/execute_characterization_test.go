package providers_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// executePeerFake implements the published Providers Service execute slice
// using only Providers root contracts.
type executePeerFake struct {
	catalogPeerFake
	cancelAttemptID string
}

func newExecutePeerFake(cancelAttemptID string, entries ...providers.Descriptor) *executePeerFake {
	return &executePeerFake{
		catalogPeerFake: *newCatalogPeerFake(entries...),
		cancelAttemptID: cancelAttemptID,
	}
}

func (fake *executePeerFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ExecuteResult{}, err
	}
	if _, ok := fake.providers[request.Provider]; !ok {
		return providers.ExecuteResult{}, providers.ErrUnknownProvider
	}
	if request.AttemptID == fake.cancelAttemptID {
		return providers.ExecuteResult{}, providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindCanceled,
			Message: "attempt cancelled by peer policy",
		}
	}

	session := providers.SessionRef{
		Provider: request.Provider,
		Kind:     providers.SessionIDKind,
		ID:       "session-" + request.AttemptID,
	}
	return providers.ExecuteResult{
		Content:    strings.TrimSpace(request.UserMessage) + "-result",
		SessionRef: &session,
		Diagnostics: &providers.ExecuteDiagnostics{
			DurationMillis: 42,
			Progress: []providers.ExecuteProgress{{
				Phase:  "completed",
				Detail: "one attempt finished",
			}},
		},
	}, nil
}

func (fake *catalogPeerFake) Execute(
	_ context.Context,
	_ providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, providers.ErrExecuteFailed
}

func TestExecuteContract_Characterization_SuccessWithSessionRef(t *testing.T) {
	t.Parallel()

	service := newExecutePeerFake("cancel-attempt", providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	})

	var root providers.Service = service
	result, err := root.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "attempt-1",
		UserMessage: "hello",
	})
	if err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	if result.Content != "hello-result" {
		t.Fatalf("Content = %q, want hello-result", result.Content)
	}
	if result.SessionRef == nil ||
		result.SessionRef.Provider != providers.IDCodex ||
		result.SessionRef.Kind != providers.SessionIDKind ||
		result.SessionRef.ID != "session-attempt-1" {
		t.Fatalf("SessionRef = %#v", result.SessionRef)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.DurationMillis != 42 ||
		len(result.Diagnostics.Progress) != 1 ||
		result.Diagnostics.Progress[0].Phase != "completed" {
		t.Fatalf("Diagnostics = %#v", result.Diagnostics)
	}

	cloned := result.Clone()
	if cloned.SessionRef == result.SessionRef {
		t.Fatal("ExecuteResult.Clone() shares SessionRef pointer")
	}
	if cloned.Diagnostics == result.Diagnostics {
		t.Fatal("ExecuteResult.Clone() shares Diagnostics pointer")
	}
}

func TestExecuteContract_Characterization_TypedFailures(t *testing.T) {
	t.Parallel()

	service := newExecutePeerFake("cancel-attempt", providers.Descriptor{
		ID:           providers.IDCodex,
		DisplayName:  "Codex",
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	})

	_, err := service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "cancel-attempt",
	})
	if !errors.Is(err, providers.ErrExecuteCancelled) {
		t.Fatalf("cancelled Execute() error = %v, want ErrExecuteCancelled", err)
	}
	var failure providers.ExecuteFailure
	if !errors.As(err, &failure) {
		t.Fatalf("cancelled Execute() error = %T(%v), want ExecuteFailure", err, err)
	}
	if failure.Kind != providers.ExecuteFailureKindCanceled {
		t.Fatalf("failure.Kind = %q, want canceled", failure.Kind)
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDClaude,
		AttemptID: "attempt-2",
	})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("unknown provider Execute() error = %v, want ErrUnknownProvider", err)
	}

	_, err = service.Execute(context.Background(), providers.ExecuteRequest{
		Provider:  providers.IDCodex,
		AttemptID: "",
	})
	if !errors.Is(err, providers.ErrExecuteFailed) {
		t.Fatalf("invalid attempt Execute() error = %v, want ErrExecuteFailed", err)
	}
}
