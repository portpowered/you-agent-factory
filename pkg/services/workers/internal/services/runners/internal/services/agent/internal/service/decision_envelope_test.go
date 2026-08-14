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

type agentOutcomePolicyTest struct {
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
}

func agentOutcomePolicyTests() []agentOutcomePolicyTest {
	return []agentOutcomePolicyTest{
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
}

func TestExecuteNormalizesAgentOutcomePolicies(t *testing.T) {
	t.Parallel()

	for _, test := range agentOutcomePolicyTests() {
		test := test
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
