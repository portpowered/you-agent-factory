package runtimepersist

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"strings"
	"time"
)

const quarantineCollisionAttempts = 16

var quarantineIdentityPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{1,128}$`)

// CurrentBoardQuarantineStore preserves the selected default snapshot and its
// reference before startup may publish a fresh board. The caller supplies its
// injected clock and unique identity; this store never reads damaged content.
type CurrentBoardQuarantineStore interface {
	QuarantineCurrentBoard(context.Context, time.Time, string) (string, error)
}

// QuarantineCurrentBoard moves evidence beside its original location without
// overwriting archives. The snapshot is mandatory; an absent reference is fine.
// A partial move, cancellation, or reference failure prevents success, retaining
// all evidence under either its original name or a quarantine name. There is no
// rollback or deletion that could discard an archive after preservation.
func (s DirectoryStore) QuarantineCurrentBoard(ctx context.Context, at time.Time, identity string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.TrimSpace(s.Dir) == "" || s.files == nil {
		return "", errors.New("durable session quarantine persistence is unavailable")
	}
	if !quarantineIdentityPattern.MatchString(identity) {
		return "", errors.New("durable session quarantine identity is invalid")
	}
	utc := at.UTC()
	suffix := fmt.Sprintf(".unreadable.%s%09dZ.%s", utc.Format("20060102T150405"), utc.Nanosecond(), identity)
	archive, err := s.quarantineFile(ctx, s.SnapshotPath("~default"), suffix)
	if err != nil {
		return "", err
	}
	if _, err := s.quarantineFile(ctx, s.currentBoardPath(), suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return archive, nil
}

func (s DirectoryStore) quarantineFile(ctx context.Context, source, suffix string) (string, error) {
	for attempt := 0; attempt < quarantineCollisionAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		destination := source + suffix
		if attempt > 0 {
			destination += fmt.Sprintf("-%d", attempt)
		}
		err := s.files.RenameNoReplace(source, destination)
		if err == nil {
			return destination, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return "", &persistenceError{operation: "preserve unreadable current board evidence", cause: err}
		}
	}
	return "", &persistenceError{operation: "reserve unreadable current board archive", cause: fs.ErrExist}
}
