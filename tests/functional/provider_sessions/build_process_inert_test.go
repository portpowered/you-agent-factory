package provider_sessions

import (
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"sync/atomic"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingProviderSessionEffect = errors.New("recording provider session effect invoked during BuildProcess")

// TestProviderSessionsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Provider Sessions without invoking session storage
// discovery effects—directory walks, symlink resolution, filesystem opens, or
// Cursor database opens—before runtime lifecycle starts.
func TestProviderSessionsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newProviderSessionEffectRecorder(t)
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.codexWalkCalls(); got != 0 {
		t.Fatalf("Codex directory walk calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.codexSymlinkCalls(); got != 0 {
		t.Fatalf("Codex symlink resolution calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.cursorWalkCalls(); got != 0 {
		t.Fatalf("Cursor directory walk calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.cursorSymlinkCalls(); got != 0 {
		t.Fatalf("Cursor symlink resolution calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.cursorDatabaseCalls(); got != 0 {
		t.Fatalf("Cursor database open calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.fileOpenCalls(); got != 0 {
		t.Fatalf("provider session filesystem open calls = %d during BuildProcess, want 0", got)
	}
}

type providerSessionEffectRecorder struct {
	t                   testing.TB
	homeCalls           atomic.Int32
	fileStatCalls       atomic.Int32
	fileOpenCount       atomic.Int32
	codexWalkCount      atomic.Int32
	codexSymlinkCount   atomic.Int32
	cursorWalkCount     atomic.Int32
	cursorSymlinkCount  atomic.Int32
	cursorDatabaseCount atomic.Int32
}

func newProviderSessionEffectRecorder(t testing.TB) *providerSessionEffectRecorder {
	t.Helper()
	return &providerSessionEffectRecorder{t: t}
}

func (recorder *providerSessionEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		ProviderSessionResolveHomeDirectory:  recorder.recordHome,
		ProviderSessionFileSystem:            recorder,
		ProviderSessionCodexWalkDirectory:    recorder.recordCodexWalk,
		ProviderSessionCodexResolveSymlinks:  recorder.recordCodexSymlink,
		ProviderSessionCursorWalkDirectory:   recorder.recordCursorWalk,
		ProviderSessionCursorResolveSymlinks: recorder.recordCursorSymlink,
		ProviderSessionCursorOpenDatabase:    recorder.recordCursorDatabase,
	}
}

func (recorder *providerSessionEffectRecorder) recordHome() (string, error) {
	recorder.homeCalls.Add(1)
	return recorder.t.TempDir(), nil
}

func (recorder *providerSessionEffectRecorder) recordCodexWalk(string, fs.WalkDirFunc) error {
	recorder.codexWalkCount.Add(1)
	return errRecordingProviderSessionEffect
}

func (recorder *providerSessionEffectRecorder) recordCodexSymlink(string) (string, error) {
	recorder.codexSymlinkCount.Add(1)
	return "", errRecordingProviderSessionEffect
}

func (recorder *providerSessionEffectRecorder) recordCursorWalk(string, fs.WalkDirFunc) error {
	recorder.cursorWalkCount.Add(1)
	return errRecordingProviderSessionEffect
}

func (recorder *providerSessionEffectRecorder) recordCursorSymlink(string) (string, error) {
	recorder.cursorSymlinkCount.Add(1)
	return "", errRecordingProviderSessionEffect
}

func (recorder *providerSessionEffectRecorder) recordCursorDatabase(string, string) (*sql.DB, error) {
	recorder.cursorDatabaseCount.Add(1)
	return nil, errRecordingProviderSessionEffect
}

func (recorder *providerSessionEffectRecorder) Open(string) (io.ReadCloser, error) {
	recorder.fileOpenCount.Add(1)
	return nil, errRecordingProviderSessionEffect
}

func (recorder *providerSessionEffectRecorder) Stat(string) (fs.FileInfo, error) {
	recorder.fileStatCalls.Add(1)
	return nil, fs.ErrNotExist
}

func (recorder *providerSessionEffectRecorder) codexWalkCalls() int32 {
	return recorder.codexWalkCount.Load()
}

func (recorder *providerSessionEffectRecorder) codexSymlinkCalls() int32 {
	return recorder.codexSymlinkCount.Load()
}

func (recorder *providerSessionEffectRecorder) cursorWalkCalls() int32 {
	return recorder.cursorWalkCount.Load()
}

func (recorder *providerSessionEffectRecorder) cursorSymlinkCalls() int32 {
	return recorder.cursorSymlinkCount.Load()
}

func (recorder *providerSessionEffectRecorder) cursorDatabaseCalls() int32 {
	return recorder.cursorDatabaseCount.Load()
}

func (recorder *providerSessionEffectRecorder) fileOpenCalls() int32 {
	return recorder.fileOpenCount.Load()
}
