package internal_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
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

type rootOnlyRuntime struct{ factoryruntime.Service }

type rootRuntimeResolver struct {
	runtime *factorysessions.LiveRuntime
}

func (r rootRuntimeResolver) Resolve(string) *factorysessions.LiveRuntime {
	return r.runtime
}

func (r workRuntimeResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return r.runtime, r.err
}

func TestNewServiceRoutesThroughWorkRootRuntimeContract(t *testing.T) {
	runtime := &recordingFactory{}
	service := internalservice.NewService(workRuntimeResolver{runtime: runtime}, os.ReadFile, nil, nil)

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
	service := internalservice.NewService(workRuntimeResolver{err: factorysessions.ErrSessionNotFound}, os.ReadFile, nil, nil)
	_, err := service.SubmitWorkRequestForSession(context.Background(), "missing", work.WorkRequest{})
	if !errors.Is(err, factorysessions.ErrSessionNotFound) {
		t.Fatalf("error = %v, want ErrSessionNotFound", err)
	}
}

func TestLegacySessionOperationsFailClosedForRootOnlyRuntime(t *testing.T) {
	service := internalservice.New(rootRuntimeResolver{runtime: &factorysessions.LiveRuntime{
		Factory: rootOnlyRuntime{},
	}})
	ctx := context.Background()

	if _, err := service.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{}); err == nil ||
		!strings.Contains(err.Error(), "legacy Factory Runtime submission is required") {
		t.Fatalf("SubmitWorkRequestForSession error = %v, want missing legacy submission capability", err)
	}
	if _, err := service.MoveWorkForSession(ctx, "session-1", "work-1", "done", "move-1"); err == nil ||
		!strings.Contains(err.Error(), "legacy Factory Runtime work move is required") {
		t.Fatalf("MoveWorkForSession error = %v, want missing legacy move capability", err)
	}
	if _, err := service.SubscribeFactoryEventsForSession(ctx, "session-1", nil); err == nil ||
		!strings.Contains(err.Error(), "legacy Factory Runtime event subscription is required") {
		t.Fatalf("SubscribeFactoryEventsForSession error = %v, want missing legacy event capability", err)
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

	if err := internalservice.SubmitFile(context.Background(), path, target, os.ReadFile); err != nil {
		t.Fatalf("SubmitFile: %v", err)
	}
	if target.submitted.RequestID != "request-from-file" {
		t.Fatalf("request ID = %q, want request-from-file", target.submitted.RequestID)
	}
}

func TestSubmitFileForSessionUsesInjectedReaderAndRuntime(t *testing.T) {
	runtime := &recordingFactory{}
	readPath := ""
	service := internalservice.NewService(workRuntimeResolver{runtime: runtime}, func(path string) ([]byte, error) {
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
	err := internalservice.SubmitFile(context.Background(), "work.json", &recordingFactory{}, nil)
	if err == nil || !strings.Contains(err.Error(), "file reader is required") {
		t.Fatalf("error = %v, want missing submitted-file reader failure", err)
	}
}

func TestSubmitFileReportsReadParseAndRuntimeFailures(t *testing.T) {
	t.Run("read", func(t *testing.T) {
		err := internalservice.SubmitFile(context.Background(), filepath.Join(t.TempDir(), "missing.json"), &recordingFactory{}, os.ReadFile)
		if err == nil || !strings.Contains(err.Error(), "read work file") {
			t.Fatalf("error = %v, want read work file failure", err)
		}
	})
	t.Run("parse", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "work.json")
		if err := os.WriteFile(path, []byte(`{`), 0o600); err != nil {
			t.Fatal(err)
		}
		err := internalservice.SubmitFile(context.Background(), path, &recordingFactory{}, os.ReadFile)
		if err == nil || !strings.Contains(err.Error(), "parse work file") {
			t.Fatalf("error = %v, want parse work file failure", err)
		}
	})
	t.Run("runtime", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "work.json")
		if err := os.WriteFile(path, []byte(`{"requestId":"request-1","type":"FACTORY_REQUEST_BATCH","works":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		err := internalservice.SubmitFile(context.Background(), path, nil, os.ReadFile)
		if err == nil || !strings.Contains(err.Error(), "factory runtime is not available") {
			t.Fatalf("error = %v, want runtime unavailable failure", err)
		}
	})
}

func TestNewServiceExposesInvocationAndReturnPolicySlice(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: &recordingFactory{}}, os.ReadFile, nil, nil)
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

type recordingContentStaging struct {
	stageReq   work.StageContentRequest
	prepareIn  []work.StagedSubmissionItem
	resolveRef string
	cleanupRef string
}

func (s *recordingContentStaging) StageContent(
	_ context.Context,
	request work.StageContentRequest,
) (work.StageContentResult, error) {
	s.stageReq = request
	return work.StageContentResult{
		StagedFileRef: "submit-work-stage:v1:svc",
		FileName:      request.FileName,
		MediaType:     request.MediaType,
		URL:           "file:///tmp/svc.png",
	}, nil
}

func (s *recordingContentStaging) PrepareContent(
	_ context.Context,
	items []work.StagedSubmissionItem,
) ([]work.WorkContentPart, error) {
	s.prepareIn = items
	return []work.WorkContentPart{{
		Type: work.WorkContentPartTypeImage, URL: "file:///tmp/svc.png", ContentType: "image/png",
	}}, nil
}

func (s *recordingContentStaging) ResolveContent(
	_ context.Context,
	ref string,
) (work.ResolvedStagedContent, error) {
	s.resolveRef = ref
	return work.ResolvedStagedContent{Path: "/tmp/svc.png", URL: "file:///tmp/svc.png"}, nil
}

func (s *recordingContentStaging) CleanupContent(_ context.Context, ref string) error {
	s.cleanupRef = ref
	return nil
}

func TestNewServiceDelegatesContentStagingSlice(t *testing.T) {
	staging := &recordingContentStaging{}
	service := internalservice.NewService(
		workRuntimeResolver{runtime: &recordingFactory{}},
		os.ReadFile,
		staging,
		nil,
	)
	ctx := context.Background()

	staged, err := service.StageContent(ctx, work.StageContentRequest{
		ItemType: "image", FileName: "svc.png", MediaType: "image/png", Content: []byte("png"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef != "submit-work-stage:v1:svc" || staging.stageReq.FileName != "svc.png" {
		t.Fatalf("stage = (%#v, %#v)", staged, staging.stageReq)
	}

	parts, err := service.PrepareContent(ctx, []work.StagedSubmissionItem{{
		ItemType: "image", StagedFileRef: staged.StagedFileRef, FileName: staged.FileName, MediaType: staged.MediaType,
	}})
	if err != nil || len(parts) != 1 || staging.prepareIn[0].StagedFileRef != staged.StagedFileRef {
		t.Fatalf("PrepareContent = (%#v, %v, %#v)", parts, err, staging.prepareIn)
	}

	resolved, err := service.ResolveContent(ctx, staged.StagedFileRef)
	if err != nil || resolved.Path == "" || staging.resolveRef != staged.StagedFileRef {
		t.Fatalf("ResolveContent = (%#v, %v, %q)", resolved, err, staging.resolveRef)
	}

	if err := service.CleanupContent(ctx, staged.StagedFileRef); err != nil || staging.cleanupRef != staged.StagedFileRef {
		t.Fatalf("CleanupContent = (%v, %q)", err, staging.cleanupRef)
	}
}

func TestNewServiceDelegatesContentMaterializationSlice(t *testing.T) {
	materialized := ""
	materializer := work.ContentMaterializeFunc(func(_ context.Context, rawURL string) (string, work.ContentCleanup, error) {
		materialized = rawURL
		return "/tmp/materialized/svc.png", func() {}, nil
	})
	service := internalservice.NewService(
		workRuntimeResolver{runtime: &recordingFactory{}},
		os.ReadFile,
		nil,
		materializer,
	)
	ctx := context.Background()

	path, cleanup, err := service.MaterializeContentURL(ctx, "file:///fixtures/svc.png")
	if err != nil || path == "" || cleanup == nil || materialized != "file:///fixtures/svc.png" {
		t.Fatalf("MaterializeContentURL = (%q, %v, %v, %q)", path, cleanup, err, materialized)
	}
	cleanup()
}

func TestNewServiceContentSliceRequiresInjectedDependencies(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: &recordingFactory{}}, os.ReadFile, nil, nil)
	ctx := context.Background()

	if _, err := service.StageContent(ctx, work.StageContentRequest{}); err == nil || !strings.Contains(err.Error(), "content staging is required") {
		t.Fatalf("StageContent error = %v, want staging required", err)
	}
	if _, err := service.PrepareContent(ctx, nil); err == nil || !strings.Contains(err.Error(), "content staging is required") {
		t.Fatalf("PrepareContent error = %v, want staging required", err)
	}
	if _, err := service.ResolveContent(ctx, "ref"); err == nil || !strings.Contains(err.Error(), "content staging is required") {
		t.Fatalf("ResolveContent error = %v, want staging required", err)
	}
	if err := service.CleanupContent(ctx, "ref"); err == nil || !strings.Contains(err.Error(), "content staging is required") {
		t.Fatalf("CleanupContent error = %v, want staging required", err)
	}
	if _, _, err := service.MaterializeContentURL(ctx, "file:///x"); err == nil || !strings.Contains(err.Error(), "content materializer is required") {
		t.Fatalf("MaterializeContentURL error = %v, want materializer required", err)
	}
}

func TestNewServiceDelegatesPrepareWorkRequest(t *testing.T) {
	service := internalservice.NewService(workRuntimeResolver{runtime: &recordingFactory{}}, os.ReadFile, nil, nil)
	ctx := context.Background()

	prepared, err := service.PrepareWorkRequest(ctx, work.WorkRequestPreparation{
		Request: work.WorkRequest{
			RequestID: "request-service-1",
			Type:      work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.Work{{
				Name:       "draft",
				WorkTypeID: "task",
				Content:    []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "hello"}},
			}},
		},
	})
	if err != nil {
		t.Fatalf("PrepareWorkRequest: %v", err)
	}
	if prepared.RequestID != "request-service-1" || len(prepared.Works) != 1 || prepared.Works[0].Name != "draft" {
		t.Fatalf("prepared = %#v, want normalized draft work request", prepared)
	}
}
