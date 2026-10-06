package filesystem

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
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
