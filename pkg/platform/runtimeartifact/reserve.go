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

// ErrNamedReservationExhausted indicates that every bounded candidate for a
// caller-named runtime artifact was already occupied.
var ErrNamedReservationExhausted = errors.New("named runtime artifact reservation exhausted")

// FileSystem is the exact filesystem effect used to reserve artifact paths.
type FileSystem interface {
	MkdirAll(string, fs.FileMode) error
	OpenFile(string, int, fs.FileMode) (io.WriteCloser, error)
}

// Reserver atomically selects a unique path under a dated artifact root.
type Reserver interface {
	Reserve(string, time.Time, string, string) (string, error)
	ReserveNamed(string, time.Time, string, string) (string, error)
}

// RuntimeArtifactPathComponents returns the sanitized, platform-independent
// suffix used by runtime artifact filenames. Callers that build artifact
// selectors can share the writer's exact component rules without acquiring a
// filesystem or artifact-owner policy.
func RuntimeArtifactPathComponents(components ...string) string {
	return internalartifact.RuntimeArtifactPathComponents(components...)
}

type reserver struct{ filesystem FileSystem }

func NewReserver(filesystem FileSystem) (Reserver, error) {
	if filesystem == nil {
		return nil, fmt.Errorf("runtime artifact filesystem is required")
	}
	return &reserver{filesystem: filesystem}, nil
}

func (r *reserver) Reserve(root string, at time.Time, kind, suffix string) (string, error) {
	return r.reserve(root, func(collision int) string {
		return internalartifact.RuntimeArtifactPathWithCollision(
			root, at, internalartifact.RuntimeArtifactKind(kind), suffix, collision,
		)
	}, nil)
}

// ReserveNamed atomically selects a unique caller-named path under a dated
// artifact root. The extension is appended exactly as supplied.
func (r *reserver) ReserveNamed(root string, at time.Time, name, ext string) (string, error) {
	return r.reserve(root, func(collision int) string {
		return internalartifact.RuntimeNamedArtifactPathWithCollision(root, at, name, ext, collision)
	}, ErrNamedReservationExhausted)
}

func (r *reserver) reserve(root string, pathForCollision func(int) string, exhaustion error) (string, error) {
	if root == "" {
		return "", fmt.Errorf("runtime artifact root is required")
	}
	for collision := 0; collision < maxPathCollisions; collision++ {
		path := pathForCollision(collision)
		if err := r.filesystem.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return "", fmt.Errorf("create runtime artifact dir %s: %w", filepath.Dir(path), err)
		}
		// Owner-only. A reserved path becomes a transcript, runtime log, or
		// metrics stream carrying session content -- the ACP wire transcript
		// is documented as holding full prompt and response text -- and none
		// of them is meant to be readable by other users on the host.
		// Reservation is also the only place this is decided: the rolling
		// writer that later takes the path over appends to this file and
		// keeps the mode it already has.
		file, err := r.filesystem.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
	if exhaustion != nil {
		return "", fmt.Errorf("%w under %s", exhaustion, root)
	}
	return "", fmt.Errorf("runtime artifact path collision budget exhausted under %s", root)
}
