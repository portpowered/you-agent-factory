package providers_test

import (
	"context"
	"errors"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// ControlAttempt implements the published Providers Service control slice for
// catalogPeerFake using only Providers root contracts. The fake answers every
// valid action with the canonical deterministic unsupported outcome, proving
// the vocabulary is usable purely from the public root package.
func (fake *catalogPeerFake) ControlAttempt(
	_ context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   providers.ControlOutcomeUnsupported,
	}, nil
}

func TestControlContract_Characterization_UnsupportedForEveryAction(t *testing.T) {
	t.Parallel()

	var root providers.Service = newCatalogPeerFake(providers.Descriptor{ID: providers.IDCodex})

	for _, action := range []providers.ControlAction{
		providers.ControlActionPause,
		providers.ControlActionCancel,
		providers.ControlActionTerminate,
	} {
		result, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
			Action:    action,
		})
		if err != nil {
			t.Fatalf("ControlAttempt(%q) error = %v, want nil", action, err)
		}
		if result.Provider != providers.IDCodex ||
			result.AttemptID != "attempt-1" ||
			result.Action != action ||
			result.Outcome != providers.ControlOutcomeUnsupported {
			t.Fatalf("ControlAttempt(%q) = %#v, want unsupported echo", action, result)
		}
	}
}

// controlPeerFake implements the published Providers Service control slice
// with configurable per-attempt outcomes, proving that completed, unsupported,
// and genuine operation failures are all distinguishable using only Providers
// root contracts.
type controlPeerFake struct {
	catalogPeerFake
	completedAttemptID string
	failingAttemptID   string
}

func newControlPeerFake(completedAttemptID, failingAttemptID string, entries ...providers.Descriptor) *controlPeerFake {
	return &controlPeerFake{
		catalogPeerFake:    *newCatalogPeerFake(entries...),
		completedAttemptID: completedAttemptID,
		failingAttemptID:   failingAttemptID,
	}
}

func (fake *controlPeerFake) ControlAttempt(
	_ context.Context,
	request providers.ControlAttemptRequest,
) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	if request.AttemptID == fake.failingAttemptID {
		return providers.ControlAttemptResult{}, providers.ErrUnknownProvider
	}
	outcome := providers.ControlOutcomeUnsupported
	if request.AttemptID == fake.completedAttemptID {
		outcome = providers.ControlOutcomeCompleted
	}
	return providers.ControlAttemptResult{
		Provider:  request.Provider,
		AttemptID: request.AttemptID,
		Action:    request.Action,
		Outcome:   outcome,
	}, nil
}

func TestControlContract_Characterization_CompletedVersusUnsupportedVersusError(t *testing.T) {
	t.Parallel()

	var root providers.Service = newControlPeerFake(
		"completed-attempt",
		"failing-attempt",
		providers.Descriptor{ID: providers.IDCodex},
	)

	completed, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "completed-attempt",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(completed) error = %v, want nil", err)
	}
	if completed.Outcome != providers.ControlOutcomeCompleted {
		t.Fatalf("ControlAttempt(completed).Outcome = %q, want completed", completed.Outcome)
	}

	unsupported, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "unsupported-attempt",
		Action:    providers.ControlActionCancel,
	})
	if err != nil {
		t.Fatalf("ControlAttempt(unsupported) error = %v, want nil", err)
	}
	if unsupported.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt(unsupported).Outcome = %q, want unsupported", unsupported.Outcome)
	}
	if completed.Outcome == unsupported.Outcome {
		t.Fatal("completed and unsupported outcomes must be distinguishable typed values")
	}

	failed, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "failing-attempt",
		Action:    providers.ControlActionCancel,
	})
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("ControlAttempt(failing) error = %v, want ErrUnknownProvider", err)
	}
	if (failed != providers.ControlAttemptResult{}) {
		t.Fatalf("ControlAttempt(failing) result = %#v, want zero value when a genuine operation error is returned", failed)
	}
}

func TestControlContract_Characterization_ValidationFailuresPrecedeOutcome(t *testing.T) {
	t.Parallel()

	root := newCatalogPeerFake(providers.Descriptor{ID: providers.IDCodex})

	_, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  "",
		AttemptID: "attempt-1",
		Action:    providers.ControlActionPause,
	})
	if !errors.Is(err, providers.ErrInvalidID) {
		t.Fatalf("ControlAttempt(empty provider) error = %v, want ErrInvalidID", err)
	}

	_, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "   ",
		Action:    providers.ControlActionPause,
	})
	if !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAttempt(empty attempt id) error = %v, want ErrInvalidControlRequest", err)
	}

	_, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
		Action:    providers.ControlAction("resume"),
	})
	if !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAttempt(unknown action) error = %v, want ErrInvalidControlRequest", err)
	}

	_, err = root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{
		Provider:  providers.IDCodex,
		AttemptID: "attempt-1",
	})
	if !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAttempt(zero action) error = %v, want ErrInvalidControlRequest", err)
	}
}

func TestControlActionValidate(t *testing.T) {
	t.Parallel()

	for _, action := range []providers.ControlAction{
		providers.ControlActionPause,
		providers.ControlActionCancel,
		providers.ControlActionTerminate,
	} {
		if err := action.Validate(); err != nil {
			t.Fatalf("ControlAction(%q).Validate() = %v, want nil", action, err)
		}
	}
	if err := providers.ControlAction("").Validate(); !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAction(\"\").Validate() = %v, want ErrInvalidControlRequest", err)
	}
	if err := providers.ControlAction("pause_all").Validate(); !errors.Is(err, providers.ErrInvalidControlRequest) {
		t.Fatalf("ControlAction(pause_all).Validate() = %v, want ErrInvalidControlRequest", err)
	}
}
