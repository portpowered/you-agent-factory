package filesystem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

type revisionProbe struct {
	*os.File
	invalidFD         bool
	info              os.FileInfo
	statErr, closeErr error
	stats, closes     int
}

func (file *revisionProbe) Fd() uintptr {
	if file.invalidFD {
		return ^uintptr(0)
	}
	return file.File.Fd()
}

func (file *revisionProbe) Stat() (os.FileInfo, error) {
	file.stats++
	if file.statErr != nil {
		return nil, file.statErr
	}
	if file.info != nil {
		return file.info, nil
	}
	return file.File.Stat()
}

func (file *revisionProbe) Close() error {
	file.closes++
	if file.File != nil {
		if err := file.File.Close(); err != nil {
			return err
		}
	}
	return file.closeErr
}

type revisionMetadata struct {
	os.FileInfo
	sys  any
	mode os.FileMode
}

func (info revisionMetadata) Sys() any          { return info.sys }
func (info revisionMetadata) Mode() os.FileMode { return info.mode }

func TestRacePathsRevisionLinux(t *testing.T) {
	t.Parallel()
	t.Run("ReliableRegularFileRevision", racePathsReliableRevision)
	t.Run("OpenFailure", func(t *testing.T) {
		t.Parallel()
		want := errors.New("open denied")
		reader := RevisionReader{OpenFile: func(path string) (RevisionFile, error) {
			if path != "selected" {
				t.Errorf("opened %q", path)
			}
			return nil, want
		}}
		if value, err := reader.ReadRevision("selected"); value != "" || !errors.Is(err, want) {
			t.Fatalf("open = %q, %v", value, err)
		}
	})
	t.Run("InvalidDescriptorFallback", func(t *testing.T) {
		t.Parallel()
		racePathsRevisionProbe(t, "invalid", nil, nil)
	})
	t.Run("UnsupportedFilesystemFallback", func(t *testing.T) {
		t.Parallel()
		racePathsRevisionProbe(t, "proc", nil, nil)
	})
	t.Run("StatFailure", func(t *testing.T) {
		t.Parallel()
		racePathsRevisionProbe(t, "regular", errors.New("stat denied"), nil)
	})
	t.Run("UnsuitableMetadataFallback", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"no syscall metadata", "nonregular"} {
			t.Run(mode, func(t *testing.T) { t.Parallel(); racePathsRevisionProbe(t, mode, nil, nil) })
		}
	})
	t.Run("CloseFailureAndErrorPrecedence", func(t *testing.T) {
		t.Parallel()
		for _, mode := range []string{"regular", "invalid", "proc", "stat error"} {
			t.Run(mode, func(t *testing.T) {
				t.Parallel()
				var statErr error
				if mode == "stat error" {
					statErr = errors.New("earlier stat error")
				}
				racePathsRevisionProbe(t, mode, statErr, errors.New("close denied"))
			})
		}
	})
}

func racePathsRevisionProbe(t *testing.T, mode string, statErr, closeErr error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte("owned"), 0o600); err != nil {
		t.Fatal(err)
	}
	if mode == "proc" {
		path = "/proc/version"
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	probe := &revisionProbe{File: file, invalidFD: mode == "invalid", statErr: statErr, closeErr: closeErr}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close() // Preserve the Stat failure while releasing the owned descriptor.
		t.Fatal(err)
	}
	if mode == "no syscall metadata" {
		probe.info = revisionMetadata{FileInfo: info, mode: info.Mode()}
	}
	if mode == "nonregular" {
		probe.info = revisionMetadata{FileInfo: info, sys: info.Sys(), mode: os.ModeDir}
	}
	reader := RevisionReader{OpenFile: func(selected string) (RevisionFile, error) {
		if selected != path {
			t.Errorf("opened %q, want %q", selected, path)
		}
		return probe, nil
	}}
	value, err := reader.ReadRevision(path)
	racePathsAssertRevisionProbe(t, probe, mode, info, value, err)
}

func racePathsAssertRevisionProbe(t *testing.T, probe *revisionProbe, mode string, info os.FileInfo, value string, err error) {
	t.Helper()
	wantErr := probe.statErr
	if wantErr == nil {
		wantErr = probe.closeErr
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("%s: error=%v, want %v", mode, err, wantErr)
	}
	wantStats := 1
	if mode == "invalid" || mode == "proc" {
		wantStats = 0
	}
	if probe.closes != 1 || probe.stats != wantStats {
		t.Fatalf("%s: Stat/Close=%d/%d, want %d/1 (requires allowlisted local filesystem)", mode, probe.stats, probe.closes, wantStats)
	}
	if wantErr != nil || mode != "regular" {
		if value != "" {
			t.Fatalf("unsafe revision %q for %s", value, mode)
		}
	} else if value != racePathsMetadataRevision(info) {
		t.Fatalf("revision=%q, want exact metadata %q", value, racePathsMetadataRevision(info))
	}
}

func racePathsMetadataRevision(info os.FileInfo) string {
	stat := info.Sys().(*syscall.Stat_t)
	return fmt.Sprintf("%x:%x:%x:%x:%x:%x:%x", stat.Dev, stat.Ino, stat.Size, stat.Mtim.Sec, stat.Mtim.Nsec, stat.Ctim.Sec, stat.Ctim.Nsec)
}

func racePathsReliableRevision(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "source")
	// A fixed old mtime makes the first write-restoration causally observable
	// even when both writes occur inside one filesystem clock tick.
	old := time.Unix(1234567890, 0)
	racePathsWriteRevisionSource(t, path, "first", old)
	opens, closes := 0, 0
	reader := RevisionReader{OpenFile: func(selected string) (RevisionFile, error) {
		file, err := os.Open(selected)
		if err != nil {
			return nil, err
		}
		opens++
		return &revisionCountedFile{File: file, closes: &closes}, nil
	}}
	read := func() string {
		t.Helper()
		value, err := reader.ReadRevision(path)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if value == "" || value != racePathsMetadataRevision(info) {
			t.Fatalf("revision=%q; requires reliable local Linux filesystem", value)
		}
		if opens != closes {
			t.Fatal("opened descriptor not closed exactly once")
		}
		return value
	}
	first := read()
	if read() != first {
		t.Fatal("read changed revision")
	}
	racePathsWriteRevisionSource(t, path, "other", old)
	second := read()
	if second == first {
		t.Fatal("same-size update with restored mtime retained revision")
	}
	if err := os.Rename(path, path+".old"); err != nil {
		t.Fatal(err)
	}
	racePathsWriteRevisionSource(t, path, "other", old)
	if read() == second {
		t.Fatal("retained old inode failed to distinguish replacement")
	}
}

func racePathsWriteRevisionSource(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

type revisionCountedFile struct {
	*os.File
	closes *int
}

func (file *revisionCountedFile) Close() error { *file.closes++; return file.File.Close() }
