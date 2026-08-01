// Package filesystem_watchers owns Automations configured-path watching,
// startup preseed, debounce/coalesce policy, cursor persistence, and
// Work-request commanding. Callers outside Automations consume the outer
// Automations service root instead of this parent-private package.
package filesystem_watchers

import automations "github.com/portpowered/infinite-you/pkg/services/automations"

// Config is the root-owned inert configuration for one watcher.
type Config = automations.FilesystemWatcherConfig

// WorkRequestSubmitter is retained as a value alias for nested implementation
// code; it is not a collaborator interface.
type WorkRequestSubmitter = automations.WorkRequestSubmitter

// ObservationIdentity is the stable Automations-owned cursor key for one watched
// input path. Peers treat it as opaque; only filesystem_watchers derives it.
type ObservationIdentity string

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

// Service constructs inert filesystem watchers for Automations composition.
type Service interface {
	NewWatcher(Config) automations.FilesystemWatcher
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
	// NewWatcherWithResume validates resume facts, wires cursor-backed handled
	// identities into config, and returns an inert watcher for explicit preseed/watch.
	NewWatcherWithResume(RestartRequest) (automations.FilesystemWatcher, WatcherFacts, error)
}

// RestartRequest carries inputs for constructing a watcher after Automations restart.
type RestartRequest struct {
	Config        Config
	Identity      WatchIdentity
	Authoritative *WatcherFacts
	Resume        *WatcherFacts
	Persist       CursorFactsPersist
}
