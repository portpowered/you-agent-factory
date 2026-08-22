package wire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

func TestSnapshotStateAccessServiceNilReaderReturnsNil(t *testing.T) {
	t.Parallel()

	if got := workwire.SnapshotStateAccessService(nil); got != nil {
		t.Fatalf("SnapshotStateAccessService(nil) = %#v, want nil", got)
	}
}

func TestSnapshotStateAccessServiceProjectsListAndGetOntoWorkRoot(t *testing.T) {
	t.Parallel()

	reader := &workSnapshotReaderStub{snapshot: reviewWorkSnapshot("work-story")}
	ctx := context.Background()
	svc := workwire.SnapshotStateAccessService(reader)
	if svc == nil {
		t.Fatal("SnapshotStateAccessService(reader) = nil, want work.Service")
	}

	list, err := svc.ListWork(ctx, "session-snapshot-wire", work.ListOptions{})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWork = %#v, want one story work item", list)
	}

	got, err := svc.GetWork(ctx, "session-snapshot-wire", "work-story")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.WorkID != "work-story" {
		t.Fatalf("GetWork = %#v, want work-story", got)
	}
	if reader.sessions != 2 {
		t.Fatalf("snapshot reader reads = %d, want one per read (ListWork and GetWork)", reader.sessions)
	}
}

func TestSnapshotStateAccessServiceSurfacesReaderFailures(t *testing.T) {
	t.Parallel()

	failure := errors.New("snapshot projection unavailable")
	svc := workwire.SnapshotStateAccessService(&workSnapshotReaderStub{err: failure})

	if _, err := svc.ListWork(context.Background(), "session-snapshot-wire", work.ListOptions{}); !errors.Is(err, failure) {
		t.Fatalf("ListWork error = %v, want the snapshot reader failure", err)
	}
}

func TestSnapshotStateAccessServiceRejectsUnsupportedWorkRootRoles(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := workwire.SnapshotStateAccessService(&workSnapshotReaderStub{})

	_, err := svc.PrepareWorkRequest(ctx, work.WorkRequestPreparation{})
	if err == nil {
		t.Fatal("PrepareWorkRequest() error = nil, want unsupported role")
	}

	_, err = svc.StageContent(ctx, work.StageContentRequest{})
	if err == nil {
		t.Fatal("StageContent() error = nil, want unsupported role")
	}

	_, err = svc.PrepareContent(ctx, nil)
	if err == nil {
		t.Fatal("PrepareContent() error = nil, want unsupported role")
	}

	_, err = svc.ResolveContent(ctx, "staged-id")
	if err == nil {
		t.Fatal("ResolveContent() error = nil, want unsupported role")
	}

	if err := svc.CleanupContent(ctx, "staged-id"); err == nil {
		t.Fatal("CleanupContent() error = nil, want unsupported role")
	}

	_, _, err = svc.MaterializeContentURL(ctx, "https://example.com/content")
	if err == nil {
		t.Fatal("MaterializeContentURL() error = nil, want unsupported role")
	}

	_, err = svc.MaterializeWorkerOutput(ctx, work.MaterializeWorkerOutputRequest{})
	if err == nil {
		t.Fatal("MaterializeWorkerOutput() error = nil, want unsupported role")
	}

	_, err = svc.PrepareInvocationInput(ctx, work.InvocationInputPreparationRequest{})
	if err == nil {
		t.Fatal("PrepareInvocationInput() error = nil, want unsupported role")
	}

	_, err = svc.ResolvePrimaryResult(ctx, work.PrimaryResultSelectionInput{})
	if err == nil {
		t.Fatal("ResolvePrimaryResult() error = nil, want unsupported role")
	}
}

func TestSnapshotStateAccessServiceDelegatesSessionMutations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := workwire.SnapshotStateAccessService(&workSnapshotReaderStub{})

	_, err := svc.SubmitWorkRequestForSession(ctx, "session-1", work.WorkRequest{})
	if err == nil {
		t.Fatal("SubmitWorkRequestForSession() error = nil, want unavailable session adapter")
	}

	_, err = svc.MoveWorkForSession(ctx, "session-1", "work-1", "done", "req-1")
	if err == nil {
		t.Fatal("MoveWorkForSession() error = nil, want unavailable session adapter")
	}

	_, err = svc.MoveWorkAndRead(ctx, "session-1", "work-1", "done", "req-1")
	if err == nil {
		t.Fatal("MoveWorkAndRead() error = nil, want unavailable session adapter")
	}
}

type workSnapshotReaderStub struct {
	snapshot work.ReadSnapshot
	err      error
	sessions int
}

func (stub *workSnapshotReaderStub) ReadWorkSnapshot(
	context.Context,
	string,
) (work.ReadSnapshot, error) {
	stub.sessions++
	if stub.err != nil {
		return work.ReadSnapshot{}, stub.err
	}
	return stub.snapshot, nil
}

func reviewWorkSnapshot(workID string) work.ReadSnapshot {
	return work.ReadSnapshot{Items: []work.ReadModel{{
		CursorID:     workID,
		Name:         "Review PRD",
		WorkID:       workID,
		WorkTypeName: "story",
		State:        &work.State{Name: "review", Type: work.StateTypeProcessing},
	}}}
}
