package filesystem

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
)

type replacementFile interface {
	io.WriteCloser
	Sync() error
}

// ReplaceDurable publishes flushed bytes using a same-directory atomic rename.
// Unlike RenameReplacing, it never removes the destination to make room. A
// staging or publication failure leaves the existing destination untouched.
// The caller owns the format, retention policy and single-writer profile.
func (local Local) ReplaceDurable(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("replacement path is required")
	}
	// Local.CreateTemp creates the staging file with private 0600 permissions.
	// Keep the effect at the existing filesystem boundary; no chmod is needed.
	temporary, err := local.CreateTemp(filepath.Dir(path), ".durable-replacement-*")
	if err != nil {
		return err
	}
	staged := temporary.Name()
	defer func() { _ = local.Remove(staged) }()
	file, ok := temporary.(replacementFile)
	if !ok {
		_ = temporary.Close()
		return errors.New("replacement requires a private, flushable file")
	}
	if err := local.flushReplacement(file, data); err != nil {
		return err
	}
	return local.publishReplacement(staged, path)
}

// A staging failure must close its descriptor and refuse publication. Close
// failures before the rename also refuse publication, even after a good flush.
func (Local) flushReplacement(file replacementFile, data []byte) error {
	written, err := file.Write(data)
	if err == nil && written != len(data) {
		err = io.ErrShortWrite
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
