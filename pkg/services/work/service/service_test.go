package service_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workservice "github.com/portpowered/infinite-you/pkg/services/work/service"
)

type recordingFactory struct {
	submitted work.WorkRequest
	movedID   string
	source    work.WorkStateChangeSource
}

type workRuntimeResolver struct {
	runtime work.Runtime
	err     error
}

func (r workRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return r.runtime, r.err
}

func TestNewServiceRoutesThroughWorkRootRuntimeContract(t *testing.T) {
	runtime := &recordingFactory{}
	service := workservice.NewService(workRuntimeResolver{runtime: runtime}, os.ReadFile, nil, nil)

	request := work.WorkRequest{RequestID: "request-root-contract"}
	if _, err := service.SubmitWorkRequestForSession(
		context.Background(),
		"session-1",
		request,
	); err != nil {
		t.Fatalf("SubmitWorkRequestForSession: %v", err)
	}
	if _, err := service.MoveWorkForSession(
		context.Background(),
		"session-1",
		"work-1",
		"done",
		"move-1",
	); err != nil {
		t.Fatalf("MoveWorkForSession: %v", err)
	}
	if runtime.submitted.RequestID != request.RequestID ||
		runtime.movedID != "work-1" ||
		runtime.source != work.WorkStateChangeSourceAPI {
		t.Fatalf(
			"routed calls = (%q, %q, %q)",
			runtime.submitted.RequestID,
			runtime.movedID,
			runtime.source,
		)
	}
}

func (f *recordingFactory) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.submitted = request
	return work.WorkRequestSubmitResult{}, nil
}

func (f *recordingFactory) MoveWork(_ context.Context, workID, _ string, source work.WorkStateChangeSource, _ string) (work.OperatorMoveResult, error) {
	f.movedID, f.source = workID, source
	return work.OperatorMoveResult{}, nil
}

func (f *recordingFactory) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}

func TestNewServicePropagatesRuntimeResolverError(t *testing.T) {
	service := workservice.NewService(workRuntimeResolver{err: factorysessions.ErrSessionNotFound}, os.ReadFile, nil, nil)
	_, err := service.SubmitWorkRequestForSession(context.Background(), "missing", work.WorkRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestSubmitFileParsesAndSubmitsCanonicalWorkRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "work.json")
	if err := os.WriteFile(path, []byte(`{
		"requestId": "request-from-file",
		"type": "FACTORY_REQUEST_BATCH",
		"works": [{"name": "work-1", "workTypeName": "test", "state": "init", "payload": {"value": "hello"}}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	target := &recordingFactory{}

	if err := workservice.SubmitFile(context.Background(), path, target, os.ReadFile); err != nil {
		t.Fatalf("SubmitFile: %v", err)
	}
	if target.submitted.RequestID != "request-from-file" {
		t.Fatalf("request ID = %q, want request-from-file", target.submitted.RequestID)
	}
}

func TestSubmitFileForSessionUsesInjectedReaderAndRuntime(t *testing.T) {
	runtime := &recordingFactory{}
	readPath := ""
	service := workservice.NewService(workRuntimeResolver{runtime: runtime}, func(path string) ([]byte, error) {
		readPath = path
		return []byte(`{"requestId":"request-edge","type":"FACTORY_REQUEST_BATCH","works":[]}`), nil
	}, nil, nil)

	result, err := service.SubmitFileForSession(context.Background(), "session-1", "edge.json")
	if err != nil {
		t.Fatalf("SubmitFileForSession: %v", err)
	}
	if readPath != "edge.json" || runtime.submitted.RequestID != "request-edge" || result.RequestID != "" {
		t.Fatalf("submitted file route = (%q, %q, %#v)", readPath, runtime.submitted.RequestID, result)
	}
}

func TestSubmitFileFailsClosedWithoutReader(t *testing.T) {
	err := workservice.SubmitFile(context.Background(), "work.json", &recordingFactory{}, nil)
	if err == nil || !strings.Contains(err.Error(), "file reader is required") {
		t.Fatalf("error = %v, want missing submitted-file reader failure", err)
	}
}

func TestSubmitFileReportsReadParseAndRuntimeFailures(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		err := workservice.SubmitFile(context.Background(), filepath.Join(t.TempDir(), "missing.json"), &recordingFactory{}, os.ReadFile)
		if err == nil || !strings.Contains(err.Error(), "read work file") {
			t.Fatalf("error = %v, want read work file failure", err)
		}
	})
	t.Run("parse", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "work.json")
		if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
			t.Fatal(err)
		}
		err := workservice.SubmitFile(context.Background(), path, &recordingFactory{}, os.ReadFile)
		if err == nil || !strings.Contains(err.Error(), "parse work file") {
			t.Fatalf("error = %v, want parse work file failure", err)
		}
	})
	t.Run("runtime", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "work.json")
		if err := os.WriteFile(path, []byte(`{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		err := workservice.SubmitFile(context.Background(), path, nil, os.ReadFile)
		if err == nil || !strings.Contains(err.Error(), "factory runtime is not available") {
			t.Fatalf("error = %v, want runtime unavailable failure", err)
		}
	})
}

func TestNewServiceExposesInvocationAndReturnPolicySlice(t *testing.T) {
	service := workservice.NewService(workRuntimeResolver{runtime: &recordingFactory{}}, os.ReadFile, nil, nil)
	ctx := context.Background()

	stdin := "from service root"
	prepared, err := service.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		Arguments: []string{"-"},
		StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != stdin {
		t.Fatalf("prepared = %#v, want stdin text", prepared)
	}

	_, err = service.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{
		Arguments: []string{""},
	})
	if !errors.Is(err, work.ErrInvalidInvocationInput) {
		t.Fatalf("PrepareInvocationInput error = %v, want ErrInvalidInvocationInput", err)
	}

	_, err = service.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{
		RequestID: "request-1",
		InvocationReturn: &work.InvocationReturnConfig{
			Policy: "NOT_A_POLICY",
		},
		WorldState: work.InvocationWorldState{
			WorkRequestsByID: map[string]work.InvocationWorkRequest{
				"request-1": {WorkItems: []work.FactoryWorkItem{{ID: "work-1"}}},
			},
		},
	})
	if !errors.Is(err, work.ErrUnsupportedReturnPolicy) {
		t.Fatalf("ResolvePrimaryResult error = %v, want ErrUnsupportedReturnPolicy", err)
	}
}
