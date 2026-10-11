package filesystem

import (
	"io"
	"io/fs"
)

// DurableJournalFileSystem supplies host effects for single-writer journals.
// OpenFile must return a descriptor supporting seek, stat, sync and truncate,
// so the journal owner can roll back a failed append. ReplaceDurable publishes
// flushed private bytes atomically and preserves the old file on failure.
// The consuming owner chooses paths, transaction format and retention policy.
type DurableJournalFileSystem interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, fs.FileMode) error
	OpenFile(string, int, fs.FileMode) (io.WriteCloser, error)
	ReplaceDurable(string, []byte) error
}

var _ DurableJournalFileSystem = Local{}
