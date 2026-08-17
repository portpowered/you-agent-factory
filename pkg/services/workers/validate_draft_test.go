package workers

import (
	"encoding/json"
	"errors"
	"testing"
)

// legalKindPhasePairs mirrors the pairs declared by the existing internal
// draft-validation policy owner (allowedPhasesByKind in
// pkg/services/workers/internal/draftvalidation). It is
// duplicated here only as fixture data for exercising the public boundary;
// the policy itself has exactly one owner.
func legalKindPhasePairs() []struct {
	Kind  Kind
	Phase Phase
} {
	return []struct {
		Kind  Kind
		Phase Phase
	}{
		{KindSession, PhaseStarted}, {KindSession, PhaseUpdated}, {KindSession, PhaseCompleted}, {KindSession, PhaseFailed}, {KindSession, PhaseCanceled},
		{KindRun, PhaseStarted}, {KindRun, PhaseCompleted}, {KindRun, PhaseFailed}, {KindRun, PhaseCanceled},
		{KindTurn, PhaseStarted}, {KindTurn, PhaseCompleted}, {KindTurn, PhaseFailed}, {KindTurn, PhaseCanceled},
		{KindMessage, PhaseStarted}, {KindMessage, PhaseDelta}, {KindMessage, PhaseCompleted},
		{KindReasoning, PhaseStarted}, {KindReasoning, PhaseDelta}, {KindReasoning, PhaseCompleted},
		{KindTool, PhaseStarted}, {KindTool, PhaseDelta}, {KindTool, PhaseCompleted}, {KindTool, PhaseFailed}, {KindTool, PhaseCanceled},
		{KindFileChange, PhaseUpdated},
		{KindPlan, PhaseUpdated},
		{KindProgress, PhaseUpdated},
		{KindUsage, PhaseUpdated},
		{KindError, PhaseUpdated}, {KindError, PhaseFailed},
		{KindStreamGap, PhaseUpdated},
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func sampleDraftPayload(t *testing.T, kind Kind, phase Phase) json.RawMessage {
	t.Helper()

	if kind == KindMessage && phase == PhaseDelta {
		return json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`)
	}
	if kind == KindMessage {
		return json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"hello"}]}`)
	}
	if kind == KindTool && phase == PhaseDelta {
		return json.RawMessage(`{"toolCallId":"call-1","outputDelta":"partial"}`)
	}
	if kind == KindTool {
		return json.RawMessage(`{"toolCallId":"call-1","toolName":"read_file","status":"started"}`)
	}

	switch kind {
	case KindSession:
		return json.RawMessage(`{"status":"active"}`)
	case KindRun:
		return json.RawMessage(`{"status":"running"}`)
	case KindTurn:
		return json.RawMessage(`{"turnIndex":1,"status":"active"}`)
	case KindReasoning:
		return json.RawMessage(`{"summaryDelta":"thinking"}`)
	case KindFileChange:
		return json.RawMessage(`{"path":"README.md","operation":"MODIFY"}`)
	case KindPlan:
		return json.RawMessage(`{"summary":"step plan","steps":[{"id":"1","description":"inspect"}]}`)
	case KindProgress:
		return json.RawMessage(`{"label":"compile","message":"building"}`)
	case KindUsage:
		return json.RawMessage(`{"inputTokens":10,"outputTokens":5,"totalTokens":15,"model":"example-model"}`)
	case KindError:
		return json.RawMessage(`{"code":"temporary","message":"retry later","retryable":true}`)
	case KindStreamGap:
		return json.RawMessage(`{"fromSequence":10,"toSequence":20,"firstAvailableSequence":21,"reason":"retention"}`)
	default:
		t.Fatalf("unsupported sample kind %q", kind)
		return nil
	}
}

func TestValidateDraft_AcceptsEveryLegalKindPhasePair(t *testing.T) {
	t.Parallel()

	for _, pair := range legalKindPhasePairs() {
		t.Run(string(pair.Kind)+"/"+string(pair.Phase), func(t *testing.T) {
			t.Parallel()
			draft := Draft{Kind: pair.Kind, Phase: pair.Phase, Payload: sampleDraftPayload(t, pair.Kind, pair.Phase)}
			if err := ValidateDraft(draft); err != nil {
				t.Fatalf("ValidateDraft(%s/%s) error = %v", pair.Kind, pair.Phase, err)
			}
		})
	}
}

func TestValidateDraft_RejectsUnknownKindDistinctlyFromPhasePolicy(t *testing.T) {
	t.Parallel()

	cases := []Kind{"", "NOT_A_KIND", "session"}
	for _, kind := range cases {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			draft := Draft{Kind: kind, Phase: PhaseStarted, Payload: json.RawMessage(`{}`)}
			err := ValidateDraft(draft)

			var invalidKind *InvalidKindError
			if !errors.As(err, &invalidKind) {
				t.Fatalf("ValidateDraft() error = %T(%v), want *InvalidKindError", err, err)
			}

			var validationErr *ValidationError
			if errors.As(err, &validationErr) {
				t.Fatalf("ValidateDraft() unexpectedly returned a phase-policy *ValidationError for kind %q: %v", kind, err)
			}
		})
	}
}

func TestValidateDraft_RejectsUnknownPhaseDistinctlyFromPhasePolicy(t *testing.T) {
	t.Parallel()

	cases := []Phase{"", "NOT_A_PHASE", "started"}
	for _, phase := range cases {
		t.Run(string(phase), func(t *testing.T) {
			t.Parallel()
			draft := Draft{Kind: KindSession, Phase: phase, Payload: json.RawMessage(`{}`)}
			err := ValidateDraft(draft)

			var invalidPhase *InvalidPhaseError
			if !errors.As(err, &invalidPhase) {
				t.Fatalf("ValidateDraft() error = %T(%v), want *InvalidPhaseError", err, err)
			}

			var validationErr *ValidationError
			if errors.As(err, &validationErr) {
				t.Fatalf("ValidateDraft() unexpectedly returned a phase-policy *ValidationError for phase %q: %v", phase, err)
			}
		})
	}
}

func TestValidateDraft_RejectsKnownButDisallowedPhaseAsPhasePolicyFailure(t *testing.T) {
	t.Parallel()

	// SESSION is a declared Kind and DELTA is a declared Phase, but SESSION
	// never allows DELTA per the existing legal-pair policy.
	draft := Draft{Kind: KindSession, Phase: PhaseDelta, Payload: json.RawMessage(`{}`)}
	err := ValidateDraft(draft)

	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("ValidateDraft() error = %T(%v), want *ValidationError (phase-policy failure)", err, err)
	}
	if validationErr.Field != "phase" {
		t.Fatalf("ValidationError.Field = %q, want %q", validationErr.Field, "phase")
	}

	var invalidKind *InvalidKindError
	if errors.As(err, &invalidKind) {
		t.Fatalf("phase-policy failure unexpectedly matched *InvalidKindError")
	}
	var invalidPhase *InvalidPhaseError
	if errors.As(err, &invalidPhase) {
		t.Fatalf("phase-policy failure unexpectedly matched *InvalidPhaseError")
	}
}

func TestValidateDraft_RejectsRepresentativeDisallowedPairsPerKind(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		kind  Kind
		phase Phase
	}{
		{"session rejects delta", KindSession, PhaseDelta},
		{"run rejects updated", KindRun, PhaseUpdated},
		{"turn rejects delta", KindTurn, PhaseDelta},
		{"message rejects updated", KindMessage, PhaseUpdated},
		{"reasoning rejects failed", KindReasoning, PhaseFailed},
		{"tool rejects updated", KindTool, PhaseUpdated},
		{"file change rejects started", KindFileChange, PhaseStarted},
		{"plan rejects completed", KindPlan, PhaseCompleted},
		{"progress rejects canceled", KindProgress, PhaseCanceled},
		{"usage rejects failed", KindUsage, PhaseFailed},
		{"error rejects started", KindError, PhaseStarted},
		{"stream gap rejects delta", KindStreamGap, PhaseDelta},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			draft := Draft{Kind: tc.kind, Phase: tc.phase, Payload: json.RawMessage(`{}`)}
			err := ValidateDraft(draft)
			var validationErr *ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("ValidateDraft(%s/%s) error = %T(%v), want *ValidationError", tc.kind, tc.phase, err, err)
			}
			if validationErr.Field != "phase" {
				t.Fatalf("ValidationError.Field = %q, want %q", validationErr.Field, "phase")
			}
		})
	}
}

func TestValidateDraft_DoesNotMutateInputOnFailure(t *testing.T) {
	t.Parallel()

	payload := json.RawMessage(`{"status":"active"}`)
	draft := Draft{Kind: KindSession, Phase: PhaseDelta, Payload: payload}
	original := CloneDraft(draft)

	_ = ValidateDraft(draft)

	if string(draft.Payload) != string(original.Payload) || draft.Kind != original.Kind || draft.Phase != original.Phase {
		t.Fatalf("ValidateDraft() mutated its input draft: got %+v, want %+v", draft, original)
	}
}

func TestSessionPayloadValidateLineageRejectsContradictoryRelationships(t *testing.T) {
	validContinuation := &SessionContinuation{Provider: "codex", Kind: "session_id", ID: "opaque"}
	tests := []struct {
		name    string
		payload SessionPayload
	}{
		{
			name: "missing resume lineage",
			payload: SessionPayload{
				WorkerSessionID: "current", DispatchID: "dispatch-current", AttemptID: "attempt-current",
				AttemptReason: AttemptReasonResume, Continuation: validContinuation,
			},
		},
		{
			name: "self predecessor",
			payload: SessionPayload{
				WorkerSessionID: "current", DispatchID: "dispatch-current", AttemptID: "attempt-current",
				AttemptReason: AttemptReasonResume, Continuation: validContinuation,
				Lineage: &SessionLineage{PredecessorWorkerSessionID: "current", PreviousDispatchID: "dispatch-old", PreviousAttemptID: "dispatch-old"},
			},
		},
		{
			name: "mismatched prior attempts",
			payload: SessionPayload{
				WorkerSessionID: "current", DispatchID: "dispatch-current", AttemptID: "attempt-current",
				AttemptReason: AttemptReasonRetry,
				Lineage:       &SessionLineage{PreviousDispatchID: "dispatch-old", PreviousAttemptID: "attempt-old"},
			},
		},
		{
			name: "current dispatch reused",
			payload: SessionPayload{
				WorkerSessionID: "current", DispatchID: "dispatch-current", AttemptID: "attempt-current",
				AttemptReason: AttemptReasonRetry,
				Lineage:       &SessionLineage{PreviousDispatchID: "dispatch-current", PreviousAttemptID: "dispatch-current"},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.payload.ValidateLineage(); !errors.Is(err, ErrInvalidSessionLineage) {
				t.Fatalf("ValidateLineage() error = %v, want ErrInvalidSessionLineage", err)
			}
		})
	}
}
