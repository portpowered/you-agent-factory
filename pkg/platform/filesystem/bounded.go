package filesystem

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// ReadSizeLimitError reports a bounded read without retaining file content.
type ReadSizeLimitError struct {
	Limit int64
}

func (e *ReadSizeLimitError) Error() string {
	return fmt.Sprintf("file exceeds read limit of %d bytes", e.Limit)
}

// ReadSizeLimit lets domain owners classify the cause without parsing text.
func (e *ReadSizeLimitError) ReadSizeLimit() int64 { return e.Limit }

// ReadFileBounded consumes at most limit+1 bytes, including when the file grows
// during the read. Exactly limit bytes are permitted; rejected bytes are never
// returned to a decoder. Metadata alone cannot enforce this bound.
func (Local) ReadFileBounded(path string, limit int64) ([]byte, error) {
	if limit < 0 || limit == math.MaxInt64 {
		return nil, errors.New("bounded read requires a nonnegative limit below MaxInt64")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(data)) > limit {
		return nil, &ReadSizeLimitError{Limit: limit}
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return data, nil
}

// RenameNoReplace preserves a regular file under a new name using an atomic
// no-replace hard link, then removes its original name. A collision leaves both
// existing files untouched. A failure removing the source retains both names;
// a crash between these operations also retains the bytes. Filesystems without
// hard-link support fail safely. Permissions are inherited from the same inode.
func (Local) RenameNoReplace(source, destination string) error {
	// Reject symbolic links in the selected path, including parent directories,
	// before creating an archive or removing any name outside that selection.
	for parent := filepath.Clean(source); ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("no-replace move rejects symbolic links")
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("no-replace move requires a regular file")
	}
	if err := os.Link(source, destination); err != nil {
		return err
	}
	return os.Remove(source)
}
