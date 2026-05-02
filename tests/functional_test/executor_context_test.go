package functional_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestExecutorContext_InputTokenColors verifies that the dispatched WorkDispatch
// contains input token colors carrying the original payload and tags submitted
// with the work item.
func TestExecutorContext_InputTokenColors(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))

	payload := []byte(`{"feature": "dark mode", "priority": "high"}`)
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "code-change",
		Payload:    payload,
		Tags:       map[string]string{"team": "frontend", "sprint": "42"},
	})

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	sweCalls := providerCallsForWorker(provider, "swe")
	if len(sweCalls) != 1 {
		t.Fatalf("expected swe called 1 time, got %d", len(sweCalls))
	}

	dispatch := sweCalls[0]
	if len(dispatch.InputTokens) == 0 {
		t.Fatal("dispatch has no input tokens")
	}

	color := firstInputToken(dispatch.InputTokens).Color

	// Verify payload is carried through.
	if !bytes.Equal(color.Payload, payload) {
		t.Errorf("expected payload %q, got %q", payload, color.Payload)
	}

	// Verify tags are carried through.
	if color.Tags["team"] != "frontend" {
		t.Errorf("expected tag team=frontend, got %q", color.Tags["team"])
	}
	if color.Tags["sprint"] != "42" {
		t.Errorf("expected tag sprint=42, got %q", color.Tags["sprint"])
	}

	// Verify work type is correct.
	if color.WorkTypeID != "code-change" {
		t.Errorf("expected WorkTypeID %q, got %q", "code-change", color.WorkTypeID)
	}
}

// TestExecutorContext_RejectionFeedback verifies that when a reviewer rejects,
// the rejection feedback is propagated to the executor's next dispatch via the
// _rejection_feedback tag on the input token.
func TestExecutorContext_RejectionFeedback(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	sweMock := h.MockWorker("swe",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("reviewer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeRejected, Feedback: "needs unit tests"},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	if sweMock.CallCount() != 2 {
		t.Fatalf("expected swe called 2 times, got %d", sweMock.CallCount())
	}

	calls := sweMock.Calls()

	// First dispatch should have no rejection feedback.
	firstColor := firstInputToken(calls[0].InputTokens).Color
	if _, ok := firstColor.Tags["_rejection_feedback"]; ok {
		t.Error("first dispatch should not have _rejection_feedback tag")
	}

	// Second dispatch should carry the rejection feedback.
	secondColor := firstInputToken(calls[1].InputTokens).Color
	feedback, ok := secondColor.Tags["_rejection_feedback"]
	if !ok {
		t.Fatal("second dispatch missing _rejection_feedback tag")
	}
	if feedback != "needs unit tests" {
		t.Errorf("expected rejection feedback %q, got %q", "needs unit tests", feedback)
	}

	// Original payload should still be present after rejection loop.
	if !bytes.Contains(secondColor.Payload, []byte("auth")) {
		t.Errorf("expected payload to contain 'auth' after rejection, got %q", secondColor.Payload)
	}
}

// TestExecutorContext_ParentLineage verifies that when a token is submitted with
// relations (e.g., PARENT_CHILD), the dispatched WorkDispatch carries those
// relations on the input token so the executor can track lineage.
func TestExecutorContext_ParentLineage(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "code_review"))

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID:  "code-change",
		WorkID:      "prereq-work-99",
		TargetState: "complete",
		Payload:     []byte(`{"feature": "prerequisite"}`),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "code-change",
		WorkID:     "child-work-1",
		Payload:    []byte(`{"feature": "login page"}`),
		Relations: []interfaces.Relation{
			{
				Type:         interfaces.RelationParentChild,
				TargetWorkID: "parent-prd-42",
			},
			{
				Type:          interfaces.RelationDependsOn,
				TargetWorkID:  "prereq-work-99",
				RequiredState: "complete",
			},
		},
	})

	provider := testutil.NewMockProvider(
		acceptedProviderResponse(),
		acceptedProviderResponse(),
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	sweCalls := providerCallsForWorker(provider, "swe")
	if len(sweCalls) != 1 {
		t.Fatalf("expected swe called 1 time, got %d", len(sweCalls))
	}

	dispatch := sweCalls[0]
	if len(dispatch.InputTokens) == 0 {
		t.Fatal("dispatch has no input tokens")
	}

	color := firstInputToken(dispatch.InputTokens).Color

	// Verify WorkID is preserved.
	if color.WorkID != "child-work-1" {
		t.Errorf("expected WorkID %q, got %q", "child-work-1", color.WorkID)
	}

	// Verify relations are preserved on the input token.
	if len(color.Relations) != 2 {
		t.Fatalf("expected 2 relations, got %d", len(color.Relations))
	}

	foundParent := false
	foundDependsOn := false
	for _, rel := range color.Relations {
		switch rel.Type {
		case interfaces.RelationParentChild:
			foundParent = true
			if rel.TargetWorkID != "parent-prd-42" {
				t.Errorf("expected parent TargetWorkID %q, got %q", "parent-prd-42", rel.TargetWorkID)
			}
		case interfaces.RelationDependsOn:
			foundDependsOn = true
			if rel.TargetWorkID != "prereq-work-99" {
				t.Errorf("expected depends-on TargetWorkID %q, got %q", "prereq-work-99", rel.TargetWorkID)
			}
			if rel.RequiredState != "complete" {
				t.Errorf("expected RequiredState %q, got %q", "complete", rel.RequiredState)
			}
		}
	}
	if !foundParent {
		t.Error("PARENT_CHILD relation not found on dispatched token")
	}
	if !foundDependsOn {
		t.Error("DEPENDS_ON relation not found on dispatched token")
	}
}
