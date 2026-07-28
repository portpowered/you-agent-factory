package automations_test

import (
	"context"
	"io/fs"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

func TestAutomationsServiceConstructsFilesystemWatcherOwnerInertly(t *testing.T) {
	t.Parallel()

	service := automationinternal.NewService(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		"workflow-composition",
		"",
		nil,
		nil,
		nil,
	)
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	watcher := service.NewFilesystemWatcher(automations.FilesystemWatcherConfig{
		Dir: t.TempDir(),
		Submitter: func(context.Context, work.WorkRequest) error {
			return nil
		},
		Files:          localFilesystemInputReader{},
		WalkDirectory:  func(string, fs.WalkDirFunc) error { return nil },
		WorkRequestIDs: func() string { return "test-id" },
	})
	if watcher == nil {
		t.Fatal("NewFilesystemWatcher returned nil")
	}
}

type localFilesystemInputReader struct{}

func (localFilesystemInputReader) ReadDir(string) ([]fs.DirEntry, error) { return nil, nil }
func (localFilesystemInputReader) ReadFile(string) ([]byte, error)       { return nil, nil }
func (localFilesystemInputReader) Stat(string) (fs.FileInfo, error)      { return nil, nil }
