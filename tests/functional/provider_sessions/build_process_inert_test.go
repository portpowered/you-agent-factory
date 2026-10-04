package provider_sessions

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"io/fs"
	"strings"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
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
	if got := recorder.providerCommandCount.Load(); got != 0 {
		t.Fatalf("provider command calls = %d during BuildProcess, want 0", got)
	}
}

type providerSessionEffectRecorder struct {
	t                    testing.TB
	homeCalls            atomic.Int32
	fileStatCalls        atomic.Int32
	fileOpenCount        atomic.Int32
	codexWalkCount       atomic.Int32
	codexSymlinkCount    atomic.Int32
	cursorWalkCount      atomic.Int32
	cursorSymlinkCount   atomic.Int32
	cursorDatabaseCount  atomic.Int32
	providerCommandCount atomic.Int32
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
		ProviderCommandRunner:                recorder,
	}
}

func (recorder *providerSessionEffectRecorder) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	recorder.providerCommandCount.Add(1)
	return platformprocess.CommandResult{}, errRecordingProviderSessionEffect
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

// A home lookup failure prevents construction, so no detail request can be
// issued. Keep this separate from the reusable successful process: its immutable
// home edge intentionally cannot construct that process.
func TestProviderSessionsHomeResolutionFailureOutcome(t *testing.T) {
	t.Parallel()

	recorder := newProviderSessionEffectRecorder(t)
	edges := recorder.edges()
	want := errors.New("provider-session home lookup failed")
	edges.ProviderSessionResolveHomeDirectory = func() (string, error) {
		recorder.homeCalls.Add(1)
		return "", want
	}

	process, err := root.BuildProcess(context.Background(), edges)
	if process != nil {
		t.Cleanup(func() { _ = process.Close(context.Background()) })
		t.Fatal("BuildProcess returned a process after home resolution failed")
	}
	if !errors.Is(err, want) {
		t.Fatalf("BuildProcess error = %v, want wrapped home lookup sentinel", err)
	}
	if !strings.HasPrefix(err.Error(), "build application process: home directory:") {
		t.Fatalf("BuildProcess error = %v, want public construction and home directory wrapping", err)
	}
	if got := recorder.homeCalls.Load(); got != 1 {
		t.Fatalf("home lookup calls = %d, want one failed lookup", got)
	}
	for name, calls := range map[string]int32{
		"candidate stat":   recorder.fileStatCalls.Load(),
		"file open":        recorder.fileOpenCalls(),
		"Codex walk":       recorder.codexWalkCalls(),
		"Codex symlink":    recorder.codexSymlinkCalls(),
		"Cursor walk":      recorder.cursorWalkCalls(),
		"Cursor symlink":   recorder.cursorSymlinkCalls(),
		"Cursor database":  recorder.cursorDatabaseCalls(),
		"provider command": recorder.providerCommandCount.Load(),
	} {
		if calls != 0 {
			t.Errorf("%s calls = %d after failed home lookup, want 0", name, calls)
		}
	}
}
