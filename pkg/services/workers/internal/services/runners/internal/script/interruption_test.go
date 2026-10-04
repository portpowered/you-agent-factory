package script

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
)

func TestRunnerNormalizesCancellationAndDeadlineAfterCommandStart(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		runInterruptedScriptCase(t, false)
	})
	t.Run("deadline", func(t *testing.T) {
		runInterruptedScriptCase(t, true)
	})
}

func runInterruptedScriptCase(t *testing.T, deadline bool) {
	t.Helper()
	observations := &observationLog{}
	commandEdge := &interruptingCommandEdge{
		started:      make(chan struct{}),
		observations: observations,
	}
	started := time.Unix(100, 0).In(time.FixedZone("selected", 3600))
	scriptRunner, err := New(Config{Command: "long-running-script"}, Dependencies{
		CommandRunner: commandEdge,
		FactoryDocs:   emptyDocs,
		Now:           (&sequenceClock{times: []time.Time{started, started.Add(3 * time.Second)}}).Now,
		Publish: func(fragment workers.ProgressFragment) {
			observations.CaptureProgress(fragment)
			observations.Append(fragment.Type + ":" + fragment.Payload)
		},
		Record: func(event workers.ScriptEvent) {
			observations.CaptureEvent(event)
			if event.Request != nil {
				observations.Append("request")
			}
			if event.Response != nil {
				observations.Append("terminal")
				observations.SetTerminal(*event.Response)
			}
		},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	var executionContext context.Context = ctx
	if deadline {
		controlled := &controlledDeadlineContext{Context: context.Background(), done: make(chan struct{})}
		executionContext = controlled
		cancel = func() { controlled.expire() }
	}
	defer cancel()
	type executionOutcome struct {
		result workers.RunnerExecutionResult
		err    error
	}
	request := observedRequest()
	done := make(chan executionOutcome, 1)
	go func() {
		result, executeErr := scriptRunner.Execute(executionContext, request)
		done <- executionOutcome{result: result, err: executeErr}
	}()
	<-commandEdge.started
	cancel()
	outcome := <-done

	assertInterruptedScriptOutcome(t, outcome.result, outcome.err, deadline)
	if !commandEdge.Cleaned() {
		t.Fatal("Execute returned before injected command cleanup completed")
	}
	wantOrder := []string{
		"request",
		"command",
		"stdout:partial stdout",
		"stderr:partial stderr",
		"terminal",
	}
	if got := observations.Values(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("observation order = %#v, want %#v", got, wantOrder)
	}
	assertAttributedObservations(t, observations, request, started, started.Add(3*time.Second), "long-running-script", nil, []outputChunk{
		{stream: platformprocess.OutputStreamStdout, payload: "partial stdout"},
		{stream: platformprocess.OutputStreamStderr, payload: "partial stderr"},
	})
	assertInterruptedTerminal(t, observations.Terminal(), deadline)
}

func assertInterruptedScriptOutcome(
	t *testing.T,
	result workers.RunnerExecutionResult,
	err error,
	deadline bool,
) {
	t.Helper()
	wantCause := context.Canceled
	if deadline {
		wantCause = context.DeadlineExceeded
		assertFailureType(t, err, workers.WorkFailureTypeTimeout)
	}
	if !errors.Is(err, wantCause) {
		t.Fatalf("Execute() error = %v, want cause %v", err, wantCause)
	}
	command := result.Diagnostics.Command
	if result.Content != "partial stdout" ||
		command.Stdout != "partial stdout" ||
		command.Stderr != "partial stderr" ||
		command.TimedOut != deadline {
		t.Fatalf("interrupted result = %#v", result)
	}
}

func assertInterruptedTerminal(
	t *testing.T,
	terminal workers.ScriptResponseEventPayload,
	deadline bool,
) {
	t.Helper()
	wantOutcome := workers.ScriptExecutionOutcomeCanceled
	wantFailure := workers.ScriptFailureTypeCanceled
	if deadline {
		wantOutcome = workers.ScriptExecutionOutcomeTimedOut
		wantFailure = workers.ScriptFailureTypeTimeout
	}
	assertObservedValue(t, "interruption outcome", terminal.Outcome, wantOutcome)
	assertObservedValue(t, "interruption failure type", terminal.FailureType, &wantFailure)
	assertObservedValue(t, "interruption exit code", terminal.ExitCode, (*int)(nil))
	assertObservedValue(t, "interruption stdout", terminal.Stdout, "partial stdout")
	assertObservedValue(t, "interruption stderr", terminal.Stderr, "partial stderr")
	assertObservedValue(t, "interruption duration", terminal.DurationMillis, int64(3000))
}

type interruptingCommandEdge struct {
	mu           sync.Mutex
	started      chan struct{}
	observations *observationLog
	cleaned      bool
}

func (edge *interruptingCommandEdge) Run(
	ctx context.Context,
	request workerprocess.CommandRequest,
) (workerprocess.CommandResult, error) {
	return edge.RunStreaming(ctx, request, nil)
}

func (edge *interruptingCommandEdge) RunStreaming(
	ctx context.Context,
	_ workerprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (workerprocess.CommandResult, error) {
	edge.observations.Append("command")
	if observer != nil {
		observer(platformprocess.OutputStreamStdout, []byte("partial stdout"))
		observer(platformprocess.OutputStreamStderr, []byte("partial stderr"))
	}
	close(edge.started)
	<-ctx.Done()
	edge.mu.Lock()
	edge.cleaned = true
	edge.mu.Unlock()
	return workerprocess.CommandResult{
		Stdout: []byte("partial stdout"),
		Stderr: []byte("partial stderr"),
	}, ctx.Err()
}

func (edge *interruptingCommandEdge) Cleaned() bool {
	edge.mu.Lock()
	defer edge.mu.Unlock()
	return edge.cleaned
}

// Expiry is released after command readiness; no wall clock drives this case.
type controlledDeadlineContext struct {
	context.Context
	done chan struct{}
	once sync.Once
}

func (ctx *controlledDeadlineContext) Deadline() (time.Time, bool) {
	return time.Unix(200, 0), true
}

func (ctx *controlledDeadlineContext) Done() <-chan struct{} { return ctx.done }

func (ctx *controlledDeadlineContext) Err() error {
	select {
	case <-ctx.done:
		return context.DeadlineExceeded
	default:
		return nil
	}
}

func (ctx *controlledDeadlineContext) expire() { ctx.once.Do(func() { close(ctx.done) }) }
