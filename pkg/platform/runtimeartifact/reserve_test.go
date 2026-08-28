package runtimeartifact

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	internalartifact "github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
)

type localFileSystem struct{}

func (localFileSystem) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}

func (localFileSystem) OpenFile(path string, flag int, mode fs.FileMode) (io.WriteCloser, error) {
	return os.OpenFile(path, flag, mode)
}

func TestNewReserverRequiresFileSystem(t *testing.T) {
	reserver, err := NewReserver(nil)
	if err == nil || reserver != nil {
		t.Fatalf("NewReserver(nil) = (%v, %v), want nil and error", reserver, err)
	}
}

func TestReserverCreatesDatedPathAndAvoidsExistingArtifact(t *testing.T) {
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	first, err := reserver.Reserve(root, at, "log", "runtime-fixed")
	if err != nil {
		t.Fatalf("Reserve first: %v", err)
	}
	if err := os.WriteFile(first, []byte("preserve"), 0o644); err != nil {
		t.Fatalf("WriteFile(first): %v", err)
	}
	second, err := reserver.Reserve(root, at, "log", "runtime-fixed")
	if err != nil {
		t.Fatalf("Reserve second: %v", err)
	}
	if first == second {
		t.Fatalf("collision returned the same path %q", first)
	}
	if got, err := os.ReadFile(first); err != nil || string(got) != "preserve" {
		t.Fatalf("first artifact = %q, %v; want preserved contents", got, err)
	}
	wantParent := filepath.Join(root, "2026", "05", "29")
	if filepath.Dir(first) != wantParent || filepath.Dir(second) != wantParent {
		t.Fatalf("reserved parents = %q, %q; want %q", filepath.Dir(first), filepath.Dir(second), wantParent)
	}
}

func TestReserverReserveNamedWithCollisionUsesUTCNameAndOrderedCollisions(t *testing.T) {
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 23, 45, 3, 0, time.FixedZone("PDT", -7*60*60))

	first, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision first: %v", err)
	}
	if err := os.WriteFile(first, []byte("first"), 0o644); err != nil {
		t.Fatalf("WriteFile(first): %v", err)
	}
	second, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision second: %v", err)
	}
	if err := os.WriteFile(second, []byte("second"), 0o644); err != nil {
		t.Fatalf("WriteFile(second): %v", err)
	}
	third, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision third: %v", err)
	}

	wantParent := filepath.Join(root, "2026", "05", "30")
	wantPaths := []string{
		filepath.Join(wantParent, "session.jsonl"),
		filepath.Join(wantParent, "session-2.jsonl"),
		filepath.Join(wantParent, "session-3.jsonl"),
	}
	assertNamedReservationPaths(t, []string{first, second, third}, wantPaths)
	if got, err := os.ReadFile(first); err != nil || string(got) != "first" {
		t.Fatalf("first artifact = %q, %v; want preserved contents", got, err)
	}
	if got, err := os.ReadFile(second); err != nil || string(got) != "second" {
		t.Fatalf("second artifact = %q, %v; want preserved contents", got, err)
	}
	info, err := os.Stat(first)
	if err != nil {
		t.Fatalf("Stat(first): %v", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("named artifact mode = %#o, want %#o", info.Mode().Perm(), 0o600)
	}
}

func assertNamedReservationPaths(t *testing.T, got, want []string) {
	t.Helper()
	for index, path := range got {
		if path != want[index] {
			t.Errorf("ReserveNamed candidate %d = %q, want %q", index, path, want[index])
		}
	}
}

func TestReserverReserveNamedWithCollisionReturnsNonCollisionFailures(t *testing.T) {
	mkdirErr := errors.New("mkdir failed")
	openErr := errors.New("open failed")
	closeErr := errors.New("close failed")
	tests := []struct {
		name       string
		fileSystem *reserveNamedFailureFileSystem
		wantErr    error
		wantOpens  int
	}{
		{
			name:       "mkdir",
			fileSystem: &reserveNamedFailureFileSystem{mkdirErr: mkdirErr},
			wantErr:    mkdirErr,
		},
		{
			name:       "open",
			fileSystem: &reserveNamedFailureFileSystem{openErr: openErr},
			wantErr:    openErr,
			wantOpens:  1,
		},
		{
			name:       "close",
			fileSystem: &reserveNamedFailureFileSystem{file: closeErrorFile{err: closeErr}},
			wantErr:    closeErr,
			wantOpens:  1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reserver, err := NewReserver(test.fileSystem)
			if err != nil {
				t.Fatalf("NewReserver: %v", err)
			}
			_, err = reserver.ReserveNamedWithCollision(t.TempDir(), time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC), "session", ".jsonl")
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("ReserveNamedWithCollision error = %v, want %v", err, test.wantErr)
			}
			if len(test.fileSystem.opened) != test.wantOpens {
				t.Fatalf("OpenFile calls = %d, want %d", len(test.fileSystem.opened), test.wantOpens)
			}
		})
	}
}

func TestReserverReserveNamedWithCollisionExhaustsAtCollisionBound(t *testing.T) {
	filesystem := &namedReservationExhaustionFileSystem{}
	reserver, err := NewReserver(filesystem)
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}

	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)
	path, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if !errors.Is(err, ErrNamedReservationExhausted) {
		t.Fatalf("ReserveNamedWithCollision error = %v, want ErrNamedReservationExhausted", err)
	}
	if path != "" {
		t.Fatalf("ReserveNamedWithCollision path = %q, want empty path", path)
	}
	if len(filesystem.opened) != maxPathCollisions {
		t.Fatalf("OpenFile calls = %d, want %d", len(filesystem.opened), maxPathCollisions)
	}

	wantFirst := filepath.Join(root, "2026", "05", "29", "session.jsonl")
	wantLast := filepath.Join(root, "2026", "05", "29", "session-1000.jsonl")
	if filesystem.opened[0] != wantFirst {
		t.Fatalf("first exhausted candidate = %q, want %q", filesystem.opened[0], wantFirst)
	}
	if filesystem.opened[len(filesystem.opened)-1] != wantLast {
		t.Fatalf("last exhausted candidate = %q, want %q", filesystem.opened[len(filesystem.opened)-1], wantLast)
	}
	for _, path := range filesystem.opened {
		if filepath.Base(path) == "session-1001.jsonl" {
			t.Fatalf("candidate walk exceeded bound with %q", path)
		}
	}
}

func TestReserverReserveNamedWithCollisionSharesCalendarDirectoryWithLogsAndMetrics(t *testing.T) {
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 23, 45, 3, 0, time.FixedZone("PDT", -7*60*60))
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}

	namedPath, err := reserver.ReserveNamedWithCollision(root, at, "session", ".jsonl")
	if err != nil {
		t.Fatalf("ReserveNamedWithCollision: %v", err)
	}
	logPath := internalartifact.RuntimeArtifactPath(root, at, internalartifact.RuntimeArtifactKindLog, "runtime")
	metricsPath := internalartifact.RuntimeArtifactPath(root, at, internalartifact.RuntimeArtifactKindMetrics, "session-runtime")
	wantDir := filepath.Join(root, "2026", "05", "30")
	for name, path := range map[string]string{
		"log":     logPath,
		"metrics": metricsPath,
		"named":   namedPath,
	} {
		if got := filepath.Dir(path); got != wantDir {
			t.Errorf("%s parent = %q, want %q", name, got, wantDir)
		}
	}
	if filepath.Dir(logPath) != internalartifact.RuntimeLogsDatedDir(root, at) {
		t.Fatalf("log parent = %q, want platform log dated directory", filepath.Dir(logPath))
	}
	if filepath.Dir(metricsPath) != internalartifact.RuntimeMetricsDatedDir(root, at) {
		t.Fatalf("metrics parent = %q, want platform metrics dated directory", filepath.Dir(metricsPath))
	}
}

type namedReservationExhaustionFileSystem struct {
	opened []string
}

func (filesystem *namedReservationExhaustionFileSystem) MkdirAll(string, fs.FileMode) error {
	return nil
}

func (filesystem *namedReservationExhaustionFileSystem) OpenFile(path string, _ int, _ fs.FileMode) (io.WriteCloser, error) {
	filesystem.opened = append(filesystem.opened, path)
	return nil, fs.ErrExist
}

type reserveNamedFailureFileSystem struct {
	mkdirErr error
	openErr  error
	file     io.WriteCloser
	opened   []string
}

func (filesystem *reserveNamedFailureFileSystem) MkdirAll(string, fs.FileMode) error {
	return filesystem.mkdirErr
}

func (filesystem *reserveNamedFailureFileSystem) OpenFile(path string, _ int, _ fs.FileMode) (io.WriteCloser, error) {
	filesystem.opened = append(filesystem.opened, path)
	if filesystem.openErr != nil {
		return nil, filesystem.openErr
	}
	return filesystem.file, nil
}

type closeErrorFile struct{ err error }

func (closeErrorFile) Write(p []byte) (int, error) { return len(p), nil }

func (file closeErrorFile) Close() error { return file.err }

func TestReserverAvoidsConcurrentCollisions(t *testing.T) {
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}
	root := t.TempDir()
	at := time.Date(2026, time.May, 29, 4, 45, 3, 0, time.UTC)

	const count = 16
	paths := make(chan string, count)
	errs := make(chan error, count)
	var group sync.WaitGroup
	for range count {
		group.Add(1)
		go func() {
			defer group.Done()
			path, reserveErr := reserver.Reserve(root, at, "metrics", "session-runtime-fixed")
			if reserveErr != nil {
				errs <- reserveErr
				return
			}
			paths <- path
		}()
	}
	group.Wait()
	close(paths)
	close(errs)
	for reserveErr := range errs {
		t.Fatalf("Reserve concurrent: %v", reserveErr)
	}
	seen := map[string]struct{}{}
	for path := range paths {
		if _, exists := seen[path]; exists {
			t.Fatalf("duplicate reserved path %q", path)
		}
		seen[path] = struct{}{}
	}
	if len(seen) != count {
		t.Fatalf("reserved %d unique paths, want %d", len(seen), count)
	}
}

// TestReserveCreatesTheArtifactOwnerReadableOnly pins the permission a
// reserved runtime artifact is created with.
//
// A reserved path is where a transcript, runtime log, or metrics stream is
// then written, and those carry session content -- the ACP wire transcript in
// particular is documented as "a transcript of the session, not a sanitized
// diagnostic", holding full prompt and response text. Reservation is also the
// only place the mode is decided: the rolling writer that takes the path over
// appends to the file this created and keeps whatever mode it already has, so
// a permissive reservation is not corrected later.
func TestReserveCreatesTheArtifactOwnerReadableOnly(t *testing.T) {
	root := t.TempDir()
	reserver, err := NewReserver(localFileSystem{})
	if err != nil {
		t.Fatalf("NewReserver: %v", err)
	}

	path, err := reserver.Reserve(root, time.Date(2026, 8, 7, 1, 2, 3, 0, time.UTC), "acp-wire", "conn-1")
	if err != nil {
		t.Fatalf("Reserve: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if runtime.GOOS == "windows" {
		// Windows has no POSIX permission bits: os.Chmod only toggles the
		// read-only attribute, so a 0o600 reservation reads back as 0o666.
		// Owner-only protection there comes from the profile directory ACL;
		// assert the mode is not widened beyond the default instead.
		if perm := info.Mode().Perm(); perm&0o600 != 0o600 {
			t.Fatalf("reserved artifact mode = %#o, want owner read/write retained", perm)
		}
		return
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("reserved artifact mode = %#o, want %#o (owner read/write only)", perm, 0o600)
	}
}
