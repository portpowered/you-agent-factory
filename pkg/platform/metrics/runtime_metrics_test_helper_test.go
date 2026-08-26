package metrics

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

func assertRuntimeArtifactRootLacksCalendarDirectories(t *testing.T, rootDir string) {
	t.Helper()
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", rootDir, err)
	}
	for _, entry := range entries {
		if entry.IsDir() && regexp.MustCompile(`^\d{4}$`).MatchString(entry.Name()) {
			t.Fatalf("root %q unexpectedly contains calendar directory %q", rootDir, entry.Name())
		}
	}
}

func assertRuntimeArtifactDatedDirPresent(t *testing.T, datedDir string) {
	t.Helper()
	info, err := os.Stat(datedDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("dated dir %q is unavailable: %v", datedDir, err)
	}
}

func assertPathUsesPlatformSeparators(t *testing.T, path string) {
	t.Helper()
	altSep := '/'
	if os.PathSeparator == '/' {
		altSep = '\\'
	}
	if strings.ContainsRune(path, altSep) {
		t.Fatalf("path %q contains non-platform separator %q", path, altSep)
	}
}

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
