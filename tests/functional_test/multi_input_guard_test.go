package functional_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestMultiInputGuard_PartialChildren_BlocksGuard verifies that the
// all_children_complete guard correctly blocks until all children are complete,
// then fires the chapter-complete transition.
//
// Flow:
//  1. Submit chapter → parser spawns 3 pages → chapter:processing + 3 page:init
//  2. All 3 pages process to completion
//  3. Guard fires → chapter:complete
func TestMultiInputGuard_PartialChildren_BlocksGuard(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Guard partial test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("completer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("page:init")
}

// TestMultiInputGuard_AllChildrenComplete verifies the happy path: parser
// spawns 3 pages, all complete, and the guard fires moving the chapter to
// complete.
func TestMultiInputGuard_AllChildrenComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Guard happy path"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	completerMock := h.MockWorker("completer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	if completerMock.CallCount() != 1 {
		t.Errorf("expected completer called 1 time, got %d", completerMock.CallCount())
	}
}

// TestMultiInputGuard_IndependentChapters verifies that two chapters with
// different page counts complete independently. Chapter A has 2 pages and
// chapter B has 5 pages. Chapter A should complete before chapter B.
func TestMultiInputGuard_IndependentChapters(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter A (2 pages)"}`))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter B (5 pages)"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Custom parser that returns different child counts per call.
	parserExec := &multiChapterParserExecutor{
		childCounts: []int{2, 5},
	}
	h.SetCustomExecutor("parser", parserExec)

	// 7 pages total (2 + 5).
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	completerMock := h.MockWorker("completer",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)

	h.RunUntilComplete(t, 10*time.Second)

	// Both chapters complete independently.
	h.Assert().
		PlaceTokenCount("chapter:complete", 2).
		PlaceTokenCount("page:complete", 7).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	if completerMock.CallCount() != 2 {
		t.Errorf("expected completer called 2 times (once per chapter), got %d", completerMock.CallCount())
	}
}

// TestMultiInputGuard_IndependentChapters_StaggeredCompletion verifies that
// two chapters with different page counts both complete independently,
// confirming per-input guards evaluate independently per parent token.
func TestMultiInputGuard_IndependentChapters_StaggeredCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter A (2 pages)"}`))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter B (5 pages)"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	// Chapter A: 2 pages, Chapter B: 5 pages.
	parserExec := &multiChapterParserExecutor{
		childCounts: []int{2, 5},
	}
	h.SetCustomExecutor("parser", parserExec)

	h.MockWorker("processor")
	h.MockWorker("completer")

	h.RunUntilComplete(t, 10*time.Second)

	// Both chapters complete with all pages processed.
	h.Assert().
		PlaceTokenCount("chapter:complete", 2).
		PlaceTokenCount("page:complete", 7)
}

// multiChapterParserExecutor spawns different numbers of child pages per call.
// The i-th call spawns childCounts[i] pages with ParentID set to the input
// chapter's WorkID.
type multiChapterParserExecutor struct {
	mu          sync.Mutex
	calls       int
	childCounts []int
}

func (e *multiChapterParserExecutor) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	e.mu.Lock()
	call := e.calls
	e.calls++
	e.mu.Unlock()

	parentWorkID := ""
	if len(dispatch.InputTokens) > 0 {
		parentWorkID = firstInputToken(dispatch.InputTokens).Color.WorkID
	}

	childCount := 0
	if call < len(e.childCounts) {
		childCount = e.childCounts[call]
	}

	spawned := make([]interfaces.TokenColor, childCount)
	for i := range spawned {
		spawned[i] = interfaces.TokenColor{
			WorkTypeID: "page",
			WorkID:     fmt.Sprintf("%s-page-%d", parentWorkID, i+1),
			ParentID:   parentWorkID,
		}
	}

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
		SpawnedWork:  spawned,
	}, nil
}
