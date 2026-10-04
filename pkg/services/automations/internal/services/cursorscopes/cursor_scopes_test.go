package cursorscopes_test

import (
	"context"
	"errors"
	"io/fs"
	"sync"
	"testing"
	"time"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	cursorscopes "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes"
	cursorscopeswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cursorscopes/wire"
)

func TestCursorScopes_BlankBaseUsesIsolatedMemoryWithoutIO(t *testing.T) {
	t.Parallel()
	owner := cursorscopeswire.NewService(unexpectedCursorIO{})
	ctx := context.Background()
	for _, runtimeID := range []string{"", "runtime-a", "runtime-b"} {
		scope := cursorscopes.CursorScope{RuntimeID: runtimeID, BaseDir: " "}
		commitScopedCursor(t, owner, scope, "", automations.Cursor("cursor-"+runtimeID))
	}
	for _, runtimeID := range []string{"", "runtime-a", "runtime-b"} {
		scope := cursorscopes.CursorScope{RuntimeID: runtimeID}
		got, err := owner.GetCursor(ctx, scope, automations.GetCursorRequest{InstanceID: "shared-instance"})
		if err != nil || got.Cursor != automations.Cursor("cursor-"+runtimeID) || got.Checkpoint != "checkpoint-cursor-"+runtimeID {
			t.Fatalf("memory scope %q: cursor=%+v error=%v", runtimeID, got, err)
		}
	}
}

func TestCursorScopes_ReleasedMemoryRestartsEmptyAndRetainsPeers(t *testing.T) {
	t.Parallel()
	owner := cursorscopeswire.NewService(unexpectedCursorIO{})
	scope := cursorscopes.CursorScope{RuntimeID: "runtime-a", BaseDir: " "}
	peer := cursorscopes.CursorScope{RuntimeID: "runtime-b"}
	detached := cursorscopes.CursorScope{}
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
	owner := cursorscopeswire.NewService(osFileSystem{})
	scope := cursorscopes.CursorScope{RuntimeID: "runtime-a", BaseDir: t.TempDir()}
	peer := cursorscopes.CursorScope{RuntimeID: "runtime-b", BaseDir: t.TempDir()}
	commitScopedCursor(t, owner, scope, "", "committed-a")
	commitScopedCursor(t, owner, peer, "", "committed-b")

	owner.ReleaseScope(scope)
	owner.ReleaseScope(scope)
	assertScopedCursor(t, owner, scope, "committed-a")
	commitScopedCursor(t, owner, scope, "committed-a", "resumed-a")
	assertScopedCursor(t, owner, scope, "resumed-a")
	assertScopedCursor(t, owner, peer, "committed-b")
	fresh := cursorscopeswire.NewService(osFileSystem{})
	assertScopedCursor(t, fresh, scope, "resumed-a")
}

func TestCursorScopes_DurableRecoveryRetainsAuthoredDestination(t *testing.T) {
	t.Parallel()
	owner := cursorscopeswire.NewService(osFileSystem{})
	scope := cursorscopes.CursorScope{RuntimeID: "runtime-a", BaseDir: t.TempDir()}
	commitScopedCursor(t, owner, scope, "", "opaque / page=7")
	// Runtime identity must not change existing customer-selected durable paths.
	restarted := cursorscopeswire.NewService(osFileSystem{})
	scope.RuntimeID = "runtime-replacement"
	assertScopedCursor(t, restarted, scope, "opaque / page=7")
	peer := cursorscopes.CursorScope{RuntimeID: "runtime-b", BaseDir: t.TempDir()}
	commitScopedCursor(t, owner, peer, "", "peer-cursor")
	assertScopedCursor(t, owner, scope, "opaque / page=7")
	assertScopedCursor(t, owner, peer, "peer-cursor")
}

func TestCursorScopes_FailedReplacementLeavesCommittedRecovery(t *testing.T) {
	t.Parallel()
	fault := errors.New("replacement unavailable")
	for _, test := range []struct {
		name  string
		files osFileSystem
	}{
		{name: "write", files: osFileSystem{writeErr: fault}},
		{name: "rename", files: osFileSystem{renameErr: fault}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			scope := cursorscopes.CursorScope{RuntimeID: "runtime-a", BaseDir: t.TempDir()}
			owner := cursorscopeswire.NewService(osFileSystem{})
			commitScopedCursor(t, owner, scope, "", "committed")
			failing := cursorscopeswire.NewService(test.files)
			err := failing.CommitCursor(context.Background(), scope, cursorscopes.CommitCursorRequest{
				InstanceID: "shared-instance", ExpectedCursor: "committed", Cursor: "uncommitted",
			})
			if !errors.Is(err, fault) {
				t.Fatalf("replacement error=%v, want injected fault", err)
			}
			assertScopedCursor(t, failing, scope, "committed")
			assertScopedCursor(t, cursorscopeswire.NewService(osFileSystem{}), scope, "committed")
		})
	}
}

func TestCursorScopes_ConcurrentExpectedCursorHasOneWinner(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, baseDir string }{
		{name: "memory"}, {name: "durable", baseDir: t.TempDir()},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			owner := cursorscopeswire.NewService(osFileSystem{})
			scope := cursorscopes.CursorScope{RuntimeID: "runtime", BaseDir: test.baseDir}
			commitScopedCursor(t, owner, scope, "", "initial")
			results := make(chan error, 2)
			var writers sync.WaitGroup
			for _, next := range []automations.Cursor{"next-a", "next-b"} {
				writers.Add(1)
				go func(next automations.Cursor) {
					defer writers.Done()
					results <- owner.CommitCursor(context.Background(), scope, cursorscopes.CommitCursorRequest{
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
	owner := cursorscopeswire.NewService(files)
	blocked := cursorscopes.CursorScope{RuntimeID: "blocked", BaseDir: t.TempDir()}
	peer := cursorscopes.CursorScope{RuntimeID: "peer"}
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
		progress <- owner.CommitCursor(context.Background(), peer, cursorscopes.CommitCursorRequest{
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

func commitScopedCursor(t *testing.T, owner cursorscopes.CursorScopes, scope cursorscopes.CursorScope, expected, cursor automations.Cursor) {
	t.Helper()
	if err := owner.CommitCursor(context.Background(), scope, cursorscopes.CommitCursorRequest{
		AutomationID: "automation", InstanceID: "shared-instance", ExpectedCursor: expected,
		Cursor: cursor, Checkpoint: "checkpoint-" + string(cursor),
	}); err != nil {
		t.Fatal(err)
	}
}

func assertScopedCursor(t *testing.T, owner cursorscopes.CursorScopes, scope cursorscopes.CursorScope, cursor automations.Cursor) {
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
