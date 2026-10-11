package filesystem

import (
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
)

type replacementFile interface {
	io.WriteCloser
	Sync() error
}

type stagedFile interface {
	replacementFile
	Chmod(fs.FileMode) error
}

// ReplaceDurable publishes flushed bytes using a same-directory atomic rename.
// Unlike RenameReplacing, it never removes the destination to make room. A
// staging or publication failure leaves the existing destination untouched.
// The caller owns the format, retention policy and single-writer profile.
func (local Local) ReplaceDurable(path string, data []byte) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("replacement path is required")
	}
	temporary, err := local.CreateTemp(filepath.Dir(path), ".durable-replacement-*")
	if err != nil {
		return err
	}
	staged := temporary.Name()
	defer func() { _ = local.Remove(staged) }()
	file, ok := temporary.(stagedFile)
	if !ok {
		_ = temporary.Close()
		return errors.New("replacement requires a private, flushable file")
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if err := local.flushReplacement(file, data); err != nil {
		return err
	}
	return local.publishReplacement(staged, path)
}

// A staging failure must close its descriptor and refuse publication. Close
// failures before the rename also refuse publication, even after a good flush.
func (Local) flushReplacement(file replacementFile, data []byte) error {
	if written, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	} else if written != len(data) {
		_ = file.Close()
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}
