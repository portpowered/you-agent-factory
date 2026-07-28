// Package replay is a transitional compile shim that re-exports the private
// replay implementation from
// pkg/services/recordings/internal/services/replay/replay. Peers should
// construct through recordings/wire; baseline deletion of this path is owned
// by DEL-REC.
package replay
