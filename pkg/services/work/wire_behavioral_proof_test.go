package work_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

// TestWireBehavioralProof_PublishedRootPreservesObservables constructs Work
// exclusively through work/wire and proves admission, query, content, and
// state-access observables on the published work.Service peer surface.
func TestWireBehavioralProof_PublishedRootPreservesObservables(t *testing.T) {
	t.Run("admission query and state access success through runtime wire", func(t *testing.T) {
		runtime := &wireBehavioralRuntime{
			snapshot: work.ReadSnapshot{Items: []work.ReadModel{{
				CursorID:     "tok-wire-1",
				WorkID:       "work-wire-1",
				Name:         "wire-story",
				WorkTypeName: "story",
				State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
			}}},
		}
		root := wireBehavioralRuntimeService(t, runtime)
		ctx := context.Background()

		request := work.WorkRequest{
			RequestID: "wire-behavioral-admit",
			Type:      work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.Work{{
				Name:       "wire-story",
				WorkTypeID: "story",
				State:      "draft",
			}},
		}
		submit, err := root.SubmitWorkRequestForSession(ctx, "session-wire", request)
		if err != nil {
			t.Fatalf("SubmitWorkRequestForSession: %v", err)
		}
		if !submit.Accepted || submit.RequestID != request.RequestID || len(submit.Works) != 1 {
			t.Fatalf("submit result = %#v, want accepted wire-behavioral-admit", submit)
		}
		if runtime.submitted.RequestID != request.RequestID {
			t.Fatalf("submitted request = %q, want %q", runtime.submitted.RequestID, request.RequestID)
		}

		listed, err := root.ListWork(ctx, "session-wire", work.ListOptions{WorkTypeName: "story"})
		if err != nil {
			t.Fatalf("ListWork: %v", err)
		}
		if len(listed.Results) != 1 || listed.Results[0].WorkID != "work-wire-1" {
			t.Fatalf("list result = %#v, want one wire-story entry", listed)
		}

		got, err := root.GetWork(ctx, "session-wire", "work-wire-1")
		if err != nil {
			t.Fatalf("GetWork: %v", err)
		}
		if got.WorkID != "work-wire-1" || got.State == nil || got.State.Name != "review" {
			t.Fatalf("get result = %#v, want work-wire-1 in review", got)
		}

		moved, err := root.MoveWorkForSession(ctx, "session-wire", "work-wire-1", "done", "move-wire")
		if err != nil {
			t.Fatalf("MoveWorkForSession: %v", err)
		}
		if moved.WorkID != "work-wire-1" || moved.FromState != "review" || moved.ToState != "done" {
			t.Fatalf("move result = %#v, want work-wire-1 review->done", moved)
		}
	})

	t.Run("content staging materialization and cleanup success through runtime wire", func(t *testing.T) {
		root := wireBehavioralRuntimeService(t, nil)
		ctx := context.Background()

		staged, err := root.StageContent(ctx, work.StageContentRequest{
			ItemType:  "image",
			FileName:  "wire-behavioral.png",
			MediaType: "image/png",
			Content:   []byte("wire-behavioral-bytes"),
		})
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		if staged.StagedFileRef == "" || staged.URL == "" {
			t.Fatalf("stage result = %#v, want opaque staged reference and URL", staged)
		}

		resolved, err := root.ResolveContent(ctx, staged.StagedFileRef)
		if err != nil {
			t.Fatalf("ResolveContent: %v", err)
		}
		if resolved.Path == "" || resolved.URL != staged.URL {
			t.Fatalf("resolve result = %#v, want local path and staged URL", resolved)
		}

		if err := root.CleanupContent(ctx, staged.StagedFileRef); err != nil {
			t.Fatalf("CleanupContent: %v", err)
		}
		if _, err := root.ResolveContent(ctx, staged.StagedFileRef); !errors.Is(err, work.ErrStagedContentNotFound) {
			t.Fatalf("post-cleanup ResolveContent error = %v, want ErrStagedContentNotFound", err)
		}

		dir := t.TempDir()
		path := filepath.Join(dir, "wire-materialize.png")
		if err := os.WriteFile(path, []byte("materialize-bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
		rawURL, err := work.FilesystemPathToContentURL(path)
		if err != nil {
			t.Fatalf("FilesystemPathToContentURL: %v", err)
		}
		localPath, cleanup, err := root.MaterializeContentURL(ctx, rawURL)
		if err != nil {
			t.Fatalf("MaterializeContentURL: %v", err)
		}
		if localPath != path || cleanup == nil {
			t.Fatalf("materialize = (%q, %v), want local path and cleanup handle", localPath, cleanup)
		}
		cleanup()
	})

	t.Run("new service construction preserves observables", func(t *testing.T) {
		runtime := &wireBehavioralRuntime{
			snapshot: work.ReadSnapshot{Items: []work.ReadModel{{
				CursorID: "tok-new-service", WorkID: "work-new-service", Name: "new-service-story",
				WorkTypeName: "story", State: &work.State{Name: "review", Type: work.StateTypeProcessing},
			}}},
		}
		root, err := wireBehavioralNewService(t, runtime)
		if err != nil {
			t.Fatalf("workwire.NewService: %v", err)
		}
		ctx := context.Background()

		request := work.WorkRequest{
			RequestID: "wire-new-service-admit",
			Type:      work.WorkRequestTypeFactoryRequestBatch,
			Works:     []work.Work{{Name: "new-service-story", WorkTypeID: "story", State: "draft"}},
		}
		if _, err := root.SubmitWorkRequestForSession(ctx, "session-new-service", request); err != nil {
			t.Fatalf("SubmitWorkRequestForSession: %v", err)
		}
		if runtime.submitted.RequestID != request.RequestID {
			t.Fatalf("submitted request = %q, want %q", runtime.submitted.RequestID, request.RequestID)
		}

		staged, err := root.StageContent(ctx, work.StageContentRequest{
			ItemType: "image", FileName: "new-service.png", MediaType: "image/png", Content: []byte("png"),
		})
		if err != nil || staged.StagedFileRef == "" {
			t.Fatalf("StageContent = %#v, %v", staged, err)
		}
	})

	t.Run("typed failures", func(t *testing.T) {
		ctx := context.Background()
		root := wireBehavioralRuntimeService(t, &wireBehavioralRuntime{snapshot: work.ReadSnapshot{}})

		if _, err := root.GetWork(ctx, "session-wire", "missing-work"); !errors.Is(err, work.ErrWorkNotFound) {
			t.Fatalf("GetWork missing = %v, want ErrWorkNotFound", err)
		}

		if _, _, err := root.MaterializeContentURL(ctx, "http://127.0.0.1/secret"); !errors.Is(err, work.ErrUnsafeContentURL) {
			t.Fatalf("MaterializeContentURL unsafe = %v, want ErrUnsafeContentURL", err)
		}

		staged, err := root.StageContent(ctx, work.StageContentRequest{
			ItemType: "image", FileName: "typed.png", MediaType: "image/png", Content: []byte("png"),
		})
		if err != nil {
			t.Fatalf("StageContent: %v", err)
		}
		if _, err := root.ResolveContent(ctx, staged.StagedFileRef+"tampered"); !errors.Is(err, work.ErrInvalidStagedContentRef) {
			t.Fatalf("ResolveContent tampered = %v, want ErrInvalidStagedContentRef", err)
		}

		moveRoot := wireBehavioralRuntimeService(t, &wireBehavioralRuntime{
			snapshot: work.ReadSnapshot{},
			moveErr:  work.ErrMoveWorkRequestAlreadyApplied,
		})
		if _, err := moveRoot.MoveWorkForSession(ctx, "session-wire", "work-1", "done", "dup-move"); !errors.Is(err, work.ErrMoveWorkRequestAlreadyApplied) {
			t.Fatalf("MoveWorkForSession duplicate = %v, want ErrMoveWorkRequestAlreadyApplied", err)
		}
	})
}

type wireBehavioralRuntime struct {
	submitted work.WorkRequest
	snapshot  work.ReadSnapshot
	moveErr   error
}

func (r *wireBehavioralRuntime) SubmitWorkRequest(_ context.Context, request work.WorkRequest) (work.WorkRequestSubmitResult, error) {
	r.submitted = request
	works := make([]work.WorkRequestSubmittedWork, 0, len(request.Works))
	for _, item := range request.Works {
		works = append(works, work.WorkRequestSubmittedWork{
			Name:         item.Name,
			WorkTypeName: item.WorkTypeID,
			WorkID:       "work-wire-admitted",
		})
	}
	return work.WorkRequestSubmitResult{
		RequestID: request.RequestID,
		Accepted:  true,
		Works:     works,
	}, nil
}

func (r *wireBehavioralRuntime) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return r.snapshot, nil
}

func (r *wireBehavioralRuntime) MoveWork(
	_ context.Context,
	workID string,
	stateName string,
	_ work.WorkStateChangeSource,
	_ string,
) (work.OperatorMoveResult, error) {
	if r.moveErr != nil {
		return work.OperatorMoveResult{}, r.moveErr
	}
	return work.OperatorMoveResult{
		WorkID:     workID,
		WorkTypeID: "story",
		FromState:  "review",
		ToState:    stateName,
	}, nil
}

type wireBehavioralResolver struct {
	runtime work.Runtime
}

func (r wireBehavioralResolver) ResolveWorkRuntime(string) (work.Runtime, error) {
	return r.runtime, nil
}

func wireBehavioralRuntimeService(t *testing.T, runtime work.Runtime) work.Service {
	t.Helper()
	service := workwire.NewRuntimeService(
		wireBehavioralResolver{runtime: runtime},
		nil,
		newPublicWorkRootStaging(t, nil),
		newPublicWorkRootMaterializer(t),
	)
	var root work.Service = service
	return root
}

func wireBehavioralNewService(t *testing.T, workRuntime work.Runtime) (work.Service, error) {
	t.Helper()
	return workwire.NewService(
		wireBehavioralResolver{runtime: workRuntime},
		&publicSeamFileSystem{root: t.TempDir()},
		publicSeamRandom{value: 0x11},
		&publicSeamClock{now: time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)},
		time.Minute,
		work.ContentHostPlatform(runtime.GOOS),
		&http.Client{
			Timeout:       workwire.DefaultContentMaterializationHTTPTimeout,
			CheckRedirect: workwire.ContentMaterializationRedirectPolicy(0, false),
		},
		os.Stat,
		func(dir, pattern string) (work.ContentTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.WriteFile,
		func(path string) (io.WriteCloser, error) {
			return os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0o600)
		},
	)
}
