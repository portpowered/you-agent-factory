// Package runtimeartifact provides policy-free runtime artifact path mechanics.
package runtimeartifact

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	internalartifact "github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
)

const maxPathCollisions = 1000

// FileSystem is the exact filesystem effect used to reserve artifact paths.
type FileSystem interface {
	MkdirAll(string, fs.FileMode) error
	OpenFile(string, int, fs.FileMode) (io.WriteCloser, error)
}

// Reserver atomically selects a unique path under a dated artifact root.
type Reserver interface {
	Reserve(string, time.Time, string, string) (string, error)
}

type reserver struct{ filesystem FileSystem }

func NewReserver(filesystem FileSystem) (Reserver, error) {
	if filesystem == nil {
		return nil, fmt.Errorf("runtime artifact filesystem is required")
	}
	return &reserver{filesystem: filesystem}, nil
}

func (r *reserver) Reserve(root string, at time.Time, kind, suffix string) (string, error) {
	if root == "" {
		return "", fmt.Errorf("runtime artifact root is required")
	}
	for collision := 0; collision < maxPathCollisions; collision++ {
		path := internalartifact.RuntimeArtifactPathWithCollision(
			root, at, internalartifact.RuntimeArtifactKind(kind), suffix, collision,
		)
		if err := r.filesystem.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("create runtime artifact dir %s: %w", filepath.Dir(path), err)
		}
		file, err := r.filesystem.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err == nil {
			if closeErr := file.Close(); closeErr != nil {
				return "", fmt.Errorf("close reserved runtime artifact %s: %w", path, closeErr)
			}
			return path, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", fmt.Errorf("reserve runtime artifact %s: %w", path, err)
		}
	}
	return "", fmt.Errorf("runtime artifact path collision budget exhausted under %s", root)
}
