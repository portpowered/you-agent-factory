package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	if _, err := New(nil, noopPublisher); err == nil {
		t.Fatal("New(nil publisher) error = nil, want missing Providers service")
	}
	if _, err := New(&providersFake{}, nil); err == nil {
		t.Fatal("New(nil publish) error = nil, want missing progress publisher")
	}
}

func TestExecuteForwardsEnvThroughProviderRequest(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID:      "dispatch-env-1",
			WorkerType:      "goal-executor",
			WorkstationName: "execute-goal",
		},
		RunnerID:           string(providers.IDCodex),
		WorkerType:         "goal-executor",
		WorkstationType:    "execute-goal",
		SystemPrompt:       "system",
		UserMessage:        "user",
		EnvVars:            map[string]string{"FIXTURE": "configured"},
		ProcessEnvironment: []string{"FIXTURE=configured"},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := providers.ExecuteRequest{
		Provider:           providers.IDCodex,
		AttemptID:          "dispatch-env-1",
		WorkerType:         "goal-executor",
		WorkstationName:    "execute-goal",
		SystemPrompt:       "system",
		UserMessage:        "user",
		EnvVars:            map[string]string{"FIXTURE": "configured"},
		ProcessEnvironment: []string{"FIXTURE=configured"},
	}
	got := fake.request
	observer := got.SessionObserver
	progressObserver := got.ProgressObserver
	got.SessionObserver = nil
	got.ProgressObserver = nil
	if observer == nil {
		t.Fatal("Providers.Execute request SessionObserver = nil, want live provider session observation")
	}
	if progressObserver == nil {
		t.Fatal("Providers.Execute request ProgressObserver = nil, want live provider progress observation")
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Providers.Execute request = %#v, want %#v", got, want)
	}
}

func TestExecutePublishesLiveProviderSessionObservationBeforeProviderReturns(t *testing.T) {
	t.Parallel()

	reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "live-session-1"}
	fake := &observingProvidersFake{reference: reference}
	var published []workers.ProgressFragment
	runner, err := New(fake, func(fragment workers.ProgressFragment) {
		published = append(published, fragment)
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := runner.Execute(t.Context(), baseAgentRequest()); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !fake.observedBeforeReturn {
		t.Fatal("Provider session observation was not delivered while Providers.Execute was live")
	}
	if len(published) < 2 || published[0].Kind != workers.ProviderSessionObservedFragmentKind ||
		published[0].ProviderSessionReference == nil || *published[0].ProviderSessionReference != reference {
		t.Fatalf("published observations = %#v, want exact live session observation before output", published)
	}
}

func TestExecuteCanonicalizesTimeoutAndUnknownFailureMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		failure providers.ExecuteFailure
		wantMsg string
	}{
		{
			name: "timeout execution timeout message",
			failure: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindTimeout,
				Message: "execution timeout",
			},
			wantMsg: agentTimeoutFailureMessage,
		},
		{
			name: "timeout empty message",
			failure: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindTimeout,
			},
			wantMsg: agentTimeoutFailureMessage,
		},
		{
			name: "timeout provider-specific message",
			failure: providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindTimeout,
				Message: "Cursor request timed out.",
			},
			wantMsg: agentTimeoutFailureMessage,
		},
		{
			name: "unknown empty message",
			failure: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindUnknown,
			},
			wantMsg: "provider invocation failed",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &failingProvidersFake{failure: test.failure}
			runner, err := New(fake, noopPublisher)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			_, err = runner.Execute(t.Context(), baseAgentRequest())
			var providerErr *workers.ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
			}
			if providerErr.Message != test.wantMsg {
				t.Fatalf("ProviderError.Message = %q, want %q", providerErr.Message, test.wantMsg)
			}
		})
	}
}

func TestExecuteFailurePreservesSessionRefAndBoundsMessages(t *testing.T) {
	t.Parallel()

	longMessage := strings.Repeat("x", failureMessageRuneLimit+32)
	failure := providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindUnknown,
		Message: longMessage,
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "failure-session-1",
		},
	}
	fake := &failingProvidersFake{failure: failure}
	var published []workers.ProgressFragment
	runner, err := New(fake, func(fragment workers.ProgressFragment) { published = append(published, fragment) })
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.SessionID = "resume-session-1"
	result, err := runner.Execute(t.Context(), request)
	if err == nil {
		t.Fatal("Execute() error = nil, want provider failure")
	}
	if len(published) != 1 {
		t.Fatalf("progress publications = %d, want terminal failure", len(published))
	}
	if published[0].Kind != workers.FailedFragmentKind || published[0].Payload != longMessage[:failureMessageRuneLimit] {
		t.Fatalf("terminal failure publication = %#v", published[0])
	}
	wantSession := &workers.ProviderSessionMetadata{
		Provider: string(providers.IDCodex),
		Kind:     providers.SessionIDKind,
		ID:       "failure-session-1",
	}
	if !reflect.DeepEqual(result.ProviderSession, wantSession) {
		t.Fatalf("ProviderSession = %#v, want %#v", result.ProviderSession, wantSession)
	}
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if len([]rune(providerErr.Message)) != failureMessageRuneLimit {
		t.Fatalf("ProviderError.Message length = %d, want %d runes", len([]rune(providerErr.Message)), failureMessageRuneLimit)
	}
	if !reflect.DeepEqual(providerErr.ProviderSession, wantSession) {
		t.Fatalf("ProviderError.ProviderSession = %#v, want failure session %#v", providerErr.ProviderSession, wantSession)
	}
}

func TestExecuteFailureAcceptsPointerExecuteFailure(t *testing.T) {
	t.Parallel()

	failure := providers.ExecuteFailure{
		Kind:    providers.ExecuteFailureKindThrottled,
		Message: "rate limited",
	}
	fake := &pointerFailureProvidersFake{failure: &failure}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = runner.Execute(t.Context(), baseAgentRequest())
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypeThrottled {
		t.Fatalf("ProviderError.Type = %q, want throttled", providerErr.Type)
	}
}

func TestExecuteResumesThroughContinueWhenSessionIDPresent(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.SessionID = "resume-session-2"
	if _, err := runner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	wantReference := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     providers.SessionIDKind,
		ID:       "resume-session-2",
	}
	if fake.continuationReference == nil || *fake.continuationReference != wantReference {
		t.Fatalf("Providers.Continue Reference = %#v, want %#v", fake.continuationReference, wantReference)
	}
}

func TestExecuteResumesThroughExactWorkerSessionReferenceWithoutLegacyReconstruction(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	exact := providers.SessionRef{
		Provider: providers.IDCodex,
		Kind:     "provider-native-thread",
		ID:       "opaque-provider-session",
	}
	wantReference := exact.Clone()
	request := baseAgentRequest()
	request.SessionID = "legacy-session-that-must-not-be-used"
	request.RunnerID = string(providers.IDClaude)
	request.ResumeSession = &exact
	if _, err := runner.Execute(t.Context(), request); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if fake.continuationReference == nil || *fake.continuationReference != wantReference {
		t.Fatalf("Providers.Continue Reference = %#v, want exact %#v", fake.continuationReference, wantReference)
	}
	if fake.request.Provider != exact.Provider {
		t.Fatalf("Providers.Continue Attempt.Provider = %q, want exact reference provider %q", fake.request.Provider, exact.Provider)
	}
	request.ResumeSession.ID = "caller-mutated"
	if fake.continuationReference.ID != wantReference.ID {
		t.Fatalf("Providers.Continue retained caller ResumeSession mutation: %#v", fake.continuationReference)
	}
	if fake.executeCalls != 0 {
		t.Fatalf("Providers.Execute calls = %d, want 0 for exact continuation", fake.executeCalls)
	}
}

func TestExecuteUnsupportedContinuationReturnsProviderError(t *testing.T) {
	t.Parallel()

	fake := &unsupportedContinuationProvidersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	request := baseAgentRequest()
	request.SessionID = "resume-session-unsupported"
	_, err = runner.Execute(t.Context(), request)
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypePermanentBadRequest {
		t.Fatalf("ProviderError.Type = %q, want %q", providerErr.Type, workers.WorkFailureTypePermanentBadRequest)
	}
	if fake.executeCalls != 0 {
		t.Fatalf("Providers.Execute calls = %d, want 0 (unsupported continuation must not fall back to a fresh attempt)", fake.executeCalls)
	}
}

// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-function-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func TestExecuteExactContinuationFailurePreservesClassificationWithoutFallback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                        string
		reference                   providers.SessionRef
		continuationErr             error
		continuationOutcome         providers.ContinuationOutcome
		wantProviderFailureKind     providers.ExecuteFailureKind
		wantContinuationFailureKind providers.ContinuationFailureKind
		wantContinuationOutcome     providers.ContinuationOutcome
		wantError                   error
	}{
		{
			name: "malformed reference",
			reference: providers.SessionRef{
				Provider: providers.IDCodex,
				ID:       "opaque-malformed-session",
			},
			wantContinuationFailureKind: providers.ContinuationFailureKindInvalid,
			wantError:                   providers.ErrInvalidContinuationRequest,
		},
		{
			name: "unsupported provider session kind",
			reference: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     "provider-native-thread",
				ID:       "opaque-unsupported-kind",
			},
			continuationErr: providers.ContinuationFailure{
				Kind: providers.ContinuationFailureKindInvalid,
			},
			wantContinuationFailureKind: providers.ContinuationFailureKindInvalid,
			wantError:                   providers.ErrInvalidContinuationRequest,
		},
		{
			name: "foreign provider reference",
			reference: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "opaque-foreign-session",
			},
			continuationErr: providers.ContinuationFailure{
				Kind: providers.ContinuationFailureKindForeign,
			},
			wantContinuationFailureKind: providers.ContinuationFailureKindForeign,
			wantError:                   providers.ErrContinuationForeign,
		},
		{
			name: "stale provider session",
			reference: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "opaque-stale-session",
			},
			continuationErr: providers.ContinuationFailure{
				Kind: providers.ContinuationFailureKindStale,
			},
			wantContinuationFailureKind: providers.ContinuationFailureKindStale,
			wantError:                   providers.ErrContinuationStale,
		},
		{
			name: "unsupported continuation capability",
			reference: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "opaque-unsupported-session",
			},
			continuationOutcome:     providers.ContinuationOutcomeUnsupported,
			wantContinuationOutcome: providers.ContinuationOutcomeUnsupported,
		},
		{
			name: "provider operational failure",
			reference: providers.SessionRef{
				Provider: providers.IDCodex,
				Kind:     providers.SessionIDKind,
				ID:       "opaque-operational-session",
			},
			continuationErr: providers.ExecuteFailure{
				Kind: providers.ExecuteFailureKindDependency,
			},
			wantProviderFailureKind: providers.ExecuteFailureKindDependency,
			wantError:               providers.ErrExecuteFailed,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			fake := &providersFake{
				continuationErr:     test.continuationErr,
				continuationOutcome: test.continuationOutcome,
			}
			runner, err := New(fake, noopPublisher)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			request := baseAgentRequest()
			reference := test.reference.Clone()
			request.ResumeSession = &reference
			_, err = runner.Execute(t.Context(), request)
			var providerErr *workers.ProviderError
			if !errors.As(err, &providerErr) {
				t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
			}
			if test.wantError != nil && !errors.Is(err, test.wantError) {
				t.Fatalf("Execute() error = %v, want %v", err, test.wantError)
			}
			if providerErr.ProviderFailureKind != test.wantProviderFailureKind ||
				providerErr.ProviderContinuationFailureKind != test.wantContinuationFailureKind ||
				providerErr.ProviderContinuationOutcome != test.wantContinuationOutcome {
				t.Fatalf("ProviderError continuation classification = %#v", providerErr)
			}
			if fake.executeCalls != 0 || fake.continueCalls != 1 {
				t.Fatalf("Providers calls execute=%d continue=%d, want execute=0 continue=1", fake.executeCalls, fake.continueCalls)
			}
		})
	}
}

func TestExecuteExactContinuationRejectsMismatchedProviderResultBeforePublishingOutput(t *testing.T) {
	tests := []struct {
		name                  string
		continuationReference *providers.SessionRef
		resultReference       *providers.SessionRef
	}{
		{
			name:                  "continuation envelope reference",
			continuationReference: &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "foreign-session"},
		},
		{
			name:            "execute result reference",
			resultReference: &providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "foreign-session"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &providersFake{
				continuationResponseReference: test.continuationReference,
				continuationResultReference:   test.resultReference,
			}
			var published []workers.ProgressFragment
			runner, err := New(fake, func(fragment workers.ProgressFragment) { published = append(published, fragment) })
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			reference := providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: "expected-session"}
			request := baseAgentRequest()
			request.ResumeSession = &reference
			result, err := runner.Execute(t.Context(), request)
			var providerErr *workers.ProviderError
			if !errors.As(err, &providerErr) || providerErr.ProviderContinuationFailureKind != providers.ContinuationFailureKindInvalid {
				t.Fatalf("Execute() error = %#v, want invalid continuation ProviderError", err)
			}
			if result.Content != "" || result.ProviderSession == nil || result.ProviderSession.ID != reference.ID {
				t.Fatalf("Execute() result = %#v, want no content and retained exact session", result)
			}
			if fake.executeCalls != 0 || fake.continueCalls != 1 {
				t.Fatalf("provider calls = execute:%d continue:%d, want execute:0 continue:1", fake.executeCalls, fake.continueCalls)
			}
			if len(published) != 1 || published[0].Kind != workers.FailedFragmentKind || published[0].Payload == "ok" {
				t.Fatalf("published progress = %#v, want one safe failure without successful output", published)
			}
		})
	}
}

func TestExecuteForwardsInputTokensToProviders(t *testing.T) {
	t.Parallel()

	fake := &providersFake{}
	runner, err := New(fake, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tokens := []any{"image-token"}
	_, err = runner.Execute(t.Context(), workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-capability",
		},
		RunnerID:    string(providers.IDCodex),
		UserMessage: "user",
		InputTokens: tokens,
		RequiredOptionalCapabilities: []workers.RunnerOptionalCapability{
			workers.RunnerOptionalCapabilityImageInput,
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(fake.request.InputTokens) != 1 || fake.request.InputTokens[0] != "image-token" {
		t.Fatalf("Providers.ExecuteRequest.InputTokens = %#v", fake.request.InputTokens)
	}
}

// recordingDecisionEnvelopeService captures what the Agent Runner hands the
// Factory Definitions owner and returns a scripted canonical WorkResult. The
// runner must copy that result verbatim: Workers owns no envelope vocabulary,
// no decision validation, and no malformed-envelope failure policy of its own.
type recordingDecisionEnvelopeService struct {
	mu           sync.Mutex
	result       workers.WorkResult
	goalResult   workers.WorkResult
	calls        []decisionEnvelopeCall
	goalRequests int
}

func (service *recordingDecisionEnvelopeService) UsesDecisionEnvelopeOutcome(
	*interfaces.FactoryWorkstationConfig,
) bool {
	return true
}

func (service *recordingDecisionEnvelopeService) UsesGoalRoutingDecisionEnvelope(
	*interfaces.FactoryWorkstationConfig,
) bool {
	return false
}

type decisionEnvelopeCall struct {
	dispatchID   string
	transitionID string
	raw          string
	goal         bool
}

func (service *recordingDecisionEnvelopeService) WorkResultFromDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workers.WorkResult {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.calls = append(service.calls, decisionEnvelopeCall{
		dispatchID:   dispatchID,
		transitionID: transitionID,
		raw:          raw,
	})
	return service.result
}

func (service *recordingDecisionEnvelopeService) WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(
	dispatchID string,
	transitionID string,
	raw string,
) workers.WorkResult {
	service.mu.Lock()
	defer service.mu.Unlock()
	service.goalRequests++
	service.calls = append(service.calls, decisionEnvelopeCall{
		dispatchID:   dispatchID,
		transitionID: transitionID,
		raw:          raw,
		goal:         true,
	})
	return service.goalResult
}

func (service *recordingDecisionEnvelopeService) snapshot() []decisionEnvelopeCall {
	service.mu.Lock()
	defer service.mu.Unlock()
	return append([]decisionEnvelopeCall(nil), service.calls...)
}

func TestExecuteNormalizesAgentOutcomePolicies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		content      string
		decision     bool
		goalDecision bool
		outputFormat string
		stopToken    string
		envelope     workers.WorkResult
		wantOutcome  workers.WorkOutcome
		wantClass    string
		wantContent  string
		wantFeedback string
	}{
		{
			name:     "decision accepted",
			content:  `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`,
			decision: true,
			envelope: workers.WorkResult{
				Outcome:  workers.OutcomeAccepted,
				Feedback: "ready",
				Output:   "ship",
			},
			wantOutcome:  workers.OutcomeAccepted,
			wantContent:  "ship",
			wantFeedback: "ready",
		},
		{
			name:         "decision continue through output format",
			content:      `{"decision":"CONTINUE","feedback":"add tests","output":"next"}`,
			outputFormat: "decision-envelope",
			envelope: workers.WorkResult{
				Outcome:  workers.OutcomeContinue,
				Feedback: "add tests",
				Output:   "next",
			},
			wantOutcome:  workers.OutcomeContinue,
			wantContent:  "next",
			wantFeedback: "add tests",
		},
		{
			name:         "goal needs changes",
			content:      `{"decision":"NEEDS-CHANGES","feedback":"revise","output":"hold"}`,
			decision:     true,
			goalDecision: true,
			envelope: workers.WorkResult{
				Outcome:                     workers.OutcomeAccepted,
				SelectedClassificationLabel: "needs_changes",
				Feedback:                    "revise",
				Output:                      "hold",
			},
			wantOutcome:  workers.OutcomeAccepted,
			wantClass:    "needs_changes",
			wantContent:  "hold",
			wantFeedback: "revise",
		},
		{
			name:        "stop token accepted",
			content:     "finished: DONE",
			stopToken:   "DONE",
			wantOutcome: workers.OutcomeAccepted,
			wantContent: "finished: DONE",
		},
		{
			name:        "continue marker",
			content:     "please review <CONTINUE>",
			stopToken:   "DONE",
			wantOutcome: workers.OutcomeContinue,
			wantContent: "please review <CONTINUE>",
		},
		{
			name:        "missing stop token rejects",
			content:     "unfinished",
			stopToken:   "DONE",
			wantOutcome: workers.OutcomeRejected,
			wantContent: "unfinished",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			envelopes := &recordingDecisionEnvelopeService{
				result:     test.envelope,
				goalResult: test.envelope,
			}
			runner, err := New(&providersFake{content: test.content}, noopPublisher, envelopes)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			request := baseAgentRequest()
			request.DecisionEnvelope = test.decision
			request.GoalRoutingDecisionEnvelope = test.goalDecision
			request.OutputFormat = test.outputFormat
			request.StopToken = test.stopToken

			result, err := runner.Execute(t.Context(), request)
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if result.Outcome != test.wantOutcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome, test.wantOutcome)
			}
			if result.Classification != test.wantClass {
				t.Fatalf("classification = %q, want %q", result.Classification, test.wantClass)
			}
			if result.Content != test.wantContent {
				t.Fatalf("content = %q, want %q", result.Content, test.wantContent)
			}
			if result.Feedback != test.wantFeedback {
				t.Fatalf("feedback = %q, want %q", result.Feedback, test.wantFeedback)
			}
		})
	}
}

// TestExecuteDelegatesDecisionEnvelopeInterpretationToDefinitions pins the
// consolidation: the Agent Runner hands the raw worker response to the injected
// Factory Definitions owner, selects the goal-routing twin only when the
// request declares goal routing, and carries the owner's recorded work back
// onto the runner result.
func TestExecuteDelegatesDecisionEnvelopeInterpretationToDefinitions(t *testing.T) {
	t.Parallel()

	raw := `{"decision":"ACCEPTED","feedback":"ok","output":"done",` +
		`"recorded_output_work":[{"workTypeId":"reviewable-work","state":"init"}]}`
	recorded := []work.FactoryWorkItem{{WorkTypeID: "reviewable-work", State: "init"}}
	envelopes := &recordingDecisionEnvelopeService{
		result: workers.WorkResult{
			Outcome:            workers.OutcomeAccepted,
			Feedback:           "ok",
			Output:             "done",
			RecordedOutputWork: recorded,
		},
	}
	runner, err := New(&providersFake{content: raw}, noopPublisher, envelopes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := baseAgentRequest()
	request.Dispatch.TransitionID = "review"
	request.DecisionEnvelope = true

	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	calls := envelopes.snapshot()
	if len(calls) != 1 {
		t.Fatalf("decision-envelope calls = %d, want exactly one delegated interpretation", len(calls))
	}
	if calls[0].goal {
		t.Fatal("Execute() used the goal-routing twin for a request that does not declare goal routing")
	}
	if calls[0].raw != raw {
		t.Fatalf("delegated raw output = %q, want the unparsed worker response", calls[0].raw)
	}
	if calls[0].dispatchID != request.Dispatch.DispatchID || calls[0].transitionID != "review" {
		t.Fatalf("delegated identity = %q/%q, want the dispatch identity", calls[0].dispatchID, calls[0].transitionID)
	}
	if !reflect.DeepEqual(result.RecordedOutputWork, recorded) {
		t.Fatalf("RecordedOutputWork = %#v, want the owner's recorded work", result.RecordedOutputWork)
	}
}

func TestExecuteRoutesGoalDecisionEnvelopeToGoalRoutingOwner(t *testing.T) {
	t.Parallel()

	envelopes := &recordingDecisionEnvelopeService{
		goalResult: workers.WorkResult{
			Outcome:                     workers.OutcomeAccepted,
			SelectedClassificationLabel: "tests_failed",
		},
	}
	runner, err := New(&providersFake{content: `{"decision":"TESTS_FAILED"}`}, noopPublisher, envelopes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := baseAgentRequest()
	request.DecisionEnvelope = true
	request.GoalRoutingDecisionEnvelope = true

	result, err := runner.Execute(t.Context(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if envelopes.goalRequests != 1 {
		t.Fatalf("goal-routing interpretations = %d, want exactly one", envelopes.goalRequests)
	}
	if result.Classification != "tests_failed" {
		t.Fatalf("classification = %q, want the owner's goal routing label", result.Classification)
	}
}

// TestExecuteKeepsCanonicalMalformedDecisionEnvelopeFailure pins the second
// divergence the runner-local envelope parser introduced: a malformed envelope
// must keep the owner's MalformedEnvelopeFailureOutcome and its
// completion_validation / missing_required_output diagnostics instead of being
// restated as a Workers-local PERMANENT_BAD_REQUEST with no diagnostics.
func TestExecuteKeepsCanonicalMalformedDecisionEnvelopeFailure(t *testing.T) {
	t.Parallel()

	envelopes := &recordingDecisionEnvelopeService{
		result: workers.WorkResult{
			Outcome:  workers.OutcomeFailed,
			Error:    "reviewer decision envelope invalid: decision envelope: invalid JSON",
			Feedback: "partial feedback",
			FailureMetadata: &workers.WorkFailureMetadata{
				Family: workers.WorkFailureFamilyTerminal,
				Type:   workers.WorkFailureTypeUnknown,
			},
			Diagnostics: &workers.WorkDiagnostics{
				Provider: &workers.ProviderDiagnostic{
					ResponseMetadata: map[string]string{
						workers.ProviderResponseMetadataFailureFamily:         string(workers.WorkFailureFamilyTerminal),
						workers.ProviderResponseMetadataFailureType:           string(workers.WorkFailureTypeUnknown),
						workers.ProviderResponseMetadataFailureOperation:      "completion_validation",
						workers.ProviderResponseMetadataFailureClassification: "missing_required_output",
					},
				},
			},
		},
	}
	runner, err := New(&diagnosticProvidersFake{content: "not-json"}, noopPublisher, envelopes)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := baseAgentRequest()
	request.DecisionEnvelope = true

	result, err := runner.Execute(t.Context(), request)
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type == workers.WorkFailureTypePermanentBadRequest {
		t.Fatal("Execute() restated the malformed envelope as a Workers-local permanent bad request")
	}
	if providerErr.Type != workers.WorkFailureTypeUnknown ||
		providerErr.Family != workers.WorkFailureFamilyTerminal {
		t.Fatalf("ProviderError = %#v, want the owner's terminal unknown classification", providerErr)
	}
	if !strings.Contains(providerErr.Message, "reviewer decision envelope invalid") {
		t.Fatalf("ProviderError.Message = %q, want the owner's malformed-envelope message", providerErr.Message)
	}
	if result.Outcome != workers.OutcomeFailed {
		t.Fatalf("Outcome = %q, want the canonical malformed-envelope failure outcome", result.Outcome)
	}
	if result.Content != "not-json" {
		t.Fatalf("Content = %q, want the unreadable worker response preserved", result.Content)
	}
	if result.Feedback != "partial feedback" {
		t.Fatalf("Feedback = %q, want the owner's partial feedback", result.Feedback)
	}
	metadata := map[string]string{}
	if result.Diagnostics != nil && result.Diagnostics.Provider != nil {
		metadata = result.Diagnostics.Provider.ResponseMetadata
	}
	if metadata[workers.ProviderResponseMetadataFailureOperation] != "completion_validation" ||
		metadata[workers.ProviderResponseMetadataFailureClassification] != "missing_required_output" {
		t.Fatalf(
			"diagnostics response metadata = %#v, want completion_validation/missing_required_output",
			metadata,
		)
	}
	if metadata["provider_fact"] != "kept" {
		t.Fatalf(
			"diagnostics response metadata = %#v, want the provider attempt facts preserved alongside the envelope verdict",
			metadata,
		)
	}
}

// diagnosticProvidersFake reports provider-owned diagnostics so the envelope
// verdict can be observed layered over, not replacing, the attempt's facts.
type diagnosticProvidersFake struct {
	providers.Service
	content string
}

func (fake *diagnosticProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{
		Content:     fake.content,
		Diagnostics: &providers.ExecuteDiagnostics{Metadata: map[string]string{"provider_fact": "kept"}},
	}, nil
}

func TestExecuteRejectsDecisionEnvelopeWithoutInjectedOwner(t *testing.T) {
	t.Parallel()

	runner, err := New(&providersFake{content: `{"decision":"ACCEPTED"}`}, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	request := baseAgentRequest()
	request.DecisionEnvelope = true

	if _, err := runner.Execute(t.Context(), request); err == nil {
		t.Fatal("Execute() error = nil, want a misconfiguration instead of a Workers-local envelope parser")
	}
}

func TestExecuteRejectsInvalidRequestsBeforeProviderCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*workers.RunnerExecutionRequest)
	}{
		{name: "provider identity", mutate: func(request *workers.RunnerExecutionRequest) { request.RunnerID = "" }},
		{name: "dispatch identity", mutate: func(request *workers.RunnerExecutionRequest) { request.Dispatch.DispatchID = "" }},
		{name: "prompt", mutate: func(request *workers.RunnerExecutionRequest) {
			request.SystemPrompt = ""
			request.UserMessage = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			fake := &providersFake{}
			runner, err := New(fake, noopPublisher)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			request := baseAgentRequest()
			test.mutate(&request)
			if _, err := runner.Execute(t.Context(), request); err == nil {
				t.Fatal("Execute() error = nil, want validation failure")
			}
			if fake.executeCalls != 0 || fake.continueCalls != 0 {
				t.Fatalf("provider calls execute=%d continue=%d, want no effect", fake.executeCalls, fake.continueCalls)
			}
		})
	}
}

func TestExecuteNormalizesUnexpectedProviderError(t *testing.T) {
	t.Parallel()

	runner, err := New(&genericErrorProvidersFake{}, noopPublisher)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = runner.Execute(t.Context(), baseAgentRequest())
	var providerErr *workers.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Execute() error = %v, want *workers.ProviderError", err)
	}
	if providerErr.Type != workers.WorkFailureTypeInternalServerError || providerErr.Message != "provider exploded" {
		t.Fatalf("ProviderError = %#v, want bounded internal provider failure", providerErr)
	}
}

func TestFailureTypeForProviderKindPreservesProviderClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind providers.ExecuteFailureKind
		want workers.WorkFailureType
	}{
		{kind: providers.ExecuteFailureKindAuthentication, want: workers.WorkFailureTypeAuthFailure},
		{kind: providers.ExecuteFailureKindInvalidRequest, want: workers.WorkFailureTypePermanentBadRequest},
		{kind: providers.ExecuteFailureKindCapabilityMismatch, want: workers.WorkFailureTypePermanentBadRequest},
		{kind: providers.ExecuteFailureKindMisconfigured, want: workers.WorkFailureTypeMisconfigured},
		{kind: providers.ExecuteFailureKindThrottled, want: workers.WorkFailureTypeThrottled},
		{kind: providers.ExecuteFailureKindDependency, want: workers.WorkFailureTypeInternalServerError},
		{kind: providers.ExecuteFailureKindTimeout, want: workers.WorkFailureTypeTimeout},
		{kind: providers.ExecuteFailureKindUnknown, want: workers.WorkFailureTypeUnknown},
	}
	for _, test := range tests {
		if got := failureTypeForProviderKind(test.kind); got != test.want {
			t.Errorf("failureTypeForProviderKind(%q) = %q, want %q", test.kind, got, test.want)
		}
	}
}

func baseAgentRequest() workers.RunnerExecutionRequest {
	return workers.RunnerExecutionRequest{
		Dispatch: work.WorkDispatch{
			DispatchID: "dispatch-agent-1",
		},
		RunnerID:     string(providers.IDCodex),
		SystemPrompt: "system",
		UserMessage:  "user",
	}
}

type providersFake struct {
	providers.Service
	content                       string
	request                       providers.ExecuteRequest
	continuationReference         *providers.SessionRef
	continuationResponseReference *providers.SessionRef
	continuationResultReference   *providers.SessionRef
	continuationErr               error
	continuationOutcome           providers.ContinuationOutcome
	executeCalls                  int
	continueCalls                 int
}

type observingProvidersFake struct {
	providers.Service
	reference            providers.SessionRef
	observedBeforeReturn bool
}

type failingProvidersFake struct {
	providers.Service
	failure providers.ExecuteFailure
}

type pointerFailureProvidersFake struct {
	providers.Service
	failure *providers.ExecuteFailure
}

type unsupportedContinuationProvidersFake struct {
	providers.Service
	executeCalls int
}

func noopPublisher(workers.ProgressFragment) {}

func (fake *providersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeCalls++
	fake.request = request.Clone()
	content := fake.content
	if content == "" {
		content = "ok"
	}
	return providers.ExecuteResult{Content: content}, nil
}

type genericErrorProvidersFake struct {
	providers.Service
}

func (*genericErrorProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, errors.New("provider exploded")
}

func (fake *observingProvidersFake) Execute(
	_ context.Context,
	request providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	request.ObserveSession(fake.reference)
	fake.observedBeforeReturn = true
	return providers.ExecuteResult{Content: "ok", SessionRef: workers.CloneProviderSessionReference(&fake.reference)}, nil
}

func (fake *observingProvidersFake) Continue(
	context.Context,
	providers.ContinueRequest,
) (providers.ContinueResult, error) {
	return providers.ContinueResult{}, errors.New("unexpected continuation")
}

func (*observingProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*observingProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *providersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	fake.continueCalls++
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	if fake.continuationErr != nil {
		return providers.ContinueResult{}, fake.continuationErr
	}
	fake.request = request.Attempt.Clone()
	reference := request.Reference.Clone()
	fake.continuationReference = &reference
	outcome := fake.continuationOutcome
	if outcome == "" {
		outcome = providers.ContinuationOutcomeResumed
	}
	responseReference := request.Reference.Clone()
	if fake.continuationResponseReference != nil {
		responseReference = fake.continuationResponseReference.Clone()
	}
	result := providers.ExecuteResult{Content: "ok"}
	if fake.continuationResultReference != nil {
		reference := fake.continuationResultReference.Clone()
		result.SessionRef = &reference
	}
	return providers.ContinueResult{
		Reference: responseReference,
		Outcome:   outcome,
		Result:    result,
	}, nil
}

func (*providersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*providersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *failingProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, fake.failure
}

func (fake *failingProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{}, fake.failure
}

func (fake *failingProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *failingProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *pointerFailureProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	return providers.ExecuteResult{}, fake.failure
}

func (fake *pointerFailureProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (fake *pointerFailureProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}

func (fake *unsupportedContinuationProvidersFake) Execute(
	context.Context,
	providers.ExecuteRequest,
) (providers.ExecuteResult, error) {
	fake.executeCalls++
	return providers.ExecuteResult{Content: "ok"}, nil
}

func (fake *unsupportedContinuationProvidersFake) Continue(
	_ context.Context,
	request providers.ContinueRequest,
) (providers.ContinueResult, error) {
	if err := request.Validate(); err != nil {
		return providers.ContinueResult{}, err
	}
	return providers.ContinueResult{
		Reference: request.Reference,
		Outcome:   providers.ContinuationOutcomeUnsupported,
	}, nil
}

func (*unsupportedContinuationProvidersFake) ListProviders(
	context.Context,
	providers.ListProvidersRequest,
) (providers.ListProvidersResult, error) {
	return providers.ListProvidersResult{}, nil
}

func (*unsupportedContinuationProvidersFake) GetProvider(
	context.Context,
	providers.GetProviderRequest,
) (providers.GetProviderResult, error) {
	return providers.GetProviderResult{}, nil
}
