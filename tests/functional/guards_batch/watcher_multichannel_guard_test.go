package guards_batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	const wantExecutionID = "exec-dynamic-guard"

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	chapterDefaultDir := filepath.Join(dir, interfaces.InputsDir, "chapter", interfaces.DefaultChannelName)
	if err := os.MkdirAll(chapterDefaultDir, 0o755); err != nil {
		t.Fatalf("create chapter default channel: %v", err)
	}

	var submissionsMu sync.Mutex
	var chapterSubmission interfaces.FactorySubmissionRecord

	proc := &execDirObservingProcessor{
		factoryDir:      dir,
		wantExecutionID: wantExecutionID,
	}

	h := support.NewGuardsBatchHarness(t, dir,
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
	h.SetCustomExecutor("processor", proc)
	h.MockWorker("completer", interfaces.WorkResult{Outcome: interfaces.OutcomeAccepted})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := support.RunGuardsBatchHarness(t, h, ctx)

	testutil.WriteDynamicExecutionFile(t, dir, "chapter", wantExecutionID, []byte(`{"title": "Dynamic execution directory guard test"}`))

	support.WaitForHarnessPlaceTokenCount(t, h, "chapter:complete", 1, 10*time.Second)

	submissionsMu.Lock()
	gotSubmission := chapterSubmission
	submissionsMu.Unlock()

	if gotSubmission.Request.ExecutionID != wantExecutionID {
		t.Fatalf("chapter submission execution ID = %q, want %q", gotSubmission.Request.ExecutionID, wantExecutionID)
	}
	if gotSubmission.Request.Tags[executionIDTagKey] != wantExecutionID {
		t.Fatalf("chapter submission execution tag = %q, want %q", gotSubmission.Request.Tags[executionIDTagKey], wantExecutionID)
	}
	if !proc.sawExecutionChannelValue() {
		t.Fatalf("processor never observed dynamic execution directory for %q", wantExecutionID)
	}
	if proc.dispatchCountValue() != 3 {
		t.Fatalf("processor dispatch count = %d, want 3", proc.dispatchCountValue())
	}

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("page:init")

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}
