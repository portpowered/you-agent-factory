package providers_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
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

	root := service
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
	root := newExecutePeerFake("cancel-attempt", resumable, unresumable)

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

	root := newControlPeerFake(
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
func TestProviderExecutionContractsCloneAndObserveDetachedValues(t *testing.T) {
	t.Run("request clone", assertExecuteRequestClone)
	t.Run("observers", assertExecuteObservers)
	t.Run("diagnostics and result clone", assertExecuteDiagnosticsClone)
	t.Run("failure kinds", assertExecuteFailureKinds)
	t.Run("reasoning and validation", assertExecuteReasoningAndValidation)
}

func assertExecuteRequestClone(t *testing.T) {
	t.Helper()
	metadata := map[string]any{"nested": []any{"before"}}
	request := providers.ExecuteRequest{
		Provider: providers.IDCodex, AttemptID: "attempt-1",
		Correlation:   providers.ExecuteCorrelation{WorkIDs: []string{"work-1"}},
		InputBindings: map[string][]string{"prompt": {"hello"}}, Args: []string{"--json"},
		RequiredCapabilities: []string{"structured_output"}, EnvVars: map[string]string{"TOKEN": "value"},
		ProcessEnvironment: []string{"TOKEN=value"}, InputTokens: []any{"token"},
		ModelBindings: []providers.ResolvedModelOperationBinding{{Slot: "prompt", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"answer":true}`), Metadata: metadata}}}},
	}
	if err := request.Validate(); err != nil {
		t.Fatalf("ExecuteRequest.Validate() = %v", err)
	}
	cloned := request.Clone()
	request.Correlation.WorkIDs[0], request.InputBindings["prompt"][0] = "mutated", "mutated"
	request.Args[0], request.RequiredCapabilities[0], request.EnvVars["TOKEN"] = "mutated", "mutated", "mutated"
	request.ProcessEnvironment[0] = "mutated"
	request.ModelBindings[0].Content[0].Metadata["nested"].([]any)[0] = "mutated"
	if cloned.Correlation.WorkIDs[0] != "work-1" || cloned.InputBindings["prompt"][0] != "hello" || cloned.Args[0] != "--json" || cloned.RequiredCapabilities[0] != "structured_output" || cloned.EnvVars["TOKEN"] != "value" || cloned.ProcessEnvironment[0] != "TOKEN=value" || cloned.ModelBindings[0].Content[0].Metadata["nested"].([]any)[0] != "before" {
		t.Fatalf("ExecuteRequest.Clone() did not detach request values: %#v", cloned)
	}
}

func assertExecuteObservers(t *testing.T) {
	t.Helper()
	request := providers.ExecuteRequest{}
	var observedSession providers.SessionRef
	sessionObservations := 0
	request.SessionObserver = func(reference providers.SessionRef) { sessionObservations++; observedSession = reference }
	request.ObserveSession(providers.SessionRef{})
	request.ObserveSession(providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"})
	if sessionObservations != 1 || observedSession.ID != "session-1" {
		t.Fatalf("ObserveSession() = %#v, observations %d; want one valid detached observation", observedSession, sessionObservations)
	}
	var observedProgress providers.ExecuteProgress
	request.ProgressObserver = func(progress providers.ExecuteProgress) { observedProgress = progress }
	progress := providers.ExecuteProgress{Phase: "running", Metadata: map[string]string{"step": "one"}}
	request.ObserveProgress(progress)
	progress.Metadata["step"] = "mutated"
	if observedProgress.Phase != "running" || observedProgress.Metadata["step"] != "one" {
		t.Fatalf("ObserveProgress() = %#v, want detached progress", observedProgress)
	}
}

func assertExecuteDiagnosticsClone(t *testing.T) {
	t.Helper()
	diagnostics := providers.ExecuteDiagnostics{
		Progress: []providers.ExecuteProgress{{Phase: "done", Metadata: map[string]string{"result": "ok"}}},
		Metadata: map[string]string{"safe": "yes"},
		Command:  &providers.ExecuteCommandDiagnostics{Args: []string{"--safe"}, Env: map[string]string{"KEY": "value"}},
		Panic:    &providers.ExecutePanicDiagnostics{Message: "bounded", Stack: "bounded-stack"},
	}
	clonedDiagnostics := diagnostics.Clone()
	diagnostics.Progress[0].Metadata["result"], diagnostics.Metadata["safe"] = "mutated", "mutated"
	diagnostics.Command.Args[0], diagnostics.Command.Env["KEY"], diagnostics.Panic.Message = "mutated", "mutated", "mutated"
	if clonedDiagnostics.Progress[0].Metadata["result"] != "ok" || clonedDiagnostics.Metadata["safe"] != "yes" || clonedDiagnostics.Command.Args[0] != "--safe" || clonedDiagnostics.Command.Env["KEY"] != "value" || clonedDiagnostics.Panic.Message != "bounded" {
		t.Fatalf("ExecuteDiagnostics.Clone() shares nested state: %#v", clonedDiagnostics)
	}
	result := providers.ExecuteResult{Content: "answer", SessionRef: &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-2"}, Diagnostics: &diagnostics}
	clonedResult := result.Clone()
	if clonedResult.SessionRef == result.SessionRef || clonedResult.Diagnostics == result.Diagnostics {
		t.Fatal("ExecuteResult.Clone() shares mutable pointers")
	}
}

func assertExecuteFailureKinds(t *testing.T) {
	t.Helper()
	result := providers.ExecuteResult{SessionRef: &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-2"}}
	diagnostics := &providers.ExecuteDiagnostics{Metadata: map[string]string{"safe": "yes"}}
	for _, testCase := range []struct {
		kind providers.ExecuteFailureKind
		want error
	}{{providers.ExecuteFailureKindCanceled, providers.ErrExecuteCancelled}, {providers.ExecuteFailureKindTimeout, providers.ErrExecuteTimeout}, {providers.ExecuteFailureKindCapabilityMismatch, providers.ErrCapabilityMismatch}, {providers.ExecuteFailureKindInvalidRequest, providers.ErrExecuteFailed}} {
		failure := providers.ExecuteFailure{Kind: testCase.kind, SessionRef: result.SessionRef, Diagnostics: diagnostics}
		if !errors.Is(failure, testCase.want) || !strings.Contains(failure.Error(), testCase.want.Error()) {
			t.Fatalf("ExecuteFailure(%q) = %v, want errors.Is(%v)", testCase.kind, failure, testCase.want)
		}
		withMessage := providers.ExecuteFailure{Kind: testCase.kind, Message: "detail"}
		if withMessage.Error() != testCase.want.Error()+": detail" {
			t.Fatalf("ExecuteFailure(%q).Error() = %q", testCase.kind, withMessage.Error())
		}
		clonedFailure := failure.Clone()
		if clonedFailure.SessionRef == failure.SessionRef || clonedFailure.Diagnostics == failure.Diagnostics {
			t.Fatalf("ExecuteFailure(%q).Clone() shares pointers", testCase.kind)
		}
	}
}

func assertExecuteReasoningAndValidation(t *testing.T) {
	t.Helper()
	for _, effort := range []string{"", " minimal ", "low", "medium", "high", "xhigh", "max"} {
		if _, ok := providers.ReasoningEffort(effort).Canonical(); !ok {
			t.Fatalf("ReasoningEffort(%q).Canonical() rejected supported value", effort)
		}
	}
	if _, ok := providers.ReasoningEffort("unsupported").Canonical(); ok {
		t.Fatal("ReasoningEffort(unsupported).Canonical() accepted an unsupported value")
	}
	for _, invalid := range []providers.ExecuteRequest{{AttemptID: "attempt-1"}, {Provider: providers.IDCodex}, {Provider: providers.IDCodex, AttemptID: "attempt-1", ReasoningEffort: "unsupported"}} {
		if err := invalid.Validate(); err == nil {
			t.Fatalf("ExecuteRequest.Validate(%#v) = nil, want validation error", invalid)
		}
	}
}
func TestProviderDescriptorClonePreservesNestedCatalogFacts(t *testing.T) {
	maximum, defaultValue := int64(128), int64(16)
	descriptor := providers.Descriptor{
		ID:            providers.IDCodex,
		Aliases:       []string{"openai-codex"},
		Prerequisites: []providers.Prerequisite{{Kind: providers.PrerequisiteDependency, Name: "codex", Status: providers.PrerequisiteSatisfied, Description: "installed"}},
		Models: []providers.ModelDescriptor{{
			ID: "gpt", Efforts: []providers.ReasoningEffort{"high"},
			Modalities: []providers.Modality{{Direction: providers.ModalityDirectionInput, Kind: providers.ModalityAudio, Support: providers.ModalitySupported, Transport: providers.ModalityTransportInline}},
		}},
		Tools:       []providers.Tool{{Name: "shell", Support: providers.ToolSupported, Description: "run shell"}},
		KnownLimits: []providers.KnownLimit{{Name: "tokens", Kind: providers.KnownLimitMaximum, Unit: "tokens", Maximum: &maximum, Default: &defaultValue, Value: "bounded"}},
	}
	cloned := descriptor.Clone()
	cloned.Aliases[0] = "mutated"
	cloned.Prerequisites[0].Description = "mutated"
	cloned.Models[0].Efforts[0] = "mutated"
	cloned.Models[0].Modalities[0].Transport = providers.ModalityTransportFilePath
	cloned.Tools[0].Description = "mutated"
	*cloned.KnownLimits[0].Maximum = 256
	*cloned.KnownLimits[0].Default = 32
	if descriptor.Aliases[0] == "mutated" || descriptor.Prerequisites[0].Description == "mutated" || descriptor.Models[0].Efforts[0] == "mutated" || descriptor.Models[0].Modalities[0].Transport == providers.ModalityTransportFilePath || descriptor.Tools[0].Description == "mutated" || *descriptor.KnownLimits[0].Maximum != 128 || *descriptor.KnownLimits[0].Default != 16 {
		t.Fatalf("Descriptor.Clone() shares nested catalog state: %#v", descriptor)
	}
}
func TestProviderContinuationReferenceContractNormalizesAndClones(t *testing.T) {
	t.Run("normalize and session", assertContinuationReferenceNormalization)
	t.Run("clone", assertContinuationReferenceClone)
	t.Run("invalid", assertContinuationReferenceInvalid)
}

func assertContinuationReferenceNormalization(t *testing.T) {
	t.Helper()
	valid := providers.ContinuationRef{Provider: " codex ", ProviderSessionID: " provider-session ", ExternalRef: " external "}
	if err := valid.Validate(); err != nil {
		t.Fatalf("ContinuationRef.Validate() = %v", err)
	}
	normalized := valid.Normalize()
	if normalized.Provider != "codex" || normalized.Kind != providers.SessionIDKind || normalized.ProviderSessionID != "provider-session" || normalized.ExternalRef != "external" {
		t.Fatalf("ContinuationRef.Normalize() = %#v, want trimmed session-id reference", normalized)
	}
	assertContinuationReferenceSessions(t, valid)
}

func assertContinuationReferenceSessions(t *testing.T, valid providers.ContinuationRef) {
	t.Helper()
	session, err := valid.ToSessionRef()
	if err != nil || session.Provider != providers.IDCodex || session.Kind != providers.SessionIDKind || session.ID != "provider-session" {
		t.Fatalf("ContinuationRef.ToSessionRef() = %#v, %v", session, err)
	}
	externalOnly := providers.ContinuationRef{Provider: "codex", Kind: "thread", ExternalRef: "external-only"}
	session, err = externalOnly.ToSessionRef()
	if err != nil || session.ID != "external-only" || session.Kind != "thread" {
		t.Fatalf("external-only ToSessionRef() = %#v, %v", session, err)
	}
	fromSession := (providers.SessionRef{Provider: providers.IDClaude, Kind: "thread", ID: "session-claude"}).ContinuationRef()
	if fromSession.Provider != "claude" || fromSession.ProviderSessionID != "session-claude" || fromSession.ExternalRef != "session-claude" || fromSession.String() != "claude/thread/session-claude" {
		t.Fatalf("ContinuationRefFromSession() = %#v, want exact identity", fromSession)
	}
}
func assertContinuationReferenceClone(t *testing.T) {
	t.Helper()
	valid := providers.ContinuationRef{Provider: "codex", Kind: "thread", ProviderSessionID: "session"}
	if cloned := valid.Clone(); cloned != valid {
		t.Fatalf("ContinuationRef.Clone() = %#v, want %#v", cloned, valid)
	}
}
func assertContinuationReferenceInvalid(t *testing.T) {
	t.Helper()
	for _, invalid := range []providers.ContinuationRef{{}, {Provider: "codex"}, {ProviderSessionID: "session"}} {
		if err := invalid.Validate(); !errors.Is(err, providers.ErrInvalidContinuationRef) {
			t.Fatalf("ContinuationRef.Validate(%#v) = %v, want ErrInvalidContinuationRef", invalid, err)
		}
		if _, err := invalid.ToSessionRef(); !errors.Is(err, providers.ErrInvalidContinuationRef) {
			t.Fatalf("ContinuationRef.ToSessionRef(%#v) = %v, want ErrInvalidContinuationRef", invalid, err)
		}
	}
}
func TestProviderOptionalContinuationRoutingIsExplicit(t *testing.T) {
	root := &providerContractCoverageRoot{identity: providers.IDCodex}
	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "session-1"}
	unsupported, err := root.Continue(context.Background(), providers.ContinueRequest{Reference: reference, Attempt: providers.ExecuteRequest{Provider: providers.IDCodex, AttemptID: "attempt-1"}})
	if err != nil || unsupported.Outcome != providers.ContinuationOutcomeUnsupported || root.executeCalls != 0 {
		t.Fatalf("Continue() = %#v, %v; want explicit unsupported without Execute", unsupported, err)
	}
	if _, err := root.Continue(context.Background(), providers.ContinueRequest{Reference: reference}); !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("Continue(invalid attempt) = %v, want ErrInvalidContinuationRequest", err)
	}
	control, err := root.ControlAttempt(context.Background(), providers.ControlAttemptRequest{Provider: providers.IDCodex, AttemptID: "attempt-1", Action: providers.ControlActionCancel})
	if err != nil || control.Outcome != providers.ControlOutcomeUnsupported {
		t.Fatalf("ControlAttempt() = %#v, %v; want explicit unsupported", control, err)
	}
}
func TestProviderOpaqueContinuationRoutesExactIdentityAndFailureKinds(t *testing.T) {
	t.Run("resumes exact reference", assertOpaqueContinuationResumes)
	t.Run("unsupported", assertOpaqueContinuationUnsupported)
	t.Run("foreign", assertOpaqueContinuationForeign)
	t.Run("unknown", assertOpaqueContinuationUnknown)
	t.Run("invalid", assertOpaqueContinuationInvalid)
	t.Run("nil service", assertOpaqueContinuationNil)
}
func newProviderContinuationCoverageRoot() *providerContinuationCoverageRoot {
	return &providerContinuationCoverageRoot{
		providerContractCoverageRoot: &providerContractCoverageRoot{
			identity:   providers.IDCodex,
			identities: map[string]providers.ID{"codex": providers.IDCodex, "claude": providers.IDClaude},
		},
		result: providers.ContinueResult{Outcome: providers.ContinuationOutcomeResumed, Result: providers.ExecuteResult{Content: "continued"}},
	}
}
func assertOpaqueContinuationResumes(t *testing.T) {
	t.Helper()
	root := newProviderContinuationCoverageRoot()
	continued, err := root.ContinueReference(context.Background(), providers.ContinueReferenceRequest{Reference: providers.ContinuationRef{Provider: "codex", Kind: "thread", ExternalRef: "external-session"}, Attempt: providers.ExecuteRequest{AttemptID: "attempt-continue"}})
	if err != nil || continued.Outcome != providers.ContinuationOutcomeResumed || continued.Result.Content != "continued" || continued.Reference.ExternalRef != "external-session" || root.request.Attempt.Provider != providers.IDCodex {
		t.Fatalf("ContinueReference() = %#v, %v; want exact resumed opaque identity", continued, err)
	}
	if cloned := continued.Clone(); cloned.Reference != continued.Reference || cloned.Result.Content != continued.Result.Content {
		t.Fatalf("ContinueReferenceResult.Clone() = %#v, want detached equivalent", cloned)
	}
}

func assertOpaqueContinuationUnsupported(t *testing.T) {
	t.Helper()
	root := &providerContractCoverageRoot{identity: providers.IDCodex}
	result, err := root.ContinueReference(context.Background(), providers.ContinueReferenceRequest{Reference: providers.ContinuationRef{Provider: "codex", Kind: "thread", ProviderSessionID: "session-unsupported"}, Attempt: providers.ExecuteRequest{AttemptID: "attempt-unsupported"}})
	if err != nil || result.Outcome != providers.ContinuationOutcomeUnsupported {
		t.Fatalf("ContinueReference(unsupported) = %#v, %v; want typed unsupported", result, err)
	}
}

func assertOpaqueContinuationForeign(t *testing.T) {
	t.Helper()
	root := newProviderContinuationCoverageRoot()
	result, err := root.ContinueReference(context.Background(), providers.ContinueReferenceRequest{Reference: providers.ContinuationRef{Provider: "codex", Kind: "thread", ProviderSessionID: "session-foreign"}, Attempt: providers.ExecuteRequest{Provider: providers.IDClaude, AttemptID: "attempt-foreign"}})
	var failure providers.ContinuationFailure
	if err == nil || !errors.Is(err, providers.ErrContinuationForeign) || result != (providers.ContinueReferenceResult{}) || !errors.As(err, &failure) || failure.Kind != providers.ContinuationFailureKindForeign {
		t.Fatalf("ContinueReference(foreign) = %#v, %v; want typed foreign failure", result, err)
	}
}

func assertOpaqueContinuationUnknown(t *testing.T) {
	t.Helper()
	root := &providerContractCoverageRoot{identity: providers.IDCodex, resolveErr: errors.New("unknown provider")}
	result, err := root.ContinueReference(context.Background(), providers.ContinueReferenceRequest{Reference: providers.ContinuationRef{Provider: "unknown", Kind: "thread", ProviderSessionID: "session-unknown"}, Attempt: providers.ExecuteRequest{AttemptID: "attempt-unknown"}})
	if err == nil || !errors.Is(err, providers.ErrContinuationForeign) || result != (providers.ContinueReferenceResult{}) {
		t.Fatalf("ContinueReference(unknown) = %#v, %v; want foreign failure", result, err)
	}
}

func assertOpaqueContinuationInvalid(t *testing.T) {
	t.Helper()
	root := newProviderContinuationCoverageRoot()
	result, err := root.ContinueReference(context.Background(), providers.ContinueReferenceRequest{Reference: providers.ContinuationRef{Kind: "thread", ExternalRef: "external-invalid"}})
	var failure providers.ContinuationFailure
	if err == nil || !errors.Is(err, providers.ErrInvalidContinuationRequest) || result != (providers.ContinueReferenceResult{}) || !errors.As(err, &failure) || failure.Reference.ID != "external-invalid" {
		t.Fatalf("ContinueReference(invalid) = %#v, %v; want invalid failure", result, err)
	}
}

func assertOpaqueContinuationNil(t *testing.T) {
	t.Helper()
	var root *providerContractCoverageRoot
	if _, err := root.ContinueReference(context.Background(), providers.ContinueReferenceRequest{Reference: providers.ContinuationRef{Provider: "codex", Kind: "thread", ProviderSessionID: "session-nil"}}); !errors.Is(err, providers.ErrInvalidContinuationRequest) {
		t.Fatalf("ContinueReference(nil service) = %v, want invalid continuation failure", err)
	}
}

type providerContractCoverageRoot struct {
	identity     providers.ID
	identities   map[string]providers.ID
	resolveErr   error
	executeErr   error
	executeCalls int
}

func (root *providerContractCoverageRoot) ListProviders(context.Context, providers.ListProvidersRequest) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (root *providerContractCoverageRoot) GetProvider(context.Context, providers.GetProviderRequest) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{Provider: providers.Descriptor{ID: root.canonicalIdentity()}}, nil
}

func (root *providerContractCoverageRoot) ResolveIdentity(_ context.Context, request providers.ResolveIdentityRequest) (providers.ResolveIdentityResult, error) {
	if root.resolveErr != nil {
		return providers.ResolveIdentityResult{}, root.resolveErr
	}
	if err := request.Validate(); err != nil {
		return providers.ResolveIdentityResult{}, err
	}
	if identity, ok := root.identities[strings.TrimSpace(request.Identity)]; ok {
		return providers.ResolveIdentityResult{ID: identity}, nil
	}
	return providers.ResolveIdentityResult{ID: root.canonicalIdentity()}, nil
}

func (root *providerContractCoverageRoot) ResolveSelection(context.Context, providers.ResolveSelectionRequest) (providers.ResolveSelectionResult, error) {
	return providers.ResolveSelectionResult{Provider: root.canonicalIdentity()}, nil
}

func (root *providerContractCoverageRoot) ValidatePrerequisites(context.Context, providers.ValidatePrerequisitesRequest) error {
	return nil
}

func (root *providerContractCoverageRoot) Execute(_ context.Context, _ providers.ExecuteRequest) (providers.ExecuteResult, error) {
	root.executeCalls++
	return providers.ExecuteResult{}, root.executeErr
}

func (root *providerContractCoverageRoot) ControlAttempt(_ context.Context, request providers.ControlAttemptRequest) (providers.ControlAttemptResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ControlAttemptResult{}, err
	}
	return providers.ControlAttemptResult{Provider: request.Provider, AttemptID: request.AttemptID, Action: request.Action, Outcome: providers.ControlOutcomeUnsupported}, nil
}

func (root *providerContractCoverageRoot) Continue(_ context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{Reference: request.Reference, Outcome: providers.ContinuationOutcomeUnsupported}, nil
}

func (root *providerContractCoverageRoot) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	if root == nil {
		return providers.ContinueReferenceResult{}, coverageContinuationFailure(providers.ContinuationFailureKindInvalid, "Providers service is required", request.Reference)
	}
	return coverageContinueReference(ctx, root, request)
}

func (root *providerContractCoverageRoot) canonicalIdentity() providers.ID {
	if root.identity == "" {
		return providers.IDCodex
	}
	return root.identity
}

type providerContinuationCoverageRoot struct {
	*providerContractCoverageRoot
	result  providers.ContinueResult
	request providers.ContinueRequest
}

func (root *providerContinuationCoverageRoot) Continue(_ context.Context, request providers.ContinueRequest) (providers.ContinueResult, error) {
	root.request = request.Clone()
	return root.result.Clone(), nil
}

func (root *providerContinuationCoverageRoot) ContinueReference(ctx context.Context, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	return coverageContinueReference(ctx, root, request)
}

func coverageContinueReference(ctx context.Context, service providers.Service, request providers.ContinueReferenceRequest) (providers.ContinueReferenceResult, error) {
	reference, err := request.Reference.ToSessionRef()
	if err != nil {
		return providers.ContinueReferenceResult{}, coverageContinuationFailure(providers.ContinuationFailureKindInvalid, err.Error(), request.Reference)
	}
	canonical, err := service.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: reference.Provider.String()})
	if err != nil {
		return providers.ContinueReferenceResult{}, coverageContinuationFailure(providers.ContinuationFailureKindForeign, err.Error(), request.Reference)
	}
	reference.Provider = canonical.ID
	attempt := request.Attempt.Clone()
	if strings.TrimSpace(attempt.Provider.String()) == "" {
		attempt.Provider = canonical.ID
	} else {
		attemptIdentity, resolveErr := service.ResolveIdentity(ctx, providers.ResolveIdentityRequest{Identity: attempt.Provider.String()})
		if resolveErr != nil || attemptIdentity.ID != canonical.ID {
			message := "attempt provider does not match continuation provider"
			if resolveErr != nil {
				message = resolveErr.Error()
			}
			return providers.ContinueReferenceResult{}, coverageContinuationFailure(providers.ContinuationFailureKindForeign, message, request.Reference)
		}
		attempt.Provider = canonical.ID
	}
	continued, err := service.Continue(ctx, providers.ContinueRequest{Reference: reference, Attempt: attempt})
	if err != nil {
		return providers.ContinueReferenceResult{}, err
	}
	continuedReference := continued.Reference
	if continuedReference.Provider == "" {
		continuedReference = reference
	}
	resultReference := continuedReference.ContinuationRef()
	resultReference.ExternalRef = request.Reference.Normalize().ExternalRef
	return providers.ContinueReferenceResult{Reference: resultReference, Outcome: continued.Outcome, Result: continued.Result}, nil
}

func coverageContinuationFailure(kind providers.ContinuationFailureKind, message string, ref providers.ContinuationRef) providers.ContinuationFailure {
	normalized := ref.Normalize()
	identity := strings.TrimSpace(normalized.ProviderSessionID)
	if identity == "" {
		identity = strings.TrimSpace(normalized.ExternalRef)
	}
	return providers.ContinuationFailure{Kind: kind, Message: message, Reference: providers.SessionRef{Provider: providers.ID(normalized.Provider), Kind: normalized.Kind, ID: identity}}
}
