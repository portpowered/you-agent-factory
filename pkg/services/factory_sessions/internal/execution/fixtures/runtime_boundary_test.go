package fixtures_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil/factoryruntimefixtures"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/fixtures"
)

func TestJavaScriptRuntimeService_StartAsync_IdempotentReplay(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())
	req := inlineWorkflowStartRequest(
		"req-runtime-async-idempotent-001",
		busyLoopWorkflowSource,
		nil,
		nil,
	)

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	second, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("replay StartAsync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	if second.Status != first.Status {
		t.Fatalf("replay status = %q, want %q", second.Status, first.Status)
	}
}

func TestJavaScriptRuntimeService_StartSync_IdempotentReplay(t *testing.T) {
	service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
	req := inlineWorkflowStartRequest(
		"req-runtime-sync-idempotent-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
		nil,
	)

	first, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartSync: %v", err)
	}
	second, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("replay StartSync: %v", err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", second.SessionID, first.SessionID)
	}
	if second.SyncOutcome != first.SyncOutcome {
		t.Fatalf("replay syncOutcome = %q, want %q", second.SyncOutcome, first.SyncOutcome)
	}
}

func TestJavaScriptRuntimeService_Start_ExecutionRequestIDConflict(t *testing.T) {
	t.Run("async", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		req := inlineWorkflowStartRequest(
			"req-runtime-async-conflict-001",
			simpleFinalWorkflowSource,
			map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
			nil,
		)
		if _, err := service.StartAsync(context.Background(), req); err != nil {
			t.Fatalf("StartAsync: %v", err)
		}

		conflict := req
		conflict.Args = map[string]any{"subject": "different"}
		_, err := service.StartAsync(context.Background(), conflict)
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureExecutionRequestConflict, "")
	})

	t.Run("sync", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		req := inlineWorkflowStartRequest(
			"req-runtime-sync-conflict-001",
			simpleFinalWorkflowSource,
			map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
			nil,
		)
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync: %v", err)
		}

		conflict := req
		conflict.Args = map[string]any{"subject": "different"}
		_, err := service.StartSync(context.Background(), conflict)
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureExecutionRequestConflict, "")
	})
}

func TestJavaScriptRuntimeService_Start_ConcurrentIdempotentStarts(t *testing.T) {
	t.Run("async", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		const workers = 12
		var wg sync.WaitGroup
		results := make([]fse.AsyncStartResult, workers)
		errs := make([]error, workers)
		req := inlineWorkflowStartRequest(
			"req-runtime-async-concurrent-001",
			simpleFinalWorkflowSource,
			map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
			nil,
		)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index], errs[index] = service.StartAsync(context.Background(), req)
			}(i)
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("StartAsync worker %d: %v", i, err)
			}
		}
		for i := 1; i < workers; i++ {
			if results[i].SessionID != results[0].SessionID {
				t.Fatalf("sessionId[%d] = %q, want %q", i, results[i].SessionID, results[0].SessionID)
			}
		}
		for i, started := range results {
			assertAsyncStartInitialized(t, i, started)
		}
	})

	t.Run("sync", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		const workers = 12
		var wg sync.WaitGroup
		results := make([]fse.SyncStartResult, workers)
		errs := make([]error, workers)
		req := inlineWorkflowStartRequest(
			"req-runtime-sync-concurrent-001",
			simpleFinalWorkflowSource,
			map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
			nil,
		)
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(index int) {
				defer wg.Done()
				results[index], errs[index] = service.StartSync(context.Background(), req)
			}(i)
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("StartSync worker %d: %v", i, err)
			}
		}
		for i := 1; i < workers; i++ {
			if results[i].SessionID != results[0].SessionID {
				t.Fatalf("sessionId[%d] = %q, want %q", i, results[i].SessionID, results[0].SessionID)
			}
		}
	})
}

func TestJavaScriptRuntimeService_StartAsyncReplay_PreservesFirstObservedRunningResponse(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())
	maxRunDurationMs := int64(200)
	req := inlineWorkflowStartRequest(
		"req-runtime-async-replay-running-001",
		busyLoopWorkflowSource,
		map[string]any{"subject": "workflows"},
		map[string]any{"maxRunDurationMs": maxRunDurationMs},
	)

	first, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartAsync: %v", err)
	}
	if first.Status != string(fse.LifecycleStatusRunning) {
		t.Fatalf("first status = %q, want RUNNING", first.Status)
	}

	waitUntilSessionStatus(t, service, first.SessionID, fse.LifecycleStatusTimedOut, 5*time.Second)

	replay, err := service.StartAsync(context.Background(), req)
	if err != nil {
		t.Fatalf("replay StartAsync: %v", err)
	}
	if replay.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", replay.SessionID, first.SessionID)
	}
	if replay.Status != first.Status {
		t.Fatalf("replay status = %q, want preserved first status %q", replay.Status, first.Status)
	}
}

func TestJavaScriptRuntimeService_StartSyncWaitTimeoutReplay_PreservesFirstObservedTimedOutResponse(t *testing.T) {
	service := newJavaScriptRuntimeService(t, blockingFixtureWorkflows())
	waitMillis := int64(50)
	maxRunDurationMs := int64(200)
	req := fse.StartRequest{
		RequestID: "req-runtime-sync-replay-wait-timeout-001",
		Source: fse.Source{
			Kind: factory.WorkflowSourceKindInlineWorkflow,
			InlineWorkflow: &fse.InlineWorkflowSource{
				InlineSource: busyLoopWorkflowSource,
				Dialect:      "you-workflow-v1",
				Metadata:     map[string]string{"name": "runtime-sync-replay-fixture"},
			},
		},
		Args: map[string]any{"subject": "workflows"},
		RequestedPolicy: map[string]any{
			"maxRunDurationMs": maxRunDurationMs,
		},
		Wait: &fse.WaitOptions{TimeoutMillis: &waitMillis},
	}

	first, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("first StartSync: %v", err)
	}
	if first.SyncOutcome != fse.SyncOutcomeTimedOut || !first.TimedOut {
		t.Fatalf("first sync response = %#v, want TIMED_OUT", first)
	}
	if first.Status != string(fse.LifecycleStatusRunning) && first.Status != string(fse.LifecycleStatusTimedOut) {
		t.Fatalf("first status = %q, want RUNNING or TIMED_OUT", first.Status)
	}

	if first.Status != string(fse.LifecycleStatusTimedOut) {
		waitUntilSessionStatus(t, service, first.SessionID, fse.LifecycleStatusTimedOut, 5*time.Second)
	}

	replay, err := service.StartSync(context.Background(), req)
	if err != nil {
		t.Fatalf("replay StartSync: %v", err)
	}
	if replay.SessionID != first.SessionID {
		t.Fatalf("replay sessionId = %q, want %q", replay.SessionID, first.SessionID)
	}
	if replay.SyncOutcome != first.SyncOutcome || !replay.TimedOut {
		t.Fatalf("replay syncOutcome = %#v, want preserved TIMED_OUT %#v", replay, first)
	}
	if replay.Status != first.Status {
		t.Fatalf("replay status = %q, want preserved first status %q", replay.Status, first.Status)
	}
}

func TestJavaScriptRuntimeService_Start_CrossModeRequestIDConflict(t *testing.T) {
	t.Run("sync after async", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		req := inlineWorkflowStartRequest(
			"req-runtime-cross-mode-async-then-sync-001",
			simpleFinalWorkflowSource,
			map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
			nil,
		)
		if _, err := service.StartAsync(context.Background(), req); err != nil {
			t.Fatalf("StartAsync: %v", err)
		}
		_, err := service.StartSync(context.Background(), req)
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureExecutionRequestConflict, "")
	})

	t.Run("async after sync", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		req := inlineWorkflowStartRequest(
			"req-runtime-cross-mode-sync-then-async-001",
			simpleFinalWorkflowSource,
			map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
			nil,
		)
		if _, err := service.StartSync(context.Background(), req); err != nil {
			t.Fatalf("StartSync: %v", err)
		}
		_, err := service.StartAsync(context.Background(), req)
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureExecutionRequestConflict, "")
	})
}

func TestJavaScriptRuntimeService_Start_RejectsInvalidWaitAndPolicy(t *testing.T) {
	t.Run("negative sync wait timeout", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		negative := int64(-1)
		req := inlineWorkflowStartRequest(
			"req-runtime-invalid-wait-001",
			simpleFinalWorkflowSource,
			nil,
			nil,
		)
		req.Wait = &fse.WaitOptions{TimeoutMillis: &negative}

		_, err := service.StartSync(context.Background(), req)
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureMalformedRequest, "wait.timeoutMillis")
	})

	t.Run("invalid requested policy", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))
		req := inlineWorkflowStartRequest(
			"req-runtime-invalid-policy-001",
			simpleFinalWorkflowSource,
			nil,
			map[string]any{"allowNetwork": true},
		)

		_, err := service.StartAsync(context.Background(), req)
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureMalformedRequest, "requestedPolicy")
	})
}

func TestJavaScriptRuntimeService_TypedFailures_MissingSessionMissingSourceBadSource(t *testing.T) {
	service := newJavaScriptRuntimeService(t, successfulFixtureWorkflows(map[string]any{"status": "done"}))

	t.Run("missing session", func(t *testing.T) {
		_, err := service.GetSession(context.Background(), "dur-sess-runtime-missing-001")
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureSessionNotFound, "")
	})

	t.Run("missing source", func(t *testing.T) {
		missingSourceService := newJavaScriptRuntimeService(t, factoryruntimefixtures.ScriptedJavaScriptWorkflows{
			ResolveSourceFunc: func(factory.WorkflowSourceRequest, factory.WorkflowSourceContext) factory.WorkflowSourceResolution {
				return factory.WorkflowSourceResolution{}
			},
		})
		_, err := missingSourceService.StartAsync(context.Background(), fse.StartRequest{
			RequestID: "req-runtime-missing-source-001",
			Source: fse.Source{
				Kind:         factory.WorkflowSourceKindWorkflowName,
				WorkflowName: "does-not-exist",
			},
		})
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureMalformedRequest, "source")
	})

	t.Run("bad source", func(t *testing.T) {
		workflows := factoryruntimefixtures.ScriptedJavaScriptWorkflows{
			LoadSourceFunc: func(factory.WorkflowValidationLoadRequest) (factory.WorkflowValidationLoadedSource, []factory.WorkflowValidationIssue) {
				return factory.WorkflowValidationLoadedSource{}, []factory.WorkflowValidationIssue{{
					Code:    factory.WorkflowValidationCodeUnsupportedLoader,
					Message: "scripted authored-source rejection",
				}}
			},
		}
		runtimeService, err := newExecutionService(
			fse.ExecutionProviderJavaScriptRuntime,
			executionServiceConfig{
				ProjectRoot: t.TempDir(), Persistence: fse.DisabledPersistence(),
				Clock: fixtureClock{now: time.Now()}, Workflows: workflows,
			},
		)
		if err != nil {
			t.Fatalf("NewExecutionService: %v", err)
		}

		_, err = runtimeService.StartSync(context.Background(), fse.StartRequest{
			RequestID: "req-runtime-bad-source-001",
			Source: fse.Source{
				Kind:         factory.WorkflowSourceKindWorkflowName,
				WorkflowName: "invalid",
			},
		})
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureMalformedRequest, "source")
	})

	t.Run("missing session result read", func(t *testing.T) {
		_, err := service.GetResult(context.Background(), "dur-sess-runtime-missing-002", fse.ResultRequest{
			Mode: fse.ResultModeFinal,
		})
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureSessionNotFound, "")
	})
}

func assertRuntimeTypedFailure(
	t *testing.T,
	err error,
	wantKind fixtures.TypedFailureKind,
	wantField string,
) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil, want typed service failure")
	}
	assertServiceBoundaryError(t, err)

	identity, ok := fixtures.TypedFailureIdentityFromError(err)
	if !ok {
		t.Fatalf("error = %v, want mappable typed failure identity", err)
	}
	if identity.Kind != wantKind {
		t.Fatalf("identity kind = %q, want %q", identity.Kind, wantKind)
	}
	if wantField != "" && identity.Field != wantField {
		t.Fatalf("identity field = %q, want %q", identity.Field, wantField)
	}
}

func assertServiceBoundaryError(t *testing.T, err error) {
	t.Helper()
	switch {
	case errors.As(err, new(*fse.ValidationError)):
	case errors.Is(err, fse.ErrSessionNotFound):
	case errors.Is(err, fse.ErrExecutionRequestIDConflict):
	case errors.As(err, new(*fse.ControlError)):
	default:
		t.Fatalf("error = %T %v, want ValidationError or service sentinel", err, err)
	}
}

func assertAsyncStartInitialized(t *testing.T, worker int, started fse.AsyncStartResult) {
	t.Helper()
	if started.SessionID == "" {
		t.Fatalf("worker %d: expected durable session id", worker)
	}
	if started.Status == "" {
		t.Fatalf("worker %d: status = %q, want initialized lifecycle status", worker, started.Status)
	}
	if started.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("worker %d: orchestratorKind = %q, want JAVASCRIPT", worker, started.OrchestratorKind)
	}
	if started.ResolvedSource.SourceRef == "" || started.SourceHash == "" {
		t.Fatalf("worker %d: resolved source = %#v", worker, started.ResolvedSource)
	}
	if started.Links.Session == "" || started.Links.Status == "" || started.Links.Results == "" {
		t.Fatalf("worker %d: links = %#v, want session inspection links", worker, started.Links)
	}
}
