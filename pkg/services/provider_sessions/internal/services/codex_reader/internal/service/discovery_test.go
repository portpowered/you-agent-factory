package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providersessions "github.com/portpowered/infinite-you/pkg/services/provider_sessions"
	codexreader "github.com/portpowered/infinite-you/pkg/services/provider_sessions/internal/services/codex_reader"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type trackingFileSystem struct {
	base       providersessions.FileSystem
	cancel     context.CancelFunc
	cancelStat int
	openCalls  int
	statCalls  int
}

func (f *trackingFileSystem) Open(path string) (io.ReadCloser, error) {
	f.openCalls++
	return f.base.Open(path)
}

func (f *trackingFileSystem) Stat(path string) (fs.FileInfo, error) {
	f.statCalls++
	info, err := f.base.Stat(path)
	if f.cancel != nil && f.statCalls == f.cancelStat {
		f.cancel()
	}
	return info, err
}

func TestReaderDetailsCancellationStopsDiscoveryEffects(t *testing.T) {
	t.Run("before discovery", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		files := &trackingFileSystem{base: platformfilesystem.Local{}}
		walkCalls, resolveCalls := 0, 0
		reader := newTestReader(t, files,
			func(string, fs.WalkDirFunc) error {
				walkCalls++
				return nil
			},
			func(path string) (string, error) {
				resolveCalls++
				return path, nil
			},
			t.TempDir(),
		)

		_, err := reader.Details(ctx, codexSessionRef("canceled"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Details error = %v, want context.Canceled", err)
		}
		if files.statCalls != 0 || files.openCalls != 0 || walkCalls != 0 || resolveCalls != 0 {
			t.Fatalf("effects after cancellation: stat=%d open=%d walk=%d resolve=%d", files.statCalls, files.openCalls, walkCalls, resolveCalls)
		}
	})

	t.Run("during walk", func(t *testing.T) {
		root := t.TempDir()
		writeCodexFixture(t, root, "rollout-canceled.jsonl")
		ctx, cancel := context.WithCancel(context.Background())
		files := &trackingFileSystem{base: platformfilesystem.Local{}}
		resolveCalls := 0
		reader := newTestReader(t, files,
			func(path string, fn fs.WalkDirFunc) error {
				return filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
					cancel()
					return fn(path, entry, walkErr)
				})
			},
			func(path string) (string, error) {
				resolveCalls++
				return filepath.EvalSymlinks(path)
			},
			root,
		)

		_, err := reader.Details(ctx, codexSessionRef("canceled"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Details error = %v, want context.Canceled", err)
		}
		if files.statCalls != 1 || files.openCalls != 0 || resolveCalls != 1 {
			t.Fatalf("effects after walk cancellation: stat=%d open=%d resolve=%d", files.statCalls, files.openCalls, resolveCalls)
		}
	})

	t.Run("after candidate stat", func(t *testing.T) {
		root := t.TempDir()
		writeCodexFixture(t, root, "rollout-canceled.jsonl")
		ctx, cancel := context.WithCancel(context.Background())
		files := &trackingFileSystem{
			base:       platformfilesystem.Local{},
			cancel:     cancel,
			cancelStat: 2,
		}
		reader := newTestReader(t, files, filepath.WalkDir, filepath.EvalSymlinks, root)

		_, err := reader.Details(ctx, codexSessionRef("canceled"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Details error = %v, want context.Canceled", err)
		}
		if files.statCalls != 2 || files.openCalls != 0 {
			t.Fatalf("effects after stat cancellation: stat=%d open=%d", files.statCalls, files.openCalls)
		}
	})
}

func TestReaderDetailsCancellationDuringSymlinkResolutionStopsStatAndOpen(t *testing.T) {
	root := t.TempDir()
	writeCodexFixture(t, root, "rollout-canceled.jsonl")
	ctx, cancel := context.WithCancel(context.Background())
	files := &trackingFileSystem{base: platformfilesystem.Local{}}
	resolveCalls := 0
	reader := newTestReader(t, files, filepath.WalkDir,
		func(path string) (string, error) {
			resolveCalls++
			resolved, err := filepath.EvalSymlinks(path)
			if resolveCalls == 2 {
				cancel()
			}
			return resolved, err
		},
		root,
	)

	_, err := reader.Details(ctx, codexSessionRef("canceled"))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Details error = %v, want context.Canceled", err)
	}
	if resolveCalls != 2 || files.statCalls != 1 || files.openCalls != 0 {
		t.Fatalf("effects after resolve cancellation: resolve=%d stat=%d open=%d", resolveCalls, files.statCalls, files.openCalls)
	}
}

func TestDiscoveryRejectsNonRegularCandidate(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "rollout-directory.jsonl"), 0o755); err != nil {
		t.Fatalf("mkdir candidate: %v", err)
	}

	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, root, "directory")
	if !errors.Is(err, providersessions.ErrSessionSourceNotRegularFile) {
		t.Fatalf("LoadDetails error = %v, want ErrSessionSourceNotRegularFile", err)
	}
}

func TestDiscoveryRejectsEmptyConfiguredRoot(t *testing.T) {
	_, err := LoadDetails(testFiles, testWalkDirectory, testResolveSymlinks, "", "session")
	if !errors.Is(err, providersessions.ErrSessionStorageUnavailable) {
		t.Fatalf("LoadDetails error = %v, want ErrSessionStorageUnavailable", err)
	}
}

func TestDiscoveryReturnsSafeStorageFailure(t *testing.T) {
	root := t.TempDir()
	hostPath := filepath.Join(root, "private", "credentials")
	_, err := LoadDetails(
		testFiles,
		func(string, fs.WalkDirFunc) error {
			return fmt.Errorf("permission denied reading %s", hostPath)
		},
		testResolveSymlinks,
		root,
		"session",
	)
	if !errors.Is(err, providersessions.ErrSessionStorageUnavailable) {
		t.Fatalf("LoadDetails error = %v, want ErrSessionStorageUnavailable", err)
	}
	if strings.Contains(err.Error(), root) || strings.Contains(err.Error(), hostPath) {
		t.Fatalf("public error exposed host path: %v", err)
	}
}

func TestSelectionIsDeterministicAndExactWins(t *testing.T) {
	exact := resolvedCodexSessionFile{relativePath: "z/rollout-session.jsonl", layout: codexSessionFileLayoutExact}
	timestamp := resolvedCodexSessionFile{relativePath: "a/rollout-2026-01-01T00-00-00-session.jsonl", layout: codexSessionFileLayoutTimestampPrefixed}
	for _, candidates := range [][]resolvedCodexSessionFile{
		{exact, timestamp},
		{timestamp, exact},
	} {
		got, err := selectResolvedCodexSessionFile(candidates)
		if err != nil {
			t.Fatalf("selectResolvedCodexSessionFile: %v", err)
		}
		if got.relativePath != exact.relativePath {
			t.Fatalf("selected %q, want exact %q", got.relativePath, exact.relativePath)
		}
	}
}

func TestDiscoveryMetadataIsContainedNormalizedAndUTC(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "2026", "07", "27", "rollout-session.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture: %v", err)
	}
	if err := os.WriteFile(path, []byte("{\"type\":\"session_meta\"}\n"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	modifiedAt := time.Date(2026, time.July, 27, 9, 30, 0, 0, time.FixedZone("fixture", 5*60*60))
	if err := os.Chtimes(path, modifiedAt, modifiedAt); err != nil {
		t.Fatalf("set fixture time: %v", err)
	}

	source, err := Resolve(testFiles, testWalkDirectory, testResolveSymlinks, root, "session")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if source.RelativePath != "2026/07/27/rollout-session.jsonl" {
		t.Fatalf("RelativePath = %q, want normalized contained path", source.RelativePath)
	}
	if source.ModifiedAt == nil || source.ModifiedAt.Location() != time.UTC || !source.ModifiedAt.Equal(modifiedAt) {
		t.Fatalf("ModifiedAt = %v, want %v in UTC", source.ModifiedAt, modifiedAt)
	}
}

func newTestReader(
	t *testing.T,
	files providersessions.FileSystem,
	walk providersessions.CodexWalkDirectory,
	resolve providersessions.CodexResolveSymlinks,
	root string,
) codexreader.Service {
	t.Helper()
	reader, err := New(codexreader.Dependencies{
		Files:           files,
		WalkDirectory:   walk,
		ResolveSymlinks: resolve,
		SessionsRoot:    root,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return reader
}

func codexSessionRef(id string) providers.SessionRef {
	return providers.SessionRef{Provider: providers.IDCodex, Kind: providers.SessionIDKind, ID: id}
}
