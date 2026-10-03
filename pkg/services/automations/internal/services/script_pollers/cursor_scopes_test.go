package script_pollers_test

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
)

func TestCursorScopes_BlankBaseUsesIsolatedMemoryWithoutIO(t *testing.T) {
	t.Parallel()
	owner := scriptpollerswire.NewCursorScopes(unexpectedCursorIO{})
	ctx := context.Background()
	for _, runtimeID := range []string{"", "runtime-a", "runtime-b"} {
		scope := scriptpollers.CursorScope{RuntimeID: runtimeID, BaseDir: " "}
		commitScopedCursor(t, owner, scope, "", automations.Cursor("cursor-"+runtimeID))
	}
	for _, runtimeID := range []string{"", "runtime-a", "runtime-b"} {
		scope := scriptpollers.CursorScope{RuntimeID: runtimeID}
		got, err := owner.GetCursor(ctx, scope, automations.GetCursorRequest{InstanceID: "shared-instance"})
		if err != nil || got.Cursor != automations.Cursor("cursor-"+runtimeID) || got.Checkpoint != "checkpoint-cursor-"+runtimeID {
			t.Fatalf("memory scope %q: cursor=%+v error=%v", runtimeID, got, err)
		}
	}
}

func TestCursorScopes_ReleasedMemoryRestartsEmptyAndRetainsPeers(t *testing.T) {
	t.Parallel()
	owner := scriptpollerswire.NewCursorScopes(unexpectedCursorIO{})
	scope := scriptpollers.CursorScope{RuntimeID: "runtime-a", BaseDir: " "}
	peer := scriptpollers.CursorScope{RuntimeID: "runtime-b"}
	detached := scriptpollers.CursorScope{}
	commitScopedCursor(t, owner, scope, "", "old-a")
	commitScopedCursor(t, owner, peer, "", "peer")
	commitScopedCursor(t, owner, detached, "", "detached")

	scope.BaseDir = ""
	owner.ReleaseScope(scope)
	owner.ReleaseScope(scope)
	_, err := owner.GetCursor(context.Background(), scope, automations.GetCursorRequest{InstanceID: "shared-instance"})
	if !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("released memory recovery error=%v, want not found", err)
	}
	commitScopedCursor(t, owner, scope, "", "new-a")
	assertScopedCursor(t, owner, scope, "new-a")
	assertScopedCursor(t, owner, peer, "peer")
	assertScopedCursor(t, owner, detached, "detached")
}

func TestCursorScopes_ReleasePreservesDurableRecoveryAndPeer(t *testing.T) {
	t.Parallel()
	owner := scriptpollerswire.NewCursorScopes(osFileSystem{})
	scope := scriptpollers.CursorScope{RuntimeID: "runtime-a", BaseDir: t.TempDir()}
	peer := scriptpollers.CursorScope{RuntimeID: "runtime-b", BaseDir: t.TempDir()}
	commitScopedCursor(t, owner, scope, "", "committed-a")
	commitScopedCursor(t, owner, peer, "", "committed-b")

	owner.ReleaseScope(scope)
	owner.ReleaseScope(scope)
	assertScopedCursor(t, owner, scope, "committed-a")
	commitScopedCursor(t, owner, scope, "committed-a", "resumed-a")
	assertScopedCursor(t, owner, scope, "resumed-a")
	assertScopedCursor(t, owner, peer, "committed-b")
	fresh := scriptpollerswire.NewCursorScopes(osFileSystem{})
	assertScopedCursor(t, fresh, scope, "resumed-a")
}

func TestCursorScopes_DurableRecoveryRetainsAuthoredDestination(t *testing.T) {
	t.Parallel()
	owner := scriptpollerswire.NewCursorScopes(osFileSystem{})
	scope := scriptpollers.CursorScope{RuntimeID: "runtime-a", BaseDir: t.TempDir()}
	commitScopedCursor(t, owner, scope, "", "opaque / page=7")
	// Runtime identity must not change existing customer-selected durable paths.
	restarted := scriptpollerswire.NewCursorScopes(osFileSystem{})
	scope.RuntimeID = "runtime-replacement"
	assertScopedCursor(t, restarted, scope, "opaque / page=7")
	legacy, err := scriptpollerswire.NewDurableCursorRecorder(scope.BaseDir, osFileSystem{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := legacy.GetCursor(context.Background(), automations.GetCursorRequest{InstanceID: "shared-instance"})
	if err != nil || got.Cursor != "opaque / page=7" {
		t.Fatalf("legacy recovery=%+v error=%v", got, err)
	}
	peer := scriptpollers.CursorScope{RuntimeID: "runtime-b", BaseDir: t.TempDir()}
	commitScopedCursor(t, owner, peer, "", "peer-cursor")
	assertScopedCursor(t, owner, scope, "opaque / page=7")
	assertScopedCursor(t, owner, peer, "peer-cursor")
}

func TestCursorScopes_FailedReplacementLeavesCommittedRecovery(t *testing.T) {
	t.Parallel()
	scope := scriptpollers.CursorScope{RuntimeID: "runtime-a", BaseDir: t.TempDir()}
	owner := scriptpollerswire.NewCursorScopes(osFileSystem{})
	commitScopedCursor(t, owner, scope, "", "committed")
	fault := errors.New("replacement unavailable")
	failing := scriptpollerswire.NewCursorScopes(osFileSystem{renameErr: fault})
	err := failing.CommitCursor(context.Background(), scope, scriptpollers.CommitCursorRequest{
		InstanceID: "shared-instance", ExpectedCursor: "committed", Cursor: "uncommitted",
	})
	if !errors.Is(err, fault) {
		t.Fatalf("replacement error=%v, want injected fault", err)
	}
	assertScopedCursor(t, failing, scope, "committed")
}

func TestCursorScopes_ConcurrentExpectedCursorHasOneWinner(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, baseDir string }{
		{name: "memory"}, {name: "durable", baseDir: t.TempDir()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			owner := scriptpollerswire.NewCursorScopes(osFileSystem{})
			scope := scriptpollers.CursorScope{RuntimeID: "runtime", BaseDir: test.baseDir}
			commitScopedCursor(t, owner, scope, "", "initial")
			results := make(chan error, 2)
			var writers sync.WaitGroup
			for _, next := range []automations.Cursor{"next-a", "next-b"} {
				writers.Add(1)
				go func(next automations.Cursor) {
					defer writers.Done()
					results <- owner.CommitCursor(context.Background(), scope, scriptpollers.CommitCursorRequest{
						InstanceID: "shared-instance", ExpectedCursor: "initial", Cursor: next,
					})
				}(next)
			}
			writers.Wait()
			first, second := <-results, <-results
			if first != nil {
				first, second = second, first
			}
			if first != nil || !errors.Is(second, automations.ErrConflict) {
				t.Fatalf("concurrent replacement errors=(%v,%v), want success and conflict", first, second)
			}
		})
	}
}

func TestCursorScopes_BlockedDurableReadDoesNotBlockPeer(t *testing.T) {
	t.Parallel()
	files := blockingCursorRead{entered: make(chan struct{}), release: make(chan struct{})}
	owner := scriptpollerswire.NewCursorScopes(files)
	blocked := scriptpollers.CursorScope{RuntimeID: "blocked", BaseDir: t.TempDir()}
	peer := scriptpollers.CursorScope{RuntimeID: "peer"}
	commitScopedCursor(t, owner, peer, "", "prior-peer-cursor")
	finished := make(chan error, 1)
	go func() {
		_, err := owner.GetCursor(context.Background(), blocked, automations.GetCursorRequest{InstanceID: "shared-instance"})
		finished <- err
	}()
	t.Cleanup(func() {
		close(files.release)
		if err := <-finished; !errors.Is(err, automations.ErrNotFound) {
			t.Errorf("released read error=%v, want not found", err)
		}
	})
	select {
	case <-files.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("durable read did not reach injected filesystem")
	}
	progress := make(chan error, 1)
	go func() {
		// Release concerns the joined peer only; blocked still owns its read.
		owner.ReleaseScope(peer)
		progress <- owner.CommitCursor(context.Background(), peer, scriptpollers.CommitCursorRequest{
			InstanceID: "shared-instance", Cursor: "peer-cursor", Checkpoint: "checkpoint-peer-cursor",
		})
	}()
	select {
	case err := <-progress:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("peer cursor commit blocked behind another scope's IO")
	}
	assertScopedCursor(t, owner, peer, "peer-cursor")
}

type blockingCursorRead struct {
	unexpectedCursorIO
	entered chan struct{}
	release chan struct{}
}

func (f blockingCursorRead) ReadFile(string) ([]byte, error) {
	close(f.entered)
	<-f.release
	return nil, fs.ErrNotExist
}

func commitScopedCursor(t *testing.T, owner scriptpollers.CursorScopes, scope scriptpollers.CursorScope, expected, cursor automations.Cursor) {
	t.Helper()
	if err := owner.CommitCursor(context.Background(), scope, scriptpollers.CommitCursorRequest{
		AutomationID: "automation", InstanceID: "shared-instance", ExpectedCursor: expected,
		Cursor: cursor, Checkpoint: "checkpoint-" + string(cursor),
	}); err != nil {
		t.Fatal(err)
	}
}

func assertScopedCursor(t *testing.T, owner scriptpollers.CursorScopes, scope scriptpollers.CursorScope, cursor automations.Cursor) {
	t.Helper()
	got, err := owner.GetCursor(context.Background(), scope, automations.GetCursorRequest{InstanceID: "shared-instance"})
	if err != nil || got.Cursor != cursor || got.Checkpoint != "checkpoint-"+string(cursor) {
		t.Fatalf("recovery=%+v error=%v, want cursor=%q with exact checkpoint", got, err, cursor)
	}
}

type unexpectedCursorIO struct{}

func (unexpectedCursorIO) ReadFile(string) ([]byte, error) { panic("unexpected cursor read") }
func (unexpectedCursorIO) MkdirAll(string, fs.FileMode) error {
	panic("unexpected cursor directory creation")
}
func (unexpectedCursorIO) WriteFile(string, []byte, fs.FileMode) error {
	panic("unexpected cursor write")
}
func (unexpectedCursorIO) Rename(string, string) error { panic("unexpected cursor replacement") }
