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

// WatchIdentity identifies one configured filesystem watch root within an
// Automation.
type WatchIdentity struct {
	AutomationID string
	WatchRoot    string
}

// Cursor is an opaque durable watcher position. Peers may persist and return
// the value but must not interpret its contents.
type Cursor string

// WatcherFacts are durable cursor facts persisted across Automations restart.
type WatcherFacts struct {
	Identity   WatchIdentity
	Cursor     Cursor
	Checkpoint string
}

// CursorProjection returns the opaque cursor/checkpoint pair peers consume
// through Automations cursor/status contracts.
func (f WatcherFacts) CursorProjection() (Cursor, string) {
	return f.Cursor, f.Checkpoint
}

// CursorFactsPersist commits updated watcher facts after a successful handled
// observation. Implementations are Automations-owned; peers inject durable
// stores without importing parent-private persistence types.
type CursorFactsPersist func(WatcherFacts) error

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
	// ResumeWatcherFacts validates detached committed facts on Automations restart.
	// When authoritative facts already exist, resume must match them.
	ResumeWatcherFacts(
		identity WatchIdentity,
		authoritative *WatcherFacts,
		resume *WatcherFacts,
	) (WatcherFacts, error)
	// ValidateExpectedCursor rejects stale optimistic concurrency tokens without
	// mutating authoritative facts.
	ValidateExpectedCursor(authoritative WatcherFacts, expected Cursor) error
	// WatcherFactsFromCursor rebuilds detached facts from opaque cursor/checkpoint
	// values peers retrieved through Automations contracts.
	WatcherFactsFromCursor(identity WatchIdentity, cursor Cursor, checkpoint string) (WatcherFacts, error)
	// NewHandledIdentities constructs a cursor-backed identity store from resumed
	// facts. Record commits updated facts through persist; persist failures do not
	// report successful recovery.
	NewHandledIdentities(facts WatcherFacts, persist CursorFactsPersist) (HandledIdentities, error)
	// NewWatcherWithResume validates resume facts, wires cursor-backed handled
	// identities into config, and returns an inert watcher for explicit preseed/watch.
	NewWatcherWithResume(RestartRequest) (Watcher, WatcherFacts, error)
}

// RestartRequest carries inputs for constructing a watcher after Automations restart.
type RestartRequest struct {
	Config        Config
	Identity      WatchIdentity
	Authoritative *WatcherFacts
	Resume        *WatcherFacts
	Persist       CursorFactsPersist
}
