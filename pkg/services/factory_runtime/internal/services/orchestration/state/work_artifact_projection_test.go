package state_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	runtimestate "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/state"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestWorkArtifactProjection_UsesActiveDispatchAndRecordedContext(t *testing.T) {
	t.Parallel()

	token := workerexecution.Token{
		ID:    "token-1",
		State: "init",
		Color: factory.RuntimeTokenColor{
			Name:       "review request",
			WorkID:     "work-1",
			WorkTypeID: "story",
			DataType:   workerexecution.DataTypeWork,
			Tags:       map[string]string{workerexecution.ProjectTagKey: "token-project"},
		},
	}
	net := &runtimestate.Net{
		WorkTypes: map[string]*runtimestate.WorkType{
			"story": {ID: "story", ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
				Name: "summary", Pattern: "summary.txt",
			}}},
		},
		Transitions: map[string]*petri.Transition{
			"review": {ID: "review", Name: "review", ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{
				Name: "report", Pattern: "{{ .Context.Project }}/{{ .Context.SessionID }}/report.txt",
			}}},
		},
	}
	dispatches := map[string]*factory.DispatchEntry{
		"earlier": nil,
		"active": {
			DispatchID:      "active",
			TransitionID:    "review",
			WorkstationName: "review",
			ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{
				Project: "project-7", SessionID: "session-9",
			},
			ConsumedTokens: []workerexecution.Token{token},
		},
	}

	got := (factory.WorkArtifactProjection{}).Project(factory.WorkArtifactProjectionInput{
		Token: &token, Topology: net, Dispatches: dispatches,
	})
	if len(got) != 2 || got[0].Pattern != "summary.txt" || got[1].Pattern != "project-7/session-9/report.txt" {
		t.Fatalf("active expected artifacts = %#v", got)
	}
	if got[0].Verification != work.ExpectedArtifactVerificationPending || got[1].Verification != work.ExpectedArtifactVerificationPending {
		t.Fatalf("active verification = %#v, want pending declarations", got)
	}
	token.Color.Tags[workerexecution.ProjectTagKey] = "mutated"
	if got[0].Pattern != "summary.txt" {
		t.Fatalf("projection changed after token mutation: %#v", got)
	}
}

func TestWorkArtifactProjection_UsesCompletedVerificationAndResultFallbacks(t *testing.T) {
	t.Parallel()

	token := workerexecution.Token{ID: "token-1", State: "init", Color: factory.RuntimeTokenColor{
		Name: "story", WorkID: "work-1", WorkTypeID: "story", DataType: workerexecution.DataTypeWork,
	}}
	net := &runtimestate.Net{
		WorkTypes: map[string]*runtimestate.WorkType{
			"story": {ID: "story", ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "report", Pattern: "report.txt"}}},
		},
		Transitions: map[string]*petri.Transition{
			"review": {ID: "review", Name: "review", ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "manifest", Pattern: "manifest.json"}}},
		},
	}

	failed := (factory.WorkArtifactProjection{}).Project(factory.WorkArtifactProjectionInput{
		Token: &token, Topology: net,
		DispatchHistory: []factory.CompletedDispatch{{
			DispatchID: "dispatch-1", TransitionID: "review", WorkstationName: "review", ConsumedTokens: []workerexecution.Token{token},
			ExpectedArtifactContext: &work.ExpectedArtifactTemplateContext{Project: "project-7", SessionID: "session-9"},
		}},
		Results: []workerexecution.WorkResult{{
			DispatchID: "dispatch-1", Outcome: workerexecution.OutcomeFailed,
			ArtifactVerification: &workerexecution.ExpectedArtifactVerification{Entries: []workerexecution.ExpectedArtifactVerificationEntry{{
				DeclarationIndex: 2, Name: "manifest", Pattern: "manifest.json", Reason: workerexecution.ExpectedArtifactVerificationReasonMissing,
			}}},
		}},
	})
	if len(failed) != 2 || failed[0].Verification != work.ExpectedArtifactVerificationSatisfied || failed[1].Verification != work.ExpectedArtifactVerificationFailed {
		t.Fatalf("completed failed projection = %#v", failed)
	}

	accepted := (factory.WorkArtifactProjection{}).Project(factory.WorkArtifactProjectionInput{
		Token: &token, Topology: net,
		DispatchHistory: []factory.CompletedDispatch{{
			DispatchID: "dispatch-accepted", TransitionID: "review", ConsumedTokens: []workerexecution.Token{token}, Outcome: workerexecution.OutcomeAccepted,
		}},
	})
	if len(accepted) != 2 || accepted[0].Verification != work.ExpectedArtifactVerificationSatisfied || accepted[1].Verification != work.ExpectedArtifactVerificationSatisfied {
		t.Fatalf("accepted projection = %#v", accepted)
	}

	pending := (factory.WorkArtifactProjection{}).Project(factory.WorkArtifactProjectionInput{
		Token: &token, Topology: net,
		DispatchHistory: []factory.CompletedDispatch{{
			DispatchID: "dispatch-failed", TransitionID: "review", ConsumedTokens: []workerexecution.Token{token}, Outcome: workerexecution.OutcomeFailed,
		}},
	})
	if len(pending) != 2 || pending[0].Verification != work.ExpectedArtifactVerificationPending || pending[1].Verification != work.ExpectedArtifactVerificationPending {
		t.Fatalf("unverified projection = %#v", pending)
	}
}

func TestWorkArtifactProjection_FallsBackToPlaceTopologyAndHandlesEmptyInput(t *testing.T) {
	t.Parallel()

	got := (factory.WorkArtifactProjection{}).Project(factory.WorkArtifactProjectionInput{})
	if got != nil {
		t.Fatalf("nil token projection = %#v, want nil", got)
	}
	token := workerexecution.Token{ID: "token-1", State: "init", Color: factory.RuntimeTokenColor{
		WorkID: "work-1", WorkTypeID: "story", DataType: workerexecution.DataTypeWork,
	}}
	net := &runtimestate.Net{
		WorkTypes: map[string]*runtimestate.WorkType{
			"story": {ID: "story", ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "report", Pattern: "report.txt"}}},
		},
		Transitions: map[string]*petri.Transition{
			"z-review": {ID: "z-review", Name: "z-review", InputArcs: []petri.Arc{{PlaceID: "other:init"}}, ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "other", Pattern: "other.txt"}}},
			"a-review": {ID: "a-review", Name: "a-review", InputArcs: []petri.Arc{{PlaceID: "story:init"}}, ExpectedArtifacts: []work.ExpectedArtifactDeclaration{{Name: "review", Pattern: "review.txt"}}},
			"nil":      nil,
		},
	}
	got = (factory.WorkArtifactProjection{}).Project(factory.WorkArtifactProjectionInput{Token: &token, Topology: net})
	if len(got) != 2 || got[0].Pattern != "report.txt" || got[1].Pattern != "review.txt" {
		t.Fatalf("place fallback projection = %#v", got)
	}
}
