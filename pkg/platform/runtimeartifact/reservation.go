package runtimeartifact

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	internalartifact "github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
)

// Reservation is an immutable identity scoped to its originating reserver.
// File metadata and rollback authority remain private to that owner.
type Reservation struct{ id uint64 }

type reservedFile struct {
	path     string
	identity fs.FileInfo
}

// OwnedNamedReserver is the optional transactional form of named reservation.
// Existing path-only callers retain the original Reserver contract.
type OwnedNamedReserver interface {
	ReserveNamedOwned(string, time.Time, string, string) (string, Reservation, error)
	ReleaseReservation(Reservation) error
	CommitReservation(Reservation)
}

// IsZero reports whether this value carries rollback authority.
func (reservation Reservation) IsZero() bool { return reservation.id == 0 }

// ReserveNamedOwned retains the identity from the exclusive creation handle.
func (r *reserver) ReserveNamedOwned(root string, at time.Time, name, ext string) (string, Reservation, error) {
	if root == "" {
		return "", Reservation{}, fmt.Errorf("runtime artifact root is required")
	}
	if _, ok := r.filesystem.(interface {
		RemoveReserved(string, fs.FileInfo) error
	}); !ok {
		return "", Reservation{}, fmt.Errorf("reservation removal capability is required")
	}
	path := internalartifact.RuntimeNamedArtifactPathWithCollision(root, at, name, ext, 0)
	if err := r.filesystem.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", Reservation{}, err
	}
	file, err := r.filesystem.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return "", Reservation{}, fmt.Errorf("%w under %s", ErrNamedReservationExists, root)
	}
	if err != nil {
		return "", Reservation{}, err
	}
	stat, ok := file.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		return "", Reservation{}, errors.Join(fmt.Errorf("reservation identity capability is required"), file.Close())
	}
	identity, statErr := stat.Stat()
	if statErr == nil {
		// Windows FileInfo may load its file ID lazily; capture it while the
		// original exclusive creation handle and pathname still exist.
		if !os.SameFile(identity, identity) {
			statErr = fmt.Errorf("reservation identity is unavailable")
		}
	}
	var reservation Reservation
	if identity != nil {
		r.reservationMu.Lock()
		r.nextReservation++
		reservation = Reservation{id: r.nextReservation}
		if r.reservations == nil {
			r.reservations = make(map[Reservation]reservedFile)
		}
		r.reservations[reservation] = reservedFile{path: path, identity: identity}
		r.reservationMu.Unlock()
	}
	if err := errors.Join(statErr, file.Close()); err != nil {
		return path, reservation, err
	}
	return path, reservation, nil
}

// ReleaseReservation removes only the unchanged, originally created file.
// Missing, replaced or written files have no rollback authority left.
func (r *reserver) ReleaseReservation(reservation Reservation) error {
	if reservation.IsZero() {
		return nil
	}
	r.reservationMu.Lock()
	defer r.reservationMu.Unlock()
	file, exists := r.reservations[reservation]
	if !exists {
		return nil
	}
	filesystem, ok := r.filesystem.(interface {
		RemoveReserved(string, fs.FileInfo) error
	})
	if !ok {
		return fmt.Errorf("reservation removal capability is required")
	}
	if err := filesystem.RemoveReserved(file.path, file.identity); err != nil {
		return err
	}
	delete(r.reservations, reservation)
	return nil
}

// CommitReservation discards rollback metadata after successful activation.
// It leaves the reserved file in place for its recording writer.
func (r *reserver) CommitReservation(reservation Reservation) {
	r.reservationMu.Lock()
	defer r.reservationMu.Unlock()
	delete(r.reservations, reservation)
}
