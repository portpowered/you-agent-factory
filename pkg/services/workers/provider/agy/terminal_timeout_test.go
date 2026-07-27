package agy_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
)

func TestAdapterParseFinalTimeoutEmitsErrorThenFailedRun(t *testing.T) {
	t.Parallel()

	providerAdapter := agy.NewAdapter(t.TempDir())
	result, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{ExitCode: 124, Stdout: nil},
		CommandError:  agypty.ErrSessionTimedOut,
		FlushReason:   adapter.FlushReasonCanceled,
		RunID:         "run-empty-timeout",
		DispatchID:    "dispatch-empty-timeout",
	})
	if err == nil {
		t.Fatal("ParseFinal() error = nil, want timeout failure")
	}
	assertTimeoutTerminalDrafts(t, result.Drafts, "", false)
}

func TestAdapterParseFinalTimeoutNonemptyCaptureOrdersPartialErrorFailedRun(t *testing.T) {
	t.Parallel()

	providerAdapter := agy.NewAdapter(t.TempDir())
	result, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 124,
			Stdout:   []byte("partial answer before timeout"),
		},
		CommandError: agypty.ErrSessionTimedOut,
		FlushReason:  adapter.FlushReasonCanceled,
		RunID:        "run-partial-timeout",
		DispatchID:   "dispatch-partial-timeout",
	})
	if err == nil {
		t.Fatal("ParseFinal() error = nil, want timeout failure")
	}
	assertTimeoutTerminalDrafts(t, result.Drafts, "partial answer before timeout", true)
}

func TestAdapterExecuteTimeoutEmitsDeterministicTerminalOrdering(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &failureStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}}
	providerAdapter := agy.NewAdapterWithDependencies(
		factoryRoot, mock, "agy", agypty.SessionConfig{}, executableDependencies(nil),
	)
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	runner, err := providerAdapter.PTYRunner()
	if err != nil {
		t.Fatalf("PTYRunner() error = %v", err)
	}
	result, executeErr := adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command: adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-timeout"},
			WorkingDirectory: ".",
			UserMessage:      "plan the goal",
		}},
		Decoder: adapter.DecoderContext{RunID: "run-agy-timeout", DispatchID: "dispatch-agy-timeout"},
	})
	if executeErr == nil {
		t.Fatal("Execute() error = nil, want timeout failure")
	}
	assertFailureFacts(t, adapter.FailureResult{Failure: result.Failure},
		workerexecution.WorkFailureTypeTimeout, "Agy request timed out.", true)
	assertTimeoutTerminalDrafts(t, result.Drafts, "partial answer before timeout", true)
}

func TestAdapterParseFinalTimeoutClassifiedDistinctlyFromAuthFailure(t *testing.T) {
	t.Parallel()

	providerAdapter := agy.NewAdapter(t.TempDir())
	result, err := providerAdapter.ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 1,
			Stdout:   []byte("Error: authentication failed: invalid api key"),
		},
		CommandError: errors.New("exit code 1"),
		RunID:        "run-auth-failure",
		DispatchID:   "dispatch-auth-failure",
	})
	if err == nil {
		t.Fatal("ParseFinal() error = nil, want auth failure")
	}
	if len(result.Drafts) != 0 {
		t.Fatalf("drafts = %#v, want no timeout terminal drafts for auth failure", result.Drafts)
	}
	failure := providerAdapter.ClassifyFailure(context.Background(), adapter.FailureContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 1,
			Stdout:   []byte("Error: authentication failed: invalid api key"),
		},
		CommandError: errors.New("exit code 1"),
		ParseError:   err,
	})
	assertFailureFacts(t, failure, workerexecution.WorkFailureTypeAuthFailure, "Agy authentication failed.", false)
}

func assertTimeoutTerminalDrafts(t *testing.T, drafts []factorysessions.ResponseEventDraft, partialText string, wantPartial bool) {
	t.Helper()

	wantCount := 2
	if wantPartial {
		wantCount = 3
	}
	if len(drafts) != wantCount {
		t.Fatalf("draft count = %d, want %d: %#v", len(drafts), wantCount, drafts)
	}

	index := 0
	if wantPartial {
		assertPartialTimeoutMessageDraft(t, drafts[index], partialText)
		index++
	}
	assertTimeoutErrorDraft(t, drafts[index])
	assertTimeoutFailedRunDraft(t, drafts[index+1])
}

func assertTimeoutErrorDraft(t *testing.T, errorDraft factorysessions.ResponseEventDraft) {
	t.Helper()

	if err := factorysessions.ValidateResponseEventDraft(errorDraft); err != nil {
		t.Fatalf("timeout error draft invalid: %v", err)
	}
	if errorDraft.Kind != factorysessions.ResponseEventKindError || errorDraft.Phase != factorysessions.ResponseEventPhaseFailed {
		t.Fatalf("timeout error draft = %#v, want ERROR/FAILED", errorDraft)
	}
	if errorDraft.Provenance.NativeEventType != "session_timeout" {
		t.Fatalf("timeout error native type = %q, want session_timeout", errorDraft.Provenance.NativeEventType)
	}
	var errorPayload factorysessions.ResponseEventErrorPayload
	if err := json.Unmarshal(errorDraft.Payload, &errorPayload); err != nil {
		t.Fatalf("unmarshal timeout error payload: %v", err)
	}
	if errorPayload.Code != string(workerexecution.WorkFailureTypeTimeout) {
		t.Fatalf("timeout error code = %q, want %q", errorPayload.Code, workerexecution.WorkFailureTypeTimeout)
	}
	if errorPayload.Message != "Agy request timed out." {
		t.Fatalf("timeout error message = %q, want actionable timeout text", errorPayload.Message)
	}
	if !errorPayload.Retryable {
		t.Fatal("timeout error retryable = false, want true")
	}
}

func assertTimeoutFailedRunDraft(t *testing.T, failedRun factorysessions.ResponseEventDraft) {
	t.Helper()

	if err := factorysessions.ValidateResponseEventDraft(failedRun); err != nil {
		t.Fatalf("failed run draft invalid: %v", err)
	}
	if failedRun.Kind != factorysessions.ResponseEventKindRun || failedRun.Phase != factorysessions.ResponseEventPhaseFailed {
		t.Fatalf("failed run draft = %#v, want RUN/FAILED", failedRun)
	}
	var runPayload factorysessions.ResponseEventRun
	if err := json.Unmarshal(failedRun.Payload, &runPayload); err != nil {
		t.Fatalf("unmarshal failed run payload: %v", err)
	}
	if runPayload.Status != "failed" {
		t.Fatalf("failed run status = %q, want failed", runPayload.Status)
	}
	if failedRun.Provenance.NativeEventType != "command_completion" {
		t.Fatalf("failed run native type = %q, want command_completion", failedRun.Provenance.NativeEventType)
	}
}
