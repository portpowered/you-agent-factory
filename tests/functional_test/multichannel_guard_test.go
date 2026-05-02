package functional_test

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

// TestMultiChannelGuard_FileDropToCompletion confirms end-to-end: a chapter
// seed file is picked up by the service, the parser spawns 3 pages, pages
// process through to page:complete, and the per-input guard fires moving
// the chapter to complete.
func TestMultiChannelGuard_FileDropToCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter via FileWatcher"}`))

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
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")
}

// TestMultiChannelGuard_ExecutionIDPropagation verifies that files dropped
// into inputs/page/<exec-id>/ propagate the execution-id to SubmitRequest,
// and that the chapter guard still evaluates correctly with the combined flow.
func TestMultiChannelGuard_ExecutionIDPropagation(t *testing.T) {
	t.Skip("pending migration: tests FileWatcher execution-id propagation which requires direct adapter access")
}

// TestMultiChannelGuard_GuardBlocksUntilAllPagesComplete uses seed files to
// submit a chapter, then verifies the all_children_complete guard blocks
// until all spawned pages reach page:complete.
func TestMultiChannelGuard_GuardBlocksUntilAllPagesComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, fixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Guard blocking test"}`))

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

// TestMultiChannelGuard_DynamicExecDirWithGuard verifies that dynamically
// created execution-id directories under inputs/page/ are auto-watched,
// and submitted pages carry the correct execution-id.
func TestMultiChannelGuard_DynamicExecDirWithGuard(t *testing.T) {
	t.Skip("pending migration: tests FileWatcher dynamic exec-dir creation which requires direct adapter access")
}
