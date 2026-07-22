package logging

import (
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"go.uber.org/zap"
)

// BuildRuntimeLogger retains the former convenience only inside Platform
// adapter tests; production callers must supply every opening dependency.
func BuildRuntimeLogger(base *zap.Logger, runtimeID, root string, config RuntimeLogConfig) (*RuntimeLogSink, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = RuntimeLogsRoot(home)
		legacy := legacyRuntimeLogDir(home)
		if _, err := os.Stat(legacy); err == nil {
			if _, canonicalErr := os.Stat(root); os.IsNotExist(canonicalErr) {
				if err := os.MkdirAll(filepath.Dir(root), 0o755); err != nil {
					return nil, err
				}
				if err := os.Rename(legacy, root); err != nil {
					return nil, err
				}
			}
		}
	}
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		return nil, err
	}
	opener, err := NewRuntimeLogOpener(paths)
	if err != nil {
		return nil, err
	}
	return opener.Open(RuntimeLogOpeningRequest{
		BaseLogger: base, RuntimeInstanceID: runtimeID, RootDirectory: root,
		StartTimeUTC: time.Now().UTC(), CollisionID: uuid.NewString(), Config: config,
	})
}
