// Package filesystem_watchers owns Automations configured-path watching,
// startup preseed, debounce/coalesce policy, cursor persistence, and
// Work-request commanding. Callers outside Automations consume the outer
// Automations service root instead of this parent-private package.
package filesystem_watchers

import (
	"context"
	"io/fs"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"go.uber.org/zap"
)

// WorkRequestSubmitter admits one Work Request materialized by a filesystem
// watcher through the accepted Work boundary.
type WorkRequestSubmitter func(context.Context, work.WorkRequest) error

// InputFileSystem reads the watched input tree after Automations has selected
// paths and collaborators.
type InputFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// DirectoryWalker traverses the watched input tree. Wire supplies the
// production implementation; owner tests may inject a deterministic traversal.
type DirectoryWalker func(string, fs.WalkDirFunc) error

// ObservationIdentity is the stable Automations-owned cursor key for one watched
// input path. Peers treat it as opaque; only filesystem_watchers derives it.
type ObservationIdentity string

// HandledIdentities records watcher cursor facts for paths whose Work commands
// already succeeded. Implementations are Automations-owned; peers inject
// durable stores without importing parent-private persistence types.
type HandledIdentities interface {
	Contains(ObservationIdentity) bool
	Record(ObservationIdentity) error
}

// Config carries the inert construction inputs for one filesystem watcher.
type Config struct {
	Dir               string
	Logger            *zap.Logger
	KnownWorkTypes    []string
	ValidStatesByType map[string]map[string]bool
	Files             InputFileSystem
	WalkDirectory     DirectoryWalker
	WorkRequestIDs    work.RequestIDGenerator
	Submitter         WorkRequestSubmitter
	HandledIdentities HandledIdentities
	Clock             clockwork.Clock
	DebounceWindow    time.Duration
}

// Watcher supervises one configured input root. Construction is inert; Watch and
// PreseedInputs perform effects only when explicitly invoked.
type Watcher interface {
	PreseedInputs(context.Context) error
	Watch(context.Context) error
}

// Service constructs inert filesystem watchers for Automations composition.
type Service interface {
	NewWatcher(Config) Watcher
}
