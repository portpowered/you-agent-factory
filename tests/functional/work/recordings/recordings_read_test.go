package workrecordings_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

// These scenarios cover CUT-WORK-REC now that the Work -> Recordings back-edge
// is gone. Work serves session-scoped list/get reads from a snapshot reader
// that composition selects, and never names the owner that projected the
// snapshot. The Recordings-owned projection that satisfies this port is proven
// against the published Recordings root by its own owner, in
// pkg/services/recordings/wire.

// TestSnapshotBackedWorkReadsServeListAndGetThroughTheWorkRoot proves a leased
// Work read edge answers both published read operations from the selected
// snapshot reader, reading the session snapshot once per operation.
func TestSnapshotBackedWorkReadsServeListAndGetThroughTheWorkRoot(t *testing.T) {
	t.Parallel()

	reader := &sessionSnapshotReader{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		storyWorkItem("work-review", "Review PRD", "review", work.StateTypeProcessing),
		storyWorkItem("work-done", "Ship release", "done", work.StateTypeTerminal),
	}}}
	svc := workwire.SnapshotStateAccessService(reader)
	ctx := context.Background()

	list, err := svc.ListWork(ctx, "session-snapshot-functional", work.ListOptions{})
	if err != nil {
		t.Fatalf("ListWork: %v", err)
	}
	if len(list.Results) != 2 {
		t.Fatalf("ListWork = %#v, want both work items", list)
	}

	got, err := svc.GetWork(ctx, "session-snapshot-functional", "work-review")
	if err != nil {
		t.Fatalf("GetWork: %v", err)
	}
	if got.WorkID != "work-review" || got.State == nil || got.State.Name != "review" {
		t.Fatalf("GetWork = %#v, want the review story", got)
	}

	if reader.reads != 2 {
		t.Fatalf("snapshot reads = %d, want one per read operation", reader.reads)
	}
	if reader.sessions[0] != "session-snapshot-functional" {
		t.Fatalf("snapshot reader sessions = %#v, want the requested session", reader.sessions)
	}
}

// TestSnapshotBackedWorkReadsFilterAndPaginateTheProjectedSnapshot proves the
// Work read edge still owns state filtering, result limits, and cursor
// pagination over whatever snapshot the selected reader returns.
func TestSnapshotBackedWorkReadsFilterAndPaginateTheProjectedSnapshot(t *testing.T) {
	t.Parallel()

	svc := workwire.SnapshotStateAccessService(&sessionSnapshotReader{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		storyWorkItem("work-a", "Review PRD", "review", work.StateTypeProcessing),
		storyWorkItem("work-b", "Review plan", "review", work.StateTypeProcessing),
		storyWorkItem("work-c", "Ship release", "done", work.StateTypeTerminal),
	}}})
	ctx := context.Background()

	filtered, err := svc.ListWork(ctx, "session-snapshot-functional", work.ListOptions{
		StateName: "review",
		Counts:    true,
	})
	if err != nil {
		t.Fatalf("filtered ListWork: %v", err)
	}
	if len(filtered.Results) != 2 {
		t.Fatalf("filtered ListWork = %#v, want the two review items", filtered.Results)
	}
	if filtered.Counts == nil || filtered.Counts.Total != 2 {
		t.Fatalf("filtered counts = %#v, want a total of 2", filtered.Counts)
	}

	first, err := svc.ListWork(ctx, "session-snapshot-functional", work.ListOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("first page ListWork: %v", err)
	}
	if len(first.Results) != 2 || first.NextToken == "" {
		t.Fatalf("first page = %#v, want two results and a continuation token", first)
	}

	second, err := svc.ListWork(ctx, "session-snapshot-functional", work.ListOptions{
		MaxResults: 2,
		NextToken:  first.NextToken,
	})
	if err != nil {
		t.Fatalf("second page ListWork: %v", err)
	}
	if len(second.Results) != 1 || second.NextToken != "" {
		t.Fatalf("second page = %#v, want the remaining item and no continuation token", second)
	}
	if second.Results[0].WorkID == first.Results[0].WorkID {
		t.Fatalf("second page repeated %q from the first page", second.Results[0].WorkID)
	}
}

// TestSnapshotBackedWorkReadsSurfaceTypedReaderFailures proves a typed failure
// raised by the selected snapshot reader stays classifiable after it crosses
// the Work read edge, so callers keep their existing error handling.
func TestSnapshotBackedWorkReadsSurfaceTypedReaderFailures(t *testing.T) {
	t.Parallel()

	failure := errors.New("projected snapshot is unavailable")
	svc := workwire.SnapshotStateAccessService(&sessionSnapshotReader{err: failure})

	if _, err := svc.ListWork(context.Background(), "session-snapshot-functional", work.ListOptions{}); !errors.Is(err, failure) {
		t.Fatalf("ListWork error = %v, want the reader failure", err)
	}
	if _, err := svc.GetWork(context.Background(), "session-snapshot-functional", "work-review"); !errors.Is(err, failure) {
		t.Fatalf("GetWork error = %v, want the reader failure", err)
	}
}

// TestSnapshotBackedWorkReadsRejectUnknownWork proves the Work read edge still
// answers the published not-found contract instead of an empty read model when
// the projected snapshot has no matching item.
func TestSnapshotBackedWorkReadsRejectUnknownWork(t *testing.T) {
	t.Parallel()

	svc := workwire.SnapshotStateAccessService(&sessionSnapshotReader{snapshot: work.ReadSnapshot{Items: []work.ReadModel{
		storyWorkItem("work-review", "Review PRD", "review", work.StateTypeProcessing),
	}}})

	_, err := svc.GetWork(context.Background(), "session-snapshot-functional", "work-missing")
	if !errors.Is(err, work.ErrWorkNotFound) {
		t.Fatalf("GetWork error = %v, want ErrWorkNotFound", err)
	}
}

// TestSnapshotBackedWorkReadsRefuseSessionMutations proves a snapshot-backed
// composition stays read-only: submit and move still require a live Factory
// Session adapter and fail closed without one.
func TestSnapshotBackedWorkReadsRefuseSessionMutations(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	svc := workwire.SnapshotStateAccessService(&sessionSnapshotReader{})

	if _, err := svc.SubmitWorkRequestForSession(ctx, "session-snapshot-functional", work.WorkRequest{}); err == nil {
		t.Fatal("SubmitWorkRequestForSession() error = nil, want an unavailable session adapter")
	}
	if _, err := svc.MoveWorkForSession(ctx, "session-snapshot-functional", "work-review", "done", "request-1"); err == nil {
		t.Fatal("MoveWorkForSession() error = nil, want an unavailable session adapter")
	}
	if _, err := svc.MoveWorkAndRead(ctx, "session-snapshot-functional", "work-review", "done", "request-1"); err == nil {
		t.Fatal("MoveWorkAndRead() error = nil, want an unavailable session adapter")
	}
}

type sessionSnapshotReader struct {
	snapshot work.ReadSnapshot
	err      error
	reads    int
	sessions []string
}

func (reader *sessionSnapshotReader) ReadWorkSnapshot(
	_ context.Context,
	sessionID string,
) (work.ReadSnapshot, error) {
	reader.reads++
	reader.sessions = append(reader.sessions, sessionID)
	if reader.err != nil {
		return work.ReadSnapshot{}, reader.err
	}
	return reader.snapshot, nil
}

func storyWorkItem(workID, name, stateName, stateType string) work.ReadModel {
	return work.ReadModel{
		CursorID:     workID,
		Name:         name,
		WorkID:       workID,
		WorkTypeName: "story",
		State:        &work.State{Name: stateName, Type: stateType},
	}
}
