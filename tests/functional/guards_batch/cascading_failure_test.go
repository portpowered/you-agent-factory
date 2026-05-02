package guards_batch

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestCascadingFailure_DirectChild(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	parentWorkID := "parent-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID:  "task",
		WorkID:      parentWorkID,
		TargetState: "processing",
		Payload:     []byte("parent"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("child"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: parentWorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"finisher": {
			{Error: errors.New("upstream service down")},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 2).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:complete")

	snap := h.Marking()
	foundChild := false
	for _, tok := range snap.Tokens {
		if tok.Color.WorkTypeID == "task" && tokenDependsOn(tok, parentWorkID) {
			foundChild = true
			if len(tok.History.FailureLog) == 0 {
				t.Error("child token should have a FailureRecord from cascading failure")
			} else {
				record := tok.History.FailureLog[0]
				if !strings.Contains(record.Error, parentWorkID) {
					t.Errorf("FailureRecord should reference parent WorkID %q, got: %q", parentWorkID, record.Error)
				}
			}
			if !strings.Contains(tok.History.LastError, parentWorkID) {
				t.Errorf("LastError should reference parent WorkID %q, got: %q", parentWorkID, tok.History.LastError)
			}
		}
	}
	if !foundChild {
		t.Fatal("child token with dependency on parent was not found")
	}
}

func tokenDependsOn(tok *interfaces.Token, workID string) bool {
	for _, rel := range tok.Color.Relations {
		if rel.Type == interfaces.RelationDependsOn && rel.TargetWorkID == workID {
			return true
		}
	}
	return false
}

func TestCascadingFailure_Transitive(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	pWorkID := "P-work-id"
	c1WorkID := "C1-work-id"

	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     pWorkID,
		Payload:    []byte("P"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     c1WorkID,
		Payload:    []byte("C1"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: pWorkID, RequiredState: "complete"},
		},
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("C2"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: c1WorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		"finisher": {
			{Error: errors.New("crash")},
			{Error: errors.New("crash")},
			{Error: errors.New("crash")},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:failed", 3).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:processing").
		HasNoTokenInPlace("task:complete")

	snap := h.Marking()
	for _, tok := range snap.Tokens {
		if tok.Color.WorkID != pWorkID && tok.Color.WorkID != c1WorkID {
			if len(tok.History.FailureLog) == 0 && tok.History.LastError == "" {
				t.Error("C2 should have a failure record")
			}
		}
	}
}

func TestCascadingFailure_CompletedNotCascaded(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	aWorkID := "A-work-id"
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     aWorkID,
		Payload:    []byte("A"),
	})
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("B"),
		Relations: []interfaces.Relation{
			{Type: interfaces.RelationDependsOn, TargetWorkID: aWorkID, RequiredState: "complete"},
		},
	})

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		"finisher": {
			{Content: "COMPLETE"},
			{Error: errors.New("oops")},
		},
	})
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		HasTokenInPlace("task:complete").
		HasTokenInPlace("task:failed")
}
