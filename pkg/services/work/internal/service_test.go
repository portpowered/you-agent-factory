package internal_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal"
)

type recordingFactory struct {
	submitted work.WorkRequest
	ctx       context.Context
	result    work.WorkRequestSubmitResult
	err       error
	calls     int
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

func (f *recordingFactory) SubmitWorkRequest(ctx context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	f.submitted = request
	f.ctx = ctx
	f.calls++
	return f.result, f.err
}

type selectedWorkResolver func(string) (work.Runtime, error)

func (r selectedWorkResolver) ResolveWorkRuntime(sessionID string) (work.Runtime, error) {
	return r(sessionID)
}

func TestSubmitFileForSessionPreservesSelectionAndFailures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"success", "resolver", "unavailable", "read", "parse", "admission", "canceled", "deadline"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runSelectedFileSubmission(t, name)
		})
	}
}

func runSelectedFileSubmission(t *testing.T, name string) {
	t.Helper()
	failure := errors.New("controlled failure")
	canonical := `{"requestId":"request-edge","type":"FACTORY_REQUEST_BATCH","works":[{"name":"item","workId":"work-edge","workTypeName":"task","state":"init","payload":{"value":"hello"}}]}`
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	want := work.WorkRequestSubmitResult{RequestID: "request-edge", WorkID: "work-edge", Accepted: true}
	peer := &recordingFactory{result: want}
	wantErr := error(nil)
	if name == "admission" {
		peer.err, wantErr = failure, failure
	}
	if name == "canceled" {
		cancel()
		peer.err, wantErr = ctx.Err(), context.Canceled
	}
	if name == "deadline" {
		deadlineCtx, deadlineCancel := context.WithDeadline(ctx, time.Time{})
		defer deadlineCancel()
		ctx = deadlineCtx
		peer.err, wantErr = ctx.Err(), context.DeadlineExceeded
	}
	readCalls := 0
	resolverCalls := 0
	service := newTestWorkService(selectedWorkResolver(func(id string) (work.Runtime, error) {
		resolverCalls++
		if id != "selected-session" {
			t.Fatalf("session = %q", id)
		}
		if name == "resolver" {
			return nil, failure
		}
		if name == "unavailable" {
			return nil, nil
		}
		return peer, nil
	}), func(path string) ([]byte, error) {
		readCalls++
		if path != "owned.json" {
			t.Fatalf("path = %q", path)
		}
		if name == "read" {
			return nil, failure
		}
		if name == "parse" {
			return []byte(`{`), nil
		}
		return []byte(canonical), nil
	}, nil, nil, nil)
	got, err := service.SubmitFileForSession(ctx, "selected-session", "owned.json")
	if resolverCalls != 1 {
		t.Fatalf("resolver calls = %d", resolverCalls)
	}
	switch name {
	case "resolver", "unavailable":
		assertSelectedFileResolutionFailure(t, name, err, failure, readCalls, peer.calls)
	case "read", "parse":
		assertSelectedFileReadFailure(t, name, err, failure, readCalls, peer.calls)
	default:
		assertSelectedFileAdmission(t, ctx, peer, readCalls, got, want, err, wantErr)
	}
	if err != nil && !reflect.DeepEqual(got, work.WorkRequestSubmitResult{}) {
		t.Fatalf("failure returned success: %#v", got)
	}
}

func assertSelectedFileResolutionFailure(t *testing.T, name string, err, failure error, readCalls, admissionCalls int) {
	t.Helper()
	if readCalls != 0 || admissionCalls != 0 {
		t.Fatal("failed resolution reached reader/admission")
	}
	// Is proves sentinel identity; DeepEqual also rejects an added wrapper.
	if name == "resolver" && (!errors.Is(err, failure) || !reflect.DeepEqual(err, failure)) {
		t.Fatalf("resolver error = %v", err)
	}
	if name == "unavailable" && (err == nil || err.Error() != "Factory Session runtime is unavailable: selected-session") {
		t.Fatalf("absence error = %v", err)
	}
}

func assertSelectedFileReadFailure(t *testing.T, name string, err, failure error, readCalls, admissionCalls int) {
	t.Helper()
	if readCalls != 1 || admissionCalls != 0 {
		t.Fatal("failed file reached admission")
	}
	prefix := name + " work file owned.json: "
	if err == nil || !strings.HasPrefix(err.Error(), prefix) {
		t.Fatalf("file error = %v", err)
	}
	if name == "read" && !errors.Is(err, failure) {
		t.Fatalf("read error identity = %v", err)
	}
	if name == "read" && err.Error() != "read work file owned.json: controlled failure" {
		t.Fatalf("read diagnostic = %v", err)
	}
	if name == "parse" {
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Fatalf("parse error lost syntax cause: %v", err)
		}
	}
}

func assertSelectedFileAdmission(t *testing.T, ctx context.Context, peer *recordingFactory, readCalls int, got, want work.WorkRequestSubmitResult, err, wantErr error) {
	t.Helper()
	request := work.WorkRequest{RequestID: "request-edge", Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{Name: "item", WorkID: "work-edge", WorkTypeID: "task", State: "init", Payload: map[string]any{"value": "hello"}}}}
	if readCalls != 1 || peer.calls != 1 || peer.ctx != ctx || !reflect.DeepEqual(peer.submitted, request) {
		t.Fatalf("admission changed context/request: %#v", peer)
	}
	if wantErr != nil && (!errors.Is(err, wantErr) || !strings.HasPrefix(err.Error(), "submit initial work: ")) {
		t.Fatalf("admission error = %v", err)
	}
	if wantErr != nil && err.Error() != "submit initial work: "+wantErr.Error() {
		t.Fatalf("admission diagnostic = %v", err)
	}
	if wantErr == nil && (err != nil || !reflect.DeepEqual(got, want)) {
		t.Fatalf("result = %#v, error = %v", got, err)
	}
}

func (f *recordingFactory) MoveWork(_ context.Context, workID, _ string, source work.WorkStateChangeSource, _ string) (work.OperatorMoveResult, error) {
	f.movedID, f.source = workID, source
	return work.OperatorMoveResult{}, nil
}

func (f *recordingFactory) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}

func TestSubmitFileParsesAndSubmitsCanonicalWorkRequest(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	target := &recordingFactory{}
	reads := 0
	reader := func(path string) ([]byte, error) {
		reads++
		if path != "work.json" || target.calls != 0 {
			t.Fatal("reader path or admission order changed")
		}
		return []byte(`{"requestId":"request-from-file","type":"FACTORY_REQUEST_BATCH","works":[{"name":"work-1","workTypeName":"test","state":"init","payload":{"value":"hello"}}]}`), nil
	}
	if err := internalservice.SubmitFile(ctx, "work.json", target, reader); err != nil {
		t.Fatalf("SubmitFile: %v", err)
	}
	want := work.WorkRequest{RequestID: "request-from-file", Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{Name: "work-1", WorkTypeID: "test", State: "init", Payload: map[string]any{"value": "hello"}}}}
	if reads != 1 || target.calls != 1 || target.ctx != ctx || !reflect.DeepEqual(target.submitted, want) {
		t.Fatalf("admission changed context/request: %#v, reads = %d", target, reads)
	}
}

func TestSubmitFileForSessionUsesInjectedReaderAndRuntime(t *testing.T) {
	runtime := &recordingFactory{}
	readPath := ""
	service := newTestWorkService(workRuntimeResolver{runtime: runtime}, func(path string) ([]byte, error) {
		readPath = path
		return []byte(`{"requestId":"request-edge","type":"FACTORY_REQUEST_BATCH","works":[]}`), nil
	}, nil, nil, nil)

	result, err := service.SubmitFileForSession(context.Background(), "session-1", "edge.json")
	if err != nil {
		t.Fatalf("SubmitFileForSession: %v", err)
	}
	if readPath != "edge.json" || runtime.submitted.RequestID != "request-edge" || result.RequestID != "" {
		t.Fatalf("submitted file route = (%q, %q, %#v)", readPath, runtime.submitted.RequestID, result)
	}
}

func TestSubmitFileFailsClosedWithoutReader(t *testing.T) {
	t.Parallel()
	target := &recordingFactory{}
	err := internalservice.SubmitFile(t.Context(), "work.json", target, nil)
	if err == nil || err.Error() != "submitted Work Request file reader is required" || target.calls != 0 {
		t.Fatalf("error = %v, admission calls = %d", err, target.calls)
	}
	// Reader validation precedes even a missing standalone target.
	err = internalservice.SubmitFile(t.Context(), "work.json", nil, nil)
	if err == nil || err.Error() != "submitted Work Request file reader is required" {
		t.Fatalf("missing reader/target error = %v", err)
	}
}

func TestSubmitFileReportsReadParseAndRuntimeFailures(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"read", "parse", "runtime", "admission", "canceled", "deadline"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			failure := errors.New("controlled failure")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			target := &recordingFactory{}
			var submitTarget internalservice.SubmitTarget = target
			wantErr := failure
			switch name {
			case "runtime":
				submitTarget = nil
			case "admission":
				target.err = failure
			case "canceled":
				cancel()
				target.err, wantErr = ctx.Err(), context.Canceled
			case "deadline":
				deadlineCtx, deadlineCancel := context.WithDeadline(ctx, time.Time{})
				defer deadlineCancel()
				ctx = deadlineCtx
				target.err, wantErr = ctx.Err(), context.DeadlineExceeded
			}
			reads := 0
			err := internalservice.SubmitFile(ctx, "owned.json", submitTarget, func(path string) ([]byte, error) {
				reads++
				if path != "owned.json" || target.calls != 0 {
					t.Fatal("reader path or admission order changed")
				}
				if name == "read" {
					return nil, failure
				}
				if name == "parse" {
					return []byte(`{`), nil
				}
				return []byte(`{"requestId":"request-edge","type":"FACTORY_REQUEST_BATCH","works":[{"name":"item","workId":"work-edge","workTypeName":"task","state":"init","payload":{"value":"hello"}}]}`), nil
			})
			switch name {
			case "read", "parse":
				assertSelectedFileReadFailure(t, name, err, failure, reads, target.calls)
			case "runtime":
				if err == nil || err.Error() != "factory runtime is not available" || reads != 1 || target.calls != 0 {
					t.Fatalf("absence error = %v, reads = %d, admission calls = %d", err, reads, target.calls)
				}
			default:
				assertSelectedFileAdmission(t, ctx, target, reads, work.WorkRequestSubmitResult{}, work.WorkRequestSubmitResult{}, err, wantErr)
			}
		})
	}
}

func TestNewServiceExposesInvocationAndReturnPolicySlice(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	stdin := "from service root"
	want := work.PreparedInvocationInput{Source: work.InputSourceStdinText, ResolvedInput: &work.ResolvedInput{Text: stdin}}
	request := work.InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &stdin}
	service := internalservice.NewService(nil, nil, nil, nil, nil, nil, nil,
		fakeInvocationPreparation(func(gotCtx context.Context, got work.InvocationInputPreparationRequest) (work.PreparedInvocationInput, error) {
			if gotCtx != ctx || got.StdinText != request.StdinText || got.Arguments[0] != "-" {
				t.Fatal("invocation request changed during delegation")
			}
			return want, nil
		}))
	prepared, err := service.PrepareInvocationInput(ctx, request)
	if err != nil || prepared.ResolvedInput != want.ResolvedInput || prepared.Source != want.Source {
		t.Fatalf("prepared = %#v, error = %v, want completed output", prepared, err)
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
	service := newTestWorkService(
		workRuntimeResolver{runtime: &recordingFactory{}},
		os.ReadFile,
		nil,
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
	service := newTestWorkService(
		workRuntimeResolver{runtime: &recordingFactory{}},
		os.ReadFile,
		nil,
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
	service := newTestWorkService(workRuntimeResolver{runtime: &recordingFactory{}}, os.ReadFile, nil, nil, nil)
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

type fakeRequestPreparation func(context.Context, work.WorkRequestPreparation) (work.WorkRequest, error)

func (f fakeRequestPreparation) PrepareWorkRequest(ctx context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
	return f(ctx, input)
}
func TestNewServiceDelegatesPrepareWorkRequest(t *testing.T) {
	t.Parallel()
	expected := work.WorkRequest{RequestID: "injected-request", Works: []work.Work{{Name: "prepared"}}}
	failure := errors.New("preparation failed")
	for _, wantErr := range []error{nil, failure, context.Canceled, context.DeadlineExceeded} {
		service := internalservice.NewService(nil, nil, nil, nil, nil, nil, fakeRequestPreparation(func(ctx context.Context, input work.WorkRequestPreparation) (work.WorkRequest, error) {
			if ctx != t.Context() || input.Request.RequestID != "caller" {
				t.Fatal("request changed during delegation")
			}
			return expected, wantErr
		}), nil)
		got, err := service.PrepareWorkRequest(t.Context(), work.WorkRequestPreparation{Request: work.WorkRequest{RequestID: "caller"}})
		if err != wantErr || got.RequestID != expected.RequestID {
			t.Fatalf("preparation = %#v, error = %v", got, err)
		}
	}
}

func (*recordingFactory) ReadWorkerSessionWork(context.Context, string) (work.WorkerSessionWork, error) {
	panic("unexpected selected Work read in legacy fixture")
}
