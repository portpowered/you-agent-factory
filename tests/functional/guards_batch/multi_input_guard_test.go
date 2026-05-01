package guards_batch

import (
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

func TestMultiInputGuard_PartialChildren_BlocksGuard(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Guard partial test"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("completer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("page:init")
}

func TestMultiInputGuard_AllChildrenComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Guard happy path"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	completerMock := h.MockWorker("completer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

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

func TestMultiInputGuard_IndependentChapters(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter A (2 pages)"}`))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter B (5 pages)"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &multiChapterParserExecutor{
		childCounts: []int{2, 5},
	}
	h.SetCustomExecutor("parser", parserExec)

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

func TestMultiInputGuard_IndependentChapters_StaggeredCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter A (2 pages)"}`))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter B (5 pages)"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &multiChapterParserExecutor{
		childCounts: []int{2, 5},
	}
	h.SetCustomExecutor("parser", parserExec)

	h.MockWorker("processor")
	h.MockWorker("completer")

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 2).
		PlaceTokenCount("page:complete", 7)
}
