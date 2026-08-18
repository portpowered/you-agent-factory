package wire_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

func TestGetWorkFromSnapshotsReadsThroughTheInjectedSnapshotReader(t *testing.T) {
	t.Parallel()

	reader := &snapshotReaderStub{snapshot: reviewWorkSnapshot("work-story")}

	got, err := stateaccesswire.GetWorkFromSnapshots(
		context.Background(),
		"session-state-access-unit",
		"work-story",
		reader,
	)
	if err != nil {
		t.Fatalf("GetWorkFromSnapshots: %v", err)
	}
	if got.WorkID != "work-story" {
		t.Fatalf("GetWorkFromSnapshots = %#v, want work-story", got)
	}
	if reader.session != "session-state-access-unit" {
		t.Fatalf("snapshot reader session = %q, want the requested session", reader.session)
	}
}

func TestListWorkFromSnapshotsReadsThroughTheInjectedSnapshotReader(t *testing.T) {
	t.Parallel()

	reader := &snapshotReaderStub{snapshot: reviewWorkSnapshot("work-story")}

	list, err := stateaccesswire.ListWorkFromSnapshots(
		context.Background(),
		"session-state-access-unit",
		reader,
		work.ListOptions{},
	)
	if err != nil {
		t.Fatalf("ListWorkFromSnapshots: %v", err)
	}
	if len(list.Results) != 1 || list.Results[0].WorkID != "work-story" {
		t.Fatalf("ListWorkFromSnapshots = %#v, want one story work item", list)
	}
	if list.Results[0].State == nil || list.Results[0].State.Name != "review" {
		t.Fatalf("ListWorkFromSnapshots state = %#v, want review", list.Results[0].State)
	}
}

func TestSnapshotRootServiceSurfacesSnapshotReaderFailures(t *testing.T) {
	t.Parallel()

	failure := errors.New("snapshot unavailable")
	_, err := stateaccesswire.ListWorkFromSnapshots(
		context.Background(),
		"session-state-access-unit",
		&snapshotReaderStub{err: failure},
		work.ListOptions{},
	)
	if !errors.Is(err, failure) {
		t.Fatalf("ListWorkFromSnapshots error = %v, want the snapshot reader failure", err)
	}
}

type snapshotReaderStub struct {
	snapshot work.ReadSnapshot
	err      error
	session  string
}

func (stub *snapshotReaderStub) ReadWorkSnapshot(
	_ context.Context,
	sessionID string,
) (work.ReadSnapshot, error) {
	stub.session = sessionID
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
