package runtimeartifact

import (
	"errors"
	"io/fs"
	"os"
	"testing"
	"time"
)

type reservationFileSystem struct {
	localFileSystem
	removeErr error
	removed   []string
}

func (filesystem *reservationFileSystem) RemoveReserved(path string, identity fs.FileInfo) error {
	if filesystem.removeErr != nil {
		return filesystem.removeErr
	}
	current, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !os.SameFile(current, identity) || current.Size() != 0 {
		return nil
	}
	filesystem.removed = append(filesystem.removed, path)
	return os.Remove(path)
}

func TestOwnedReservationRetryAndForeignCollision(t *testing.T) {
	t.Parallel()
	filesystem := &reservationFileSystem{}
	paths, err := NewReserver(filesystem)
	if err != nil {
		t.Fatal(err)
	}
	reserver := paths.(OwnedNamedReserver)
	root := t.TempDir()
	at := time.Date(2041, 2, 3, 0, 0, 0, 0, time.UTC)
	path, reservation, err := reserver.ReserveNamedOwned(root, at, "peer", ".json")
	if err != nil || reservation.IsZero() {
		t.Fatalf("owned reserve: %s %+v %v", path, reservation, err)
	}
	if _, other, err := reserver.ReserveNamedOwned(root, at, "peer", ".json"); !errors.Is(err, ErrNamedReservationExists) || !other.IsZero() {
		t.Fatalf("collision acquired authority: %+v %v", other, err)
	}
	assertReservationReleaseRetry(t, reserver, filesystem, path, reservation)
	retryPath, retry, err := reserver.ReserveNamedOwned(root, at, "peer", ".json")
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if retry == reservation {
		t.Fatal("retry reused rollback identity")
	}
	if err := reserver.ReleaseReservation(reservation); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(retryPath); err != nil {
		t.Fatalf("stale release removed retry: %v", err)
	}
	reserver.CommitReservation(retry)
	if err := reserver.ReleaseReservation(retry); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(retryPath); err != nil {
		t.Fatalf("committed placeholder removed: %v", err)
	}
	if err := reserver.ReleaseReservation(Reservation{}); err != nil {
		t.Fatal(err)
	}
}

func assertReservationReleaseRetry(t *testing.T, reserver OwnedNamedReserver, filesystem *reservationFileSystem, path string, reservation Reservation) {
	t.Helper()
	cause := errors.New("removal unavailable")
	filesystem.removeErr = cause
	if err := reserver.ReleaseReservation(reservation); !errors.Is(err, cause) {
		t.Fatalf("release cause lost: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("failed release lost placeholder: %v", err)
	}
	filesystem.removeErr = nil
	if err := reserver.ReleaseReservation(reservation); err != nil {
		t.Fatal(err)
	}
	if err := reserver.ReleaseReservation(reservation); err != nil {
		t.Fatal(err)
	}
	if len(filesystem.removed) != 1 {
		t.Fatalf("release count=%d", len(filesystem.removed))
	}

}
