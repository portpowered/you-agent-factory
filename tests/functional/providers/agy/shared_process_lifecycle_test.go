package agy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func runAgySharedColdWatchComplete(
	t *testing.T,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	fixture := agySharedProcess(t)
	route := fixture.routes["role-cold-watch-complete"]
	return fixture.runRole(t, route.selector, []string{
		"you", "--json", "run",
		"--named", agyColdWatchFactoryName,
		"--cut-path", route.assetPath,
	})
}

func runAgySharedClipQAPass(
	t *testing.T,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	fixture := agySharedProcess(t)
	route := fixture.routes["role-clipqa-pass"]
	return fixture.runRole(t, route.selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
}

func runAgySharedClipQAReroll(
	t *testing.T,
) (factoryapi.InvocationResponse, []factoryapi.FactoryEvent, *agySharedCommandRoute, string, int) {
	t.Helper()
	fixture := agySharedProcess(t)
	route := fixture.routes["role-clipqa-reroll"]
	return fixture.runRole(t, route.selector, []string{
		"you", "--json", "run",
		"--named", agyClipQAFactoryName,
		"--clip-path", route.assetPath,
		"--shot-specification", agyClipQAShotSpec,
	})
}

func agySharedEnvironment(homeDir string) []string {
	return append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
}

func copyAgyDirectory(t *testing.T, sourceDir, targetDir string) {
	t.Helper()
	err := filepath.WalkDir(sourceDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return os.MkdirAll(targetDir, 0o755)
		}
		targetPath := filepath.Join(targetDir, relative)
		if entry.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy shared AGY fixture %q to %q: %v", sourceDir, targetDir, err)
	}
}

func copyAgySharedAsset(t *testing.T, dir, name string) {
	t.Helper()
	data := readAgyGoldenAsset(t, name)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatalf("write shared AGY asset %s: %v", name, err)
	}
}

func (fixture *agySharedProcessFixture) close() error {
	fixture.closeOnce.Do(func() {
		var closeErrors []error
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if fixture.processBuilds != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared AGY process builds = %d, want 1", fixture.processBuilds))
		}
		if fixture.recordingBuilds > 1 {
			closeErrors = append(closeErrors, fmt.Errorf("isolated AGY recording process builds = %d, want at most 1", fixture.recordingBuilds))
		}
		fixture.hostMu.Lock()
		command := fixture.command
		fixture.hostMu.Unlock()
		if command != nil {
			if err := command.stop(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("stop shared AGY host command: %w", err))
			}
		}
		if fixture.api != nil {
			if command != nil {
				if err := fixture.api.waitClosed(closeCtx); err != nil {
					closeErrors = append(closeErrors, err)
				}
			}
			wantStarts := 0
			if command != nil {
				wantStarts = 1
			}
			if fixture.api.startCount() != wantStarts {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY listener starts = %d, want %d", fixture.api.startCount(), wantStarts))
			}
		}
		if fixture.process != nil {
			fixture.processCloses++
			if err := fixture.process.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, err)
			}
		}
		if fixture.recordingProcess != nil {
			fixture.recordingCloses++
			if err := fixture.recordingProcess.Close(closeCtx); err != nil {
				closeErrors = append(closeErrors, fmt.Errorf("close isolated AGY recording process: %w", err))
			}
		}
		if fixture.runner != nil {
			if got := fixture.runner.activeCallCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY active command calls = %d, want 0", got))
			}
			fixture.sessionMu.Lock()
			activeSessions := len(fixture.activeSessionIDs)
			fixture.sessionMu.Unlock()
			if activeSessions != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY active Factory Sessions = %d, want 0", activeSessions))
			}
			if got := fixture.runner.routeCount(); got != agySharedRouteCount {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY routes before clear = %d, want %d", got, agySharedRouteCount))
			}
			for _, route := range fixture.routes {
				for _, recordingPath := range route.recordingPathsSnapshot() {
					if _, err := os.Stat(recordingPath); !errors.Is(err, os.ErrNotExist) {
						closeErrors = append(closeErrors, fmt.Errorf("shared AGY recording remains at %q: %v", recordingPath, err))
					}
				}
			}
			if err := fixture.runner.clear(); err != nil {
				closeErrors = append(closeErrors, err)
			}
			if got := fixture.runner.routeCount(); got != 0 {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY routes after clear = %d, want 0", got))
			}
		}
		if fixture.processCloses != 1 {
			closeErrors = append(closeErrors, fmt.Errorf("shared AGY process closes = %d, want 1", fixture.processCloses))
		}
		if fixture.recordingCloses != fixture.recordingBuilds {
			closeErrors = append(closeErrors, fmt.Errorf("isolated AGY recording process closes = %d, want %d", fixture.recordingCloses, fixture.recordingBuilds))
		}
		if fixture.rootDir != "" {
			if err := os.RemoveAll(fixture.rootDir); err != nil {
				closeErrors = append(closeErrors, err)
			} else if _, err := os.Stat(fixture.rootDir); !errors.Is(err, os.ErrNotExist) {
				closeErrors = append(closeErrors, fmt.Errorf("shared AGY fixture root remains: %w", err))
			}
		}
		fixture.routes = nil
		fixture.closeErr = errors.Join(closeErrors...)
	})
	return fixture.closeErr
}
