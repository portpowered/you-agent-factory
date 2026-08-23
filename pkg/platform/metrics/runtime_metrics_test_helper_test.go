package metrics

import (
	"os"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// BuildRuntimeMetricsSink retains the former convenience only in adapter tests.
func BuildRuntimeMetricsSink(
	sessionID, runtimeID, folderPath, factoryDir, root string,
	config RuntimeMetricsConfig,
) (*RuntimeMetricsSink, error) {
	if root == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		root = RuntimeMetricsRoot(home)
	}
	paths, err := platformartifact.NewReserver(platformfilesystem.Local{})
	if err != nil {
		return nil, err
	}
	coordination, err := NewRuntimeMetricsCoordination()
	if err != nil {
		return nil, err
	}
	retention, err := NewRuntimeMetricsRetention(platformfilesystem.Local{}, time.Now, coordination)
	if err != nil {
		return nil, err
	}
	scheduler, err := NewRuntimeMetricsRetentionScheduler(retention, nil, nil)
	if err != nil {
		return nil, err
	}
	opener, err := NewRuntimeMetricsOpener(paths, scheduler, coordination)
	if err != nil {
		return nil, err
	}
	return opener.Open(RuntimeMetricsOpeningRequest{
		SessionID: sessionID, RuntimeInstanceID: runtimeID,
		FolderPath: folderPath, FactoryDirectory: factoryDir,
		RootDirectory: root, StartTimeUTC: time.Now().UTC(), CollisionID: uuid.NewString(),
		Config: config,
	})
}
