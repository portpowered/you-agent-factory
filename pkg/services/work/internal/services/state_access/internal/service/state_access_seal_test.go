package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

// TestStateAccessSealSubmitAndMovePipeline seals IMP-WORK-04 story 001 focused
// proof: detached submit/move success and already-applied move typed failure
// through one state_access Service instance and a private Session adapter fake.
func TestStateAccessSealSubmitAndMovePipeline(t *testing.T) {
	t.Parallel()

	adapter := &sealSessionAdapter{}
	svc := New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()

	submitted, err := svc.SubmitWorkRequestForSession(ctx, "session-seal", work.WorkRequest{
		RequestID: "request-seal-1",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "story-1",
			WorkTypeID: "story",
		}},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !submitted.Accepted || submitted.RequestID != "request-seal-1" {
		t.Fatalf("submit result = %#v, want accepted request-seal-1", submitted)
	}
	if submitted.Works[0].WorkID != "work-seal-1" {
		t.Fatalf("submit works = %#v, want detached work identity", submitted.Works)
	}

	moved, err := svc.MoveWorkForSession(ctx, "session-seal", "work-seal-1", "review", "move-seal-1")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	assertDetachedMoveResult(t, moved, "work-seal-1", "draft", "review")

	_, err = svc.MoveWorkForSession(ctx, "session-seal", "work-seal-1", "done", "dup-move")
	if err != nil {
		t.Fatalf("first dup-move: %v", err)
	}
	_, err = svc.MoveWorkForSession(ctx, "session-seal", "work-seal-1", "done", "dup-move")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("duplicate move error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}

// TestStateAccessSealFullStateAccessPipeline seals IMP-WORK-04 story 003 proof:
// submit/move/list/get/move-and-read return detached Work-owned shapes through
// one state_access Service and a private Session adapter fake.
func TestStateAccessSealFullStateAccessPipeline(t *testing.T) {
	t.Parallel()

	adapter := &sealSessionAdapter{}
	svc := New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()
	sessionID := "session-seal-full"

	submitted, err := svc.SubmitWorkRequestForSession(ctx, sessionID, work.WorkRequest{
		RequestID: "request-seal-full",
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name:       "story-1",
			WorkTypeID: "story",
		}},
	})
	if err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if !submitted.Accepted || submitted.RequestID != "request-seal-full" {
		t.Fatalf("submit result = %#v, want accepted request-seal-full", submitted)
	}

	listed, err := svc.ListWork(ctx, sessionID, work.ListOptions{WorkTypeName: "story"})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(listed.Results) != 1 || listed.Results[0].WorkID != "work-seal-1" {
		t.Fatalf("ListWork = %#v, want one story work item", listed)
	}
	listed.Results[0].State.Name = "mutated"
	secondList, err := svc.ListWork(ctx, sessionID, work.ListOptions{WorkTypeName: "story"})
	if err != nil || secondList.Results[0].State.Name != "draft" {
		t.Fatalf("ListWork mutated source snapshot: %#v, %v", secondList, err)
	}

	got, err := svc.GetWork(ctx, sessionID, "work-seal-1")
	if err != nil || got.WorkID != "work-seal-1" || got.State.Name != "draft" {
		t.Fatalf("GetWork = %#v, %v", got, err)
	}

	moved, err := svc.MoveWorkForSession(ctx, sessionID, "work-seal-1", "review", "move-seal-full")
	if err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	assertDetachedMoveResult(t, moved, "work-seal-1", "draft", "review")

	readAfterMove, err := svc.MoveWorkAndRead(ctx, sessionID, "work-seal-1", "done", "move-and-read-seal")
	if err != nil {
		t.Fatalf("MoveWorkAndRead: %v", err)
	}
	if readAfterMove.WorkID != "work-seal-1" || readAfterMove.State.Name != "done" {
		t.Fatalf("MoveWorkAndRead = %#v, want detached done read model", readAfterMove)
	}
	readAfterMove.State.Name = "mutated"
	refreshed, err := svc.GetWork(ctx, sessionID, "work-seal-1")
	if err != nil || refreshed.State.Name != "done" {
		t.Fatalf("detached read mutated source snapshot: %#v, %v", refreshed, err)
	}
}

// TestStateAccessSealTypedFailures seals IMP-WORK-04 story 003 typed failure
// proof for not-found reads and already-applied operator moves.
func TestStateAccessSealTypedFailures(t *testing.T) {
	t.Parallel()

	adapter := &sealSessionAdapter{}
	svc := New(stubSessionResolver{adapter: adapter})
	ctx := context.Background()
	sessionID := "session-seal-failures"

	if _, err := svc.GetWork(ctx, sessionID, "missing-work"); !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork(missing) error = %v, want ErrWorkNotFound", err)
	}

	_, err := svc.MoveWorkForSession(ctx, sessionID, "work-seal-1", "done", "dup-move-seal")
	if err != nil {
		t.Fatalf("first dup-move: %v", err)
	}
	_, err = svc.MoveWorkForSession(ctx, sessionID, "work-seal-1", "done", "dup-move-seal")
	if !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
		t.Fatalf("duplicate move error = %v, want ErrMoveWorkRequestAlreadyApplied", err)
	}
}

// TestStateAccessOwnerDoesNotImportFactoryRuntimeOrPetri fails closed when the
// private state_access owner reintroduces Factory Runtime or Petri packages.
func TestStateAccessOwnerDoesNotImportFactoryRuntimeOrPetri(t *testing.T) {
	t.Parallel()

	forbidden := []string{
		"pkg/services/factory_runtime",
		"orchestrators/petri",
		"pkg/factory/",
	}
	packages := []string{
		".",
		filepath.Join("..", ".."),
	}
	for _, pkgDir := range packages {
		entries, err := os.ReadDir(pkgDir)
		if err != nil {
			t.Fatalf("read %s: %v", pkgDir, err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
				strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			source, err := os.ReadFile(filepath.Join(pkgDir, entry.Name()))
			if err != nil {
				t.Fatalf("read %s: %v", entry.Name(), err)
			}
			for _, needle := range forbidden {
				if strings.Contains(string(source), needle) {
					t.Errorf("%s imports forbidden Factory Runtime or Petri package %q", entry.Name(), needle)
				}
			}
		}
	}
}

func assertDetachedMoveResult(
	t *testing.T,
	result work.OperatorMoveResult,
	workID string,
	fromState string,
	toState string,
) {
	t.Helper()
	if result.WorkID != workID || result.FromState != fromState || result.ToState != toState {
		t.Fatalf("move result = %#v, want detached %s->%s for %s", result, fromState, toState, workID)
	}
	if result.FromPlaceID != "" || result.ToPlaceID != "" || result.TokenID != "" {
		t.Fatalf("move result leaked Petri fields: %#v", result)
	}
}

type sealSessionAdapter struct {
	appliedMoves map[string]bool
	workStates   map[string]string
}

func (a *sealSessionAdapter) SubmitWorkRequest(
	_ context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if a.workStates == nil {
		a.workStates = map[string]string{"work-seal-1": "draft"}
	}
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
		Works: []work.WorkRequestSubmittedWork{{
			Name:         "story-1",
			WorkTypeName: "story",
			WorkID:       "work-seal-1",
		}},
	}, nil
}

func (a *sealSessionAdapter) MoveWork(
	_ context.Context,
	workID string,
	stateName string,
	_ work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	if a.appliedMoves == nil {
		a.appliedMoves = make(map[string]bool)
	}
	if a.workStates == nil {
		a.workStates = map[string]string{"work-seal-1": "draft"}
	}
	if a.appliedMoves[requestID] {
		return work.OperatorMoveResult{}, work.ErrMoveWorkRequestAlreadyApplied
	}
	a.appliedMoves[requestID] = true
	fromState := a.workStates[workID]
	if fromState == "" {
		fromState = "draft"
	}
	a.workStates[workID] = stateName
	return work.OperatorMoveResult{
		WorkID:      workID,
		WorkTypeID:  "story",
		FromState:   fromState,
		ToState:     stateName,
		FromPlaceID: "story:" + fromState,
		ToPlaceID:   "story:" + stateName,
		TokenID:     "tok-seal",
	}, nil
}

func (a *sealSessionAdapter) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	if a.workStates == nil {
		a.workStates = map[string]string{"work-seal-1": "draft"}
	}
	stateName := a.workStates["work-seal-1"]
	stateType := work.StateTypeInitial
	if stateName == "review" {
		stateType = work.StateTypeProcessing
	}
	if stateName == "done" {
		stateType = work.StateTypeTerminal
	}
	return work.ReadSnapshot{Items: []work.ReadModel{{
		CursorID:     "tok-seal-1",
		WorkID:       "work-seal-1",
		Name:         "story-1",
		WorkTypeName: "story",
		State:        &work.State{Name: stateName, Type: stateType},
	}}}, nil
}

type stubSessionResolver struct {
	adapter stateaccess.SessionAdapter
}

func (r stubSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return r.adapter, nil
}
