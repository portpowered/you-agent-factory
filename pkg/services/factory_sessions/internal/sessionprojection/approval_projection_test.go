package sessionprojection_test

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func TestBuildProjectionContextDetachesPendingHumanApprovalsInStableOrder(t *testing.T) {
	description := interfaces.NameValueConfig{
		Type:    interfaces.NameValueTypeLocalizableAsset,
		Value:   "Review release",
		Locales: []string{"fr-FR"},
		Values:  map[string]string{"fr-FR": "Examiner la release"},
	}
	world := interfaces.FactoryWorldState{
		PendingHumanApprovalsByID: map[string]interfaces.FactoryWorldHumanApproval{
			"approval-z": {
				ApprovalID:             "approval-z",
				WorkItemIDs:            []string{"work-without-description"},
				TraceIDs:               []string{"trace-z"},
				Decisions:              []interfaces.HumanApprovalDecision{interfaces.HumanApprovalDecisionApprove},
				WorkstationDescription: nil,
			},
			"approval-a": {
				ApprovalID:             "approval-a",
				WorkItemIDs:            []string{"work-with-description"},
				TraceIDs:               []string{"trace-a"},
				Decisions:              []interfaces.HumanApprovalDecision{interfaces.HumanApprovalDecisionApprove, interfaces.HumanApprovalDecisionReject},
				WorkstationDescription: &description,
			},
		},
	}
	ctx, err := sessionprojection.BuildProjectionContext(sessionprojection.ProjectionBuildInput{
		Now:    time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC),
		Events: []interfaces.FactoryEvent{{}},
		WorldStateProjector: func(_ []interfaces.FactoryEvent, _ int) (interfaces.FactoryWorldState, error) {
			return world, nil
		},
	})
	if err != nil {
		t.Fatalf("BuildProjectionContext() error = %v", err)
	}
	if len(ctx.PendingHumanApprovals) != 2 || ctx.PendingHumanApprovals[0].ApprovalID != "approval-a" || ctx.PendingHumanApprovals[1].ApprovalID != "approval-z" {
		t.Fatalf("pending approvals = %#v, want stable approval-id order", ctx.PendingHumanApprovals)
	}
	if ctx.PendingHumanApprovals[0].WorkstationDescription == nil || ctx.PendingHumanApprovals[0].WorkstationDescription.Values["fr-FR"] != "Examiner la release" {
		t.Fatalf("approval description = %#v, want detached localized copy", ctx.PendingHumanApprovals[0].WorkstationDescription)
	}

	ctx.PendingHumanApprovals[0].WorkItemIDs[0] = "mutated-work"
	ctx.PendingHumanApprovals[0].TraceIDs[0] = "mutated-trace"
	ctx.PendingHumanApprovals[0].Decisions[0] = interfaces.HumanApprovalDecisionReject
	ctx.PendingHumanApprovals[0].WorkstationDescription.Values["fr-FR"] = "mutated description"
	original := world.PendingHumanApprovalsByID["approval-a"]
	if original.WorkItemIDs[0] != "work-with-description" || original.TraceIDs[0] != "trace-a" ||
		original.Decisions[0] != interfaces.HumanApprovalDecisionApprove ||
		original.WorkstationDescription.Values["fr-FR"] != "Examiner la release" {
		t.Fatalf("pending approval projection aliases reconstructed world state: %#v", original)
	}

	runtime := sessionprojection.ProjectRuntimeContract(ctx)
	if len(runtime.PendingHumanApprovals) != 2 || runtime.PendingHumanApprovals[0].ApprovalID != "approval-a" {
		t.Fatalf("runtime pending approvals = %#v, want projected approval list", runtime.PendingHumanApprovals)
	}
}

func TestBuildProjectionContextUsesDetachedIncrementalSessionFacts(t *testing.T) {
	description := interfaces.NameValueConfig{
		Type:   interfaces.NameValueTypeLocalizableAsset,
		Value:  "Review release",
		Values: map[string]string{"fr-FR": "Examiner la release"},
	}
	facts := recordings.SessionProjectionFacts{
		PendingHumanApprovals: map[string]interfaces.FactoryWorldHumanApproval{
			"approval-b": {ApprovalID: "approval-b", WorkItemIDs: []string{"work-b"}},
			"approval-a": {ApprovalID: "approval-a", WorkItemIDs: []string{"work-a"}, WorkstationDescription: &description},
		},
	}
	ctx, err := sessionprojection.BuildProjectionContext(sessionprojection.ProjectionBuildInput{
		Now:               time.Date(2026, 8, 12, 20, 0, 0, 0, time.UTC),
		SessionProjection: &facts,
	})
	if err != nil {
		t.Fatalf("BuildProjectionContext() error = %v", err)
	}
	if len(ctx.PendingHumanApprovals) != 2 || ctx.PendingHumanApprovals[0].ApprovalID != "approval-a" || ctx.PendingHumanApprovals[1].ApprovalID != "approval-b" {
		t.Fatalf("pending approvals = %#v, want stable incremental projection order", ctx.PendingHumanApprovals)
	}
	ctx.PendingHumanApprovals[0].WorkItemIDs[0] = "mutated-work"
	ctx.PendingHumanApprovals[0].WorkstationDescription.Values["fr-FR"] = "mutated description"
	if facts.PendingHumanApprovals["approval-a"].WorkItemIDs[0] != "work-a" || facts.PendingHumanApprovals["approval-a"].WorkstationDescription.Values["fr-FR"] != "Examiner la release" {
		t.Fatalf("incremental session facts were aliased by projection context: %#v", facts.PendingHumanApprovals["approval-a"])
	}
}

var _ factoryruntime.WorldStateProjector = func([]interfaces.FactoryEvent, int) (interfaces.FactoryWorldState, error) {
	return interfaces.FactoryWorldState{}, nil
}
