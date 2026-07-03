package guards_batch

import (
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMultiChannelGuard_FileDropToCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter via FileWatcher"}`))

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
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")
}

func TestMultiChannelGuard_ExecutionIDPropagation(t *testing.T) {
	const wantExecutionID = "exec-guard-propagation"

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedExecutionFile(t, dir, "chapter", wantExecutionID, []byte(`{"title": "Execution ID propagation test"}`))

	var submissionsMu sync.Mutex
	var chapterSubmission interfaces.FactorySubmissionRecord

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithExtraOptions(factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
			if record.Request.WorkTypeID != "chapter" {
				return
			}
			submissionsMu.Lock()
			defer submissionsMu.Unlock()
			if chapterSubmission.Request.WorkTypeID == "" {
				chapterSubmission = record
			}
		})),
	)

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.MockWorker("processor",
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
		interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted},
	)
	h.MockWorker("completer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	h.RunUntilComplete(t, 10*time.Second)

	submissionsMu.Lock()
	gotSubmission := chapterSubmission
	submissionsMu.Unlock()

	if gotSubmission.Request.ExecutionID != wantExecutionID {
		t.Fatalf("chapter submission execution ID = %q, want %q", gotSubmission.Request.ExecutionID, wantExecutionID)
	}
	if gotSubmission.Request.Tags[executionIDTagKey] != wantExecutionID {
		t.Fatalf("chapter submission execution tag = %q, want %q", gotSubmission.Request.Tags[executionIDTagKey], wantExecutionID)
	}

	pageTokens := h.Marking().TokensInPlace("page:complete")
	if len(pageTokens) != 3 {
		t.Fatalf("page:complete token count = %d, want 3", len(pageTokens))
	}
	for _, token := range pageTokens {
		if token.Color.Tags[executionIDTagKey] != wantExecutionID {
			t.Fatalf("page work %s execution tag = %q, want %q", token.Color.WorkID, token.Color.Tags[executionIDTagKey], wantExecutionID)
		}
	}

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")
}

func TestMultiChannelGuard_GuardBlocksUntilAllPagesComplete(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Guard blocking test"}`))

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

func TestMultiChannelGuard_DynamicExecDirWithGuard(t *testing.T) {
	t.Skip("pending migration: tests FileWatcher dynamic exec-dir creation which requires direct adapter access")
}
