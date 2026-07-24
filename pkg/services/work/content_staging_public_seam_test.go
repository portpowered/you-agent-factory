package work_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
	workwire "github.com/portpowered/infinite-you/pkg/services/work/wire"
)

type publicSeamFileSystem struct {
	root    string
	written map[string][]byte
}

func (f *publicSeamFileSystem) MkdirTemp(_ string, pattern string) (string, error) {
	return os.MkdirTemp(f.root, pattern)
}

func (f *publicSeamFileSystem) WriteFile(path string, data []byte, mode fs.FileMode) error {
	if f.written == nil {
		f.written = make(map[string][]byte)
	}
	f.written[path] = append([]byte(nil), data...)
	return os.WriteFile(path, data, mode)
}

func (f *publicSeamFileSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func (f *publicSeamFileSystem) RemoveAll(path string) error { return os.RemoveAll(path) }

type publicSeamRandom struct{ value byte }

func (r publicSeamRandom) Read(buffer []byte) (int, error) {
	for i := range buffer {
		buffer[i] = r.value
	}
	return len(buffer), nil
}

type publicSeamClock struct{ now time.Time }

func (c publicSeamClock) Now() time.Time { return c.now }

func TestPublicWorkRootStagingSeamIssuesSecureStagedReference(t *testing.T) {
	filesystem := &publicSeamFileSystem{root: t.TempDir()}
	staging, err := workwire.NewContentStagingService(
		filesystem,
		publicSeamRandom{value: 0x11},
		publicSeamClock{now: time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("work wire.NewContentStagingService: %v", err)
	}

	staged, err := staging.StageContent(context.Background(), work.StageContentRequest{
		ItemType:  "image",
		FileName:  "../escape/ui.png",
		MediaType: "image/png",
		Content:   []byte("png-bytes"),
	})
	if err != nil {
		t.Fatalf("StageContent: %v", err)
	}
	if staged.StagedFileRef == "" || staged.URL == "" {
		t.Fatalf("stage result = %#v, want opaque staged reference and URL", staged)
	}
	resolved, err := staging.ResolveContent(context.Background(), staged.StagedFileRef)
	if err != nil {
		t.Fatalf("ResolveContent: %v", err)
	}
	if filepath.Base(resolved.Path) != "ui.png" {
		t.Fatalf("resolved path = %q, want detached base filename", resolved.Path)
	}
	if got := string(filesystem.written[resolved.Path]); got != "png-bytes" {
		t.Fatalf("written content = %q, want png-bytes", got)
	}
}
