package fixtures_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution/fixtures"
	workflowsource "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
)

func TestJavaScriptRuntimeService_StartAsync_IdempotentReplay(t *testing.T) {
	service := newJavaScriptRuntimeService(t)
	req := inlineWorkflowStartRequest(
		"req-runtime-async-idempotent-001",
		simpleFinalWorkflowSource,
		map[string]any{"subject": "workflows", "count": 1, "prefix": "you"},
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
	service := newJavaScriptRuntimeService(t)
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
		service := newJavaScriptRuntimeService(t)
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
		service := newJavaScriptRuntimeService(t)
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
		service := newJavaScriptRuntimeService(t)
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
	})

	t.Run("sync", func(t *testing.T) {
		service := newJavaScriptRuntimeService(t)
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

func TestJavaScriptRuntimeService_TypedFailures_MissingSessionMissingSourceBadSource(t *testing.T) {
	service := newJavaScriptRuntimeService(t)

	t.Run("missing session", func(t *testing.T) {
		_, err := service.GetSession(context.Background(), "dur-sess-runtime-missing-001")
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureSessionNotFound, "")
	})

	t.Run("missing source", func(t *testing.T) {
		_, err := service.StartAsync(context.Background(), fse.StartRequest{
			RequestID: "req-runtime-missing-source-001",
			Source: fse.Source{
				Kind:         workflowsource.KindWorkflowName,
				WorkflowName: "does-not-exist",
			},
		})
		assertRuntimeTypedFailure(t, err, fixtures.TypedFailureMalformedRequest, "source")
	})

	t.Run("bad source", func(t *testing.T) {
		projectRoot := t.TempDir()
		workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
		if err := os.MkdirAll(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir workflows: %v", err)
		}
		workflowPath := filepath.Join(workflowDir, "invalid.ts")
		if err := os.WriteFile(workflowPath, []byte(`import fs from "node:fs";`), 0o600); err != nil {
			t.Fatalf("write workflow: %v", err)
		}

		runtimeService, err := fse.NewExecutionService(
			fse.ExecutionProviderJavaScriptRuntime,
			fse.ServiceConfig{ProjectRoot: projectRoot},
		)
		if err != nil {
			t.Fatalf("NewExecutionService: %v", err)
		}

		_, err = runtimeService.StartSync(context.Background(), fse.StartRequest{
			RequestID: "req-runtime-bad-source-001",
			Source: fse.Source{
				Kind:         workflowsource.KindWorkflowName,
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
