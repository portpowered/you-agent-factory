package petri

import "github.com/portpowered/infinite-you/pkg/services/work"

// Transition represents a workstation — the unit of transformation.
// A transition is enabled when all input arcs are satisfied (tokens present + guards pass).
// When fired, it consumes input tokens, invokes the worker, and produces output tokens.
//
// Four output arc sets handle the distinct outcomes:
//   - OutputArcs:    the transition succeeded (e.g., review approved → code-change:complete)
//   - ContinueArcs:  the transition made partial progress and should iterate
//     (e.g., process returned <CONTINUE> → code-change:init for another pass)
//   - RejectionArcs: the transition succeeded but the business result was negative
//     (e.g., review rejected → code-change:init for retry)
//   - FailureArcs:   the transition crashed, timed out, or hit execution limits
//     (e.g., agent OOM'd → code-change:failed)
type Transition struct {
	ID                string                             `json:"id"`
	Name              string                             `json:"name"`
	Type              TransitionType                     `json:"type"` // NORMAL, HUMAN_APPROVAL, or EXHAUSTION
	InputArcs         []Arc                              `json:"input_arcs"`
	OutputArcs        []Arc                              `json:"output_arcs"`    // used when WorkResult.Outcome == ACCEPTED
	ContinueArcs      []Arc                              `json:"continue_arcs"`  // used when WorkResult.Outcome == CONTINUE
	RejectionArcs     []Arc                              `json:"rejection_arcs"` // used when WorkResult.Outcome == REJECTED
	FailureArcs       []Arc                              `json:"failure_arcs"`   // used when WorkResult.Outcome == FAILED
	WorkerType        string                             `json:"worker_type"`    // which worker type executes this transition
	ExpectedArtifacts []work.ExpectedArtifactDeclaration `json:"expected_artifacts,omitempty"`
}

// TransitionType distinguishes worker-backed work, human approval, and
// exhaustion transitions.
type TransitionType string

const (
	// TransitionNormal is a regular work transition — fires when inputs are satisfied,
	// dispatches to a worker, produces output tokens.
	TransitionNormal TransitionType = "NORMAL"

	// TransitionHumanApproval is a workerless graph transition whose claimed
	// work remains pending until the human-approval runtime records a decision.
	TransitionHumanApproval TransitionType = "HUMAN_APPROVAL"

	// TransitionExhaustion is a circuit-breaking transition — fires when a token's
	// visit history exceeds a threshold. No worker is invoked.
	TransitionExhaustion TransitionType = "EXHAUSTION"
)
