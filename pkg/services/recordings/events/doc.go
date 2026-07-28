// Package events is a transitional compile shim that re-exports the canonical
// ledger event history implementation from
// pkg/services/recordings/internal/services/canonical_ledger/events. Peers
// should construct through recordings/wire; baseline deletion of this path is
// owned by DEL-REC.
package events
