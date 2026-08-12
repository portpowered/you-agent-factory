package filesystem

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// DurableAppender is the policy-free host filesystem effect for appending an
// already-redacted runtime artifact record.
type DurableAppender interface {
	AppendDurable(string, []byte) error
}

// AppendDurable appends one record and durably flushes it before returning.
// The caller owns the record format; this adapter owns directory creation,
// file permissions, and the host filesystem effect.
func (Local) AppendDurable(path string, line []byte) error {
	trimmedPath := strings.TrimSpace(path)
	if trimmedPath == "" {
		return fmt.Errorf("runtime artifact path is required")
	}
	local := Local{}
	if err := local.MkdirAll(filepath.Dir(trimmedPath), 0o700); err != nil {
		return fmt.Errorf("create runtime artifact directory: %w", err)
	}
	file, err := local.OpenFile(trimmedPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open runtime artifact: %w", err)
	}
	if written, writeErr := file.Write(line); writeErr != nil {
		_ = file.Close()
		return fmt.Errorf("append runtime artifact: %w", writeErr)
	} else if written != len(line) {
		_ = file.Close()
		return fmt.Errorf("append runtime artifact: %w", io.ErrShortWrite)
	}
	syncer, ok := file.(interface{ Sync() error })
	if !ok {
		_ = file.Close()
		return fmt.Errorf("sync runtime artifact: file does not support durable flush")
	}
	if err := syncer.Sync(); err != nil {
		_ = file.Close()
		return fmt.Errorf("sync runtime artifact: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close runtime artifact: %w", err)
	}
	return nil
}

var _ DurableAppender = Local{}
