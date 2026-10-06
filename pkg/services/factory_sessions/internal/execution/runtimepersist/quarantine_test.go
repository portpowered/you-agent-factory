package runtimepersist_test

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	platformreplay "github.com/portpowered/infinite-you/pkg/platform/replay"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution/runtimepersist"
)

type quarantineFiles struct {
	boardReferenceFiles
	contents   map[string][]byte
	moves      int
	failSource string
	failure    error
	afterMove  func()
}

func (files *quarantineFiles) RenameNoReplace(source, destination string) error {
	files.moves++
	if source == files.failSource {
		return files.failure
	}
	data, exists := files.contents[source]
	if !exists {
		return fs.ErrNotExist
	}
	if _, exists := files.contents[destination]; exists {
		return fs.ErrExist
	}
	files.contents[destination] = data
	delete(files.contents, source)
	if files.afterMove != nil {
		files.afterMove()
	}
	return nil
}

func (files *quarantineFiles) WriteFile(path string, data []byte, _ fs.FileMode) error {
	files.contents[path] = append([]byte(nil), data...)
	return nil
}

func newQuarantineStore(t *testing.T) (runtimepersist.Store, *quarantineFiles, string, string) {
	t.Helper()
	root := t.TempDir()
	files := &quarantineFiles{contents: make(map[string][]byte)}
	store, err := runtimepersist.NewLazyProjectStore(root, files)
	if err != nil {
		t.Fatal(err)
	}
	return store, files, runtimepersist.SnapshotPathForProjectRoot(root, "~default"), filepath.Join(root, ".you-agent-factory", "current-board.json")
}

func TestCurrentBoardQuarantinePreservesBytesReferenceCollisionsAndFreshWrites(t *testing.T) {
	t.Parallel()
	store, files, snapshot, reference := newQuarantineStore(t)
	damaged := []byte("{\"prompt\":\"private § —\",\x00")
	priorReference := []byte("prior-reference")
	files.contents[snapshot], files.contents[reference] = damaged, priorReference
	at := time.Date(2026, 10, 6, 11, 0, 0, 123, time.FixedZone("offset", -7*60*60))
	suffix := ".unreadable.20261006T180000000000123Z.unique-id"
	files.contents[snapshot+suffix] = []byte("older archive")
	files.contents[reference+suffix] = []byte("older reference")
	quarantine := store.(runtimepersist.CurrentBoardQuarantineStore)
	archive, err := quarantine.QuarantineCurrentBoard(t.Context(), at, "unique-id")
	if err != nil || archive != snapshot+suffix+"-1" {
		t.Fatalf("quarantine = %q, %v", archive, err)
	}
	if _, exists := files.contents[snapshot]; exists {
		t.Fatal("snapshot remained selected after successful quarantine")
	}
	if _, exists := files.contents[reference]; exists {
		t.Fatal("prior reference remained selected after successful quarantine")
	}
	if err := store.Save("~default", []byte("fresh snapshot")); err != nil {
		t.Fatal(err)
	}
	if err := store.(runtimepersist.CurrentBoardStore).SaveCurrentBoard(t.Context(), filepath.Dir(snapshot), filepath.Join(t.TempDir(), "fresh.json")); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string][]byte{
		archive: damaged, reference + suffix + "-1": priorReference,
		snapshot + suffix: []byte("older archive"), reference + suffix: []byte("older reference"),
	} {
		if !bytes.Equal(files.contents[path], want) {
			t.Fatalf("preserved bytes changed at %s", path)
		}
	}
	// A second opening using even the same injected identity cannot overwrite
	// either prior archive, and preserves the new snapshot separately.
	second, err := quarantine.QuarantineCurrentBoard(t.Context(), at, "unique-id")
	if err != nil || second != snapshot+suffix+"-2" || string(files.contents[second]) != "fresh snapshot" || !bytes.Equal(files.contents[archive], damaged) {
		t.Fatalf("repeated quarantine = %q, %v", second, err)
	}
}

func TestCurrentBoardQuarantineFailuresNeverReportSuccessOrLoseEvidence(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"snapshot", "reference", "canceled-before", "canceled-after-snapshot", "canceled-after-reference", "missing-snapshot", "missing-reference", "collision-exhaustion", "invalid-identity"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			store, files, snapshot, reference := newQuarantineStore(t)
			files.contents[snapshot], files.contents[reference] = []byte("damaged"), []byte("reference")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			identity, fault := configureQuarantineFault(name, files, snapshot, reference, cancel)
			archive, err := store.(runtimepersist.CurrentBoardQuarantineStore).QuarantineCurrentBoard(ctx, time.Time{}, identity)
			if name == "missing-reference" {
				if err != nil || string(files.contents[archive]) != "damaged" {
					t.Fatalf("missing reference = %q, %v", archive, err)
				}
				return
			}
			assertQuarantineFailure(t, name, archive, err, fault)
			if name != "missing-snapshot" {
				assertQuarantineEvidence(t, files, "damaged")
			}
			assertQuarantineEvidence(t, files, "reference")
			if (name == "canceled-before" || name == "invalid-identity") && files.moves != 0 {
				t.Fatal("invalid request mutated evidence")
			}
			if name == "collision-exhaustion" && files.moves != 16 {
				t.Fatalf("unbounded collision retries: %d", files.moves)
			}
		})
	}
}

func configureQuarantineFault(name string, files *quarantineFiles, snapshot, reference string, cancel context.CancelFunc) (string, error) {
	fault := errors.New("credential=private prompt")
	switch name {
	case "snapshot", "reference":
		files.failSource, files.failure = snapshot, fault
		if name == "reference" {
			files.failSource = reference
		}
	case "canceled-before":
		cancel()
	case "canceled-after-snapshot", "canceled-after-reference":
		files.afterMove = func() {
			if name == "canceled-after-snapshot" || files.moves == 2 {
				cancel()
			}
		}
	case "missing-snapshot":
		delete(files.contents, snapshot)
	case "missing-reference":
		delete(files.contents, reference)
	case "collision-exhaustion":
		files.failSource, files.failure = snapshot, fs.ErrExist
	case "invalid-identity":
		return "../escape", fault
	}
	return "id", fault
}

func assertQuarantineFailure(t *testing.T, name, archive string, err, fault error) {
	t.Helper()
	if err == nil || archive != "" {
		t.Fatalf("false quarantine success = %q, %v", archive, err)
	}
	if strings.HasPrefix(name, "canceled") && !errors.Is(err, context.Canceled) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if name == "snapshot" || name == "reference" {
		var diagnostic interface{ CLIErrorMessage() string }
		if !errors.Is(err, fault) || !errors.As(err, &diagnostic) || strings.Contains(diagnostic.CLIErrorMessage(), "private") {
			t.Fatalf("unsafe failure classification: %v", err)
		}
	}
}

func assertQuarantineEvidence(t *testing.T, files *quarantineFiles, want string) {
	t.Helper()
	for _, data := range files.contents {
		if string(data) == want {
			return
		}
	}
	t.Fatalf("lost %s evidence", want)
}

func TestCurrentBoardQuarantineLocalFilesSurviveFreshSnapshotAndReference(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	files := &interruptingStorage{delegate: platformreplay.NewLocal(runtime.GOOS)}
	store, err := runtimepersist.NewLazyProjectStore(root, files)
	if err != nil {
		t.Fatal(err)
	}
	bad := []byte("{\"prompt\":\"private § —\",\x00")
	if err := store.Save("~default", bad); err != nil {
		t.Fatal(err)
	}
	references := store.(runtimepersist.CurrentBoardStore)
	factory := filepath.Join(root, "factory")
	if err := references.SaveCurrentBoard(t.Context(), factory, filepath.Join(root, "old-recording.json")); err != nil {
		t.Fatal(err)
	}
	reference := filepath.Join(root, ".you-agent-factory", "current-board.json")
	prior, err := os.ReadFile(reference)
	if err != nil {
		t.Fatal(err)
	}
	archive, err := store.(runtimepersist.CurrentBoardQuarantineStore).QuarantineCurrentBoard(t.Context(), time.Time{}, "local-id")
	if err != nil {
		t.Fatal(err)
	}
	snapshot := runtimepersist.SnapshotPathForProjectRoot(root, "~default")
	if _, err := os.Stat(snapshot); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("snapshot still selected: %v", err)
	}
	if err := store.Save("~default", []byte("fresh snapshot")); err != nil {
		t.Fatal(err)
	}
	if err := references.SaveCurrentBoard(t.Context(), factory, filepath.Join(root, "new-recording.json")); err != nil {
		t.Fatal(err)
	}
	for path, expected := range map[string][]byte{
		archive: bad, reference + strings.TrimPrefix(archive, snapshot): prior,
	} {
		got, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(got, expected) {
			t.Fatalf("archive changed after fresh publication at %s: %v", path, err)
		}
	}
}

func TestCurrentBoardArtifactQuarantinePreservesSelectedAndRelatedBytes(t *testing.T) {
	t.Parallel()
	for _, cell := range []string{"reference", "recording", "denied", "cancel"} {
		t.Run(cell, func(t *testing.T) {
			t.Parallel()
			store, files, snapshot, reference := newQuarantineStore(t)
			recording := filepath.Join(filepath.Dir(reference), "history.jsonl")
			files.contents[snapshot], files.contents[reference], files.contents[recording] = []byte("snapshot"), []byte("reference"), []byte("recording")
			source, selected := recording, recording
			if cell == "reference" {
				source, selected = reference, ""
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if cell == "denied" {
				files.failSource, files.failure = source, fs.ErrPermission
			}
			if cell == "cancel" {
				files.afterMove = cancel
			}
			quarantine := store.(interface {
				QuarantineCurrentBoardArtifact(context.Context, time.Time, string, string) (string, string, error)
			})
			file, archive, err := quarantine.QuarantineCurrentBoardArtifact(ctx, time.Time{}, "artifact-id", selected)
			failed := cell == "denied" || cell == "cancel"
			if failed {
				if err == nil || file != "" || archive != "" {
					t.Fatal("failed preservation claimed success")
				}
			} else if err != nil || file != source || !bytes.Equal(files.contents[archive], []byte(cell)) {
				t.Fatalf("archive %s, %s, %v", file, archive, err)
			}
			for _, data := range []string{"snapshot", "reference", "recording"} {
				retained := false
				for _, got := range files.contents {
					if string(got) == data {
						retained = true
					}
				}
				if !retained {
					t.Fatalf("lost %s evidence", data)
				}
			}
		})
	}
}
