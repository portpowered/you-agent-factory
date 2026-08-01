// Package contracts owns Automations-internal effect shapes that are exposed
// through the public Automations root without becoming additional root service
// authorities.
package contracts

import (
	"context"
	"io/fs"
)

// FilesystemInputReader reads the watched input tree selected by Automations.
type FilesystemInputReader interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// FilesystemWatcher supervises one configured input root. Construction is
// inert; PreseedInputs and Watch perform effects only when explicitly invoked.
type FilesystemWatcher interface {
	PreseedInputs(context.Context) error
	Watch(context.Context) error
}
