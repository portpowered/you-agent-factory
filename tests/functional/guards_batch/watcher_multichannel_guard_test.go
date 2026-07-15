package guards_batch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/factory"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMultiChannelGuard_FileDropToCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	testutil.WriteSeedFile(t, dir, "chapter", []byte(`{"title": "Chapter via FileWatcher"}`))

	h := testutil.NewServiceTestHarness(t, dir)

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.MockWorker("processor",
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
	)
	h.MockWorker("completer", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

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
	var chapterSubmission work.FactorySubmissionRecord

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithExtraOptions(factory.WithSubmissionRecorder(func(record work.FactorySubmissionRecord) {
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
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
		workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted},
	)
	h.MockWorker("completer", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

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

	releaseCh := make(chan struct{})
	h := testutil.NewServiceTestHarness(t, dir, testutil.WithRunAsync())

	parserExec := &fanoutParserExecutor{childCount: 3}
	h.SetCustomExecutor("parser", parserExec)
	h.SetCustomExecutor("processor", &gatedProcessor{release: releaseCh})
	h.MockWorker("completer", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	errCh := h.RunInBackground(ctx)
	support.WaitForHarnessRuntimeAvailability(t, h, errCh, 15*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if parserExec.callCount() >= 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if parserExec.callCount() < 1 {
		t.Fatal("parser never fanned out page Work")
	}
	support.WaitForHarnessPlaceTokenCount(t, h, "chapter:complete", 0, time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "page:complete", 0, time.Second)

	close(releaseCh)

	support.WaitForHarnessPlaceTokenCount(t, h, "page:complete", 3, 10*time.Second)
	support.WaitForHarnessPlaceTokenCount(t, h, "chapter:complete", 1, 10*time.Second)

	h.Assert().
		PlaceTokenCount("chapter:complete", 1).
		PlaceTokenCount("page:complete", 3).
		HasNoTokenInPlace("chapter:processing").
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("page:init")

	cancel()
	if err := <-errCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("factory run error: %v", err)
	}
}

func TestMultiChannelGuard_DynamicExecDirWithGuard(t *testing.T) {
	const wantExecutionID = "exec-dynamic-guard"

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_input_guard_dir"))
	chapterDefaultDir := filepath.Join(dir, interfaces.InputsDir, "chapter", interfaces.DefaultChannelName)
	if err := os.MkdirAll(chapterDefaultDir, 0o755); err != nil {
		t.Fatalf("create chapter default channel: %v", err)
	}

	var submissionsMu sync.Mutex
	var chapterSubmission work.FactorySubmissionRecord

	proc := &execDirObservingProcessor{
		factoryDir:      dir,
		wantExecutionID: wantExecutionID,
	}

	h := support.NewGuardsBatchHarness(t, dir,
		testutil.WithExtraOptions(factory.WithSubmissionRecorder(func(record work.FactorySubmissionRecord) {
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
	h.MockWorker("completer", workerexecution.WorkResult{Outcome: workerexecution.OutcomeAccepted})

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
