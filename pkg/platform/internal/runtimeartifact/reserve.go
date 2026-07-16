package runtimeartifact

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
)

const maxRuntimeArtifactPathCollisions = 1000

var errRuntimeArtifactPathCollisionBudget = errors.New("runtime artifact path collision budget exhausted")

// ReserveAvailablePath selects a non-colliding runtime artifact
// path under the shared dated directory. It reserves the chosen path with an
// exclusive create so concurrent sink creation does not truncate an existing
// file.
func ReserveAvailablePath(
	rootDir string,
	at time.Time,
	kind defaultpaths.RuntimeArtifactKind,
	suffix string,
) (string, error) {
	for collisionIndex := 0; collisionIndex < maxRuntimeArtifactPathCollisions; collisionIndex++ {
		path := defaultpaths.RuntimeArtifactPathWithCollision(rootDir, at, kind, suffix, collisionIndex)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("create runtime artifact dir %s: %w", filepath.Dir(path), err)
		}

		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if closeErr := file.Close(); closeErr != nil {
				return "", fmt.Errorf("close reserved runtime artifact %s: %w", path, closeErr)
			}
			return path, nil
		}
		if !os.IsExist(err) {
			return "", fmt.Errorf("reserve runtime artifact %s: %w", path, err)
		}
	}

	return "", fmt.Errorf(
		"reserve runtime artifact under %s at %s kind %s suffix %q: %w",
		rootDir,
		at.UTC().Format(time.RFC3339Nano),
		kind,
		suffix,
		errRuntimeArtifactPathCollisionBudget,
	)
}
