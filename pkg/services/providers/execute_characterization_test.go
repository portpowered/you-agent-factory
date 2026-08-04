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

// Continue implements the published Providers Service continuation slice for
// executePeerFake using only Providers root contracts: a supported reference
// resumes through its explicit continuation operation, while ordinary Execute
// remains incapable of selecting a prior session.
func (fake *executePeerFake) Continue(
	ctx context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	descriptor, ok := fake.providers[request.Reference.Provider]
	if !ok {
		return providers.ContinueResult{}, providers.ErrUnknownProvider
	}
	if !hasCapability(descriptor, providers.CapabilitySessionResume) {
		return providers.ContinueResult{
			Reference: request.Reference,
			Outcome:   providers.ContinuationOutcomeUnsupported,
		}, nil
	}
	reference := request.Reference.Clone()
	result, err := fake.Execute(ctx, request.Attempt.Clone())
	if err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: reference,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result:    result,
	}, nil
}

func TestContinuationContract_Characterization_ResumedVersusUnsupported(t *testing.T) {
	t.Parallel()

	resumable := providers.Descriptor{
		ID:           providers.IDCodex,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilitySessionResume},
	}
	unresumable := providers.Descriptor{
		ID:           providers.IDClaude,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
	}
	var root providers.Service = newExecutePeerFake("cancel-attempt", resumable, unresumable)

	resumed, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "prior-session"},
		Attempt: providers.ExecuteRequest{
			Provider:    providers.IDCodex,
			AttemptID:   "attempt-continue",
			UserMessage: "continue",
		},
	})
	if err != nil {
		t.Fatalf("Continue(resumable) error = %v, want nil", err)
	}
	if resumed.Outcome != providers.ContinuationOutcomeResumed {
		t.Fatalf("Continue(resumable).Outcome = %q, want resumed", resumed.Outcome)
	}
	if resumed.Reference.Provider != providers.IDCodex ||
		resumed.Reference.Kind != providers.SessionIDKind ||
		resumed.Reference.ID != "prior-session" {
		t.Fatalf("Continue(resumable).Reference = %#v, want the exact continued reference", resumed.Reference)
	}
	if resumed.Result.Content != "continue-result" {
		t.Fatalf("Continue(resumable).Result.Content = %q, want continue-result", resumed.Result.Content)
	}

	unsupported, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDClaude, Kind: providers.SessionIDKind, ID: "prior-session"},
		Attempt: providers.ExecuteRequest{
			Provider:  providers.IDClaude,
			AttemptID: "attempt-unsupported",
		},
	})
	if err != nil {
		t.Fatalf("Continue(unresumable) error = %v, want nil", err)
	}
	if unsupported.Outcome != providers.ContinuationOutcomeUnsupported {
		t.Fatalf("Continue(unresumable).Outcome = %q, want unsupported", unsupported.Outcome)
	}
	if (unsupported.Result != providers.ExecuteResult{}) {
		t.Fatalf("Continue(unresumable).Result = %#v, want zero value when unsupported", unsupported.Result)
	}
	if resumed.Outcome == unsupported.Outcome {
		t.Fatal("resumed and unsupported outcomes must be distinguishable typed values")
	}
}

func TestContinuationContract_Characterization_ValidationFailuresPrecedeOutcome(t *testing.T) {
	t.Parallel()

	root := newExecutePeerFake("cancel-attempt", providers.Descriptor{
		ID:           providers.IDCodex,
		Availability: providers.AvailabilitySelectable,
		Readiness:    providers.ReadinessReady,
		Capabilities: []providers.Capability{providers.CapabilitySessionResume},
	})

	_, err := root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Kind: providers.SessionIDKind, ID: "prior-session"},
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"},
	})
	if !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("Continue(blank provider) error = %v, want ErrInvalidContinuationRequest", err)
	}
	var failure providers.ContinuationFailure
	if !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindInvalid {
		t.Fatalf("Continue(blank provider) error = %#v, want ContinuationFailureKindInvalid", err)
	}

	_, err = root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDCodex, ID: "prior-session"},
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"},
	})
	if !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("Continue(blank kind) error = %v, want ErrInvalidContinuationRequest", err)
	}

	_, err = root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind},
		Attempt:   providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"},
	})
	if !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("Continue(blank id) error = %v, want ErrInvalidContinuationRequest", err)
	}

	_, err = root.Continue(context.Background(), providers.ContinueRequest{
		Reference: providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "prior-session"},
		Attempt:   providers.ExecuteRequest{Provider: providers.IDClaude, AttemptID: "attempt-1"},
	})
	if !errors.Is(err, providers.ErrContinuationForeign) {
		t.Fatalf("Continue(mismatched attempt provider) error = %v, want ErrContinuationForeign", err)
	}
	if !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindForeign {
		t.Fatalf("Continue(mismatched attempt provider) error = %#v, want ContinuationFailureKindForeign", err)
	}
}

func TestContinuationFailure_ErrorAndUnwrapBranching(t *testing.T) {
	t.Parallel()

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "prior-session"}
	cases := []struct {
		kind providers.ContinuationFailureKind
		want error
	}{
		{providers.ContinuationFailureKindInvalid, providers.ErrInvalidContinuationRequest},
		{providers.ContinuationFailureKindForeign, providers.ErrContinuationForeign},
		{providers.ContinuationFailureKindStale, providers.ErrContinuationStale},
	}
	for _, testCase := range cases {
		failure := providers.ContinuationFailure{Kind: testCase.kind, Reference: reference}
		if !errors.Is(failure, testCase.want) {
			t.Fatalf("ContinuationFailure{Kind: %q}.Unwrap() mismatch, want errors.Is(%v)", testCase.kind, testCase.want)
		}
		if failure.Error() != testCase.want.Error() {
			t.Fatalf("ContinuationFailure{Kind: %q}.Error() = %q, want %q", testCase.kind, failure.Error(), testCase.want.Error())
		}
		withMessage := providers.ContinuationFailure{Kind: testCase.kind, Message: "detail", Reference: reference}
		if withMessage.Error() != testCase.want.Error()+": detail" {
			t.Fatalf("ContinuationFailure{Kind: %q}.Error() with message = %q", testCase.kind, withMessage.Error())
		}
	}
}

func TestContinuationContract_Characterization_CloningDetachesReferencesAndResults(t *testing.T) {
	t.Parallel()

	session := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "prior-session"}
	request := providers.ContinueRequest{
		Reference: session,
		Attempt: providers.ExecuteRequest{
			Provider:  providers.IDCodex,
			AttemptID: "attempt-1",
			EnvVars:   map[string]string{"KEY": "value"},
		},
	}
	cloned := request.Clone()
	if cloned.Reference != request.Reference {
		t.Fatalf("ContinueRequest.Clone().Reference = %#v, want %#v", cloned.Reference, request.Reference)
	}
	cloned.Attempt.EnvVars["KEY"] = "mutated"
	if request.Attempt.EnvVars["KEY"] != "value" {
		t.Fatal("ContinueRequest.Clone() shares Attempt.EnvVars backing map")
	}

	sessionRef := &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "result-session"}
	result := providers.ContinueResult{
		Reference: session,
		Outcome:   providers.ContinuationOutcomeResumed,
		Result: providers.ExecuteResult{
			Content:    "hello",
			SessionRef: sessionRef,
		},
	}
	clonedResult := result.Clone()
	if clonedResult.Reference != result.Reference || clonedResult.Outcome != result.Outcome {
		t.Fatalf("ContinueResult.Clone() = %#v, want %#v", clonedResult, result)
	}
	if clonedResult.Result.SessionRef == result.Result.SessionRef {
		t.Fatal("ContinueResult.Clone() shares Result.SessionRef pointer")
	}

	failure := providers.ContinuationFailure{
		Kind:      providers.ContinuationFailureKindStale,
		Message:   "no longer live",
		Reference: session,
	}
	clonedFailure := failure.Clone()
	if clonedFailure != failure {
		t.Fatalf("ContinuationFailure.Clone() = %#v, want %#v", clonedFailure, failure)
	}
}

func TestExecuteRequestReasoningEffortValidation(t *testing.T) {
	t.Parallel()

	request := providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "attempt-effort",
		ReasoningEffort: " XHIGH ",
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("Validate(xhigh) = %v", err)
	}
	if got, ok := providers.ReasoningEffort(request.ReasoningEffort).Canonical(); !ok || got != "xhigh" {
		t.Fatalf("ReasoningEffort.Canonical() = %q, %t; want xhigh, true", got, ok)
	}
	request.ReasoningEffort = "extreme"
	if err := request.Validate(); err == nil || !strings.Contains(err.Error(), "unsupported reasoning effort") {
		t.Fatalf("Validate(extreme) = %v, want unsupported reasoning effort", err)
	}
}

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
