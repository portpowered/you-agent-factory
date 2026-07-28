// Package artifacts is a transitional compile shim that re-exports the private
// artifacts implementation from
// pkg/services/recordings/internal/services/artifacts_export/artifacts. Peers
// should construct through recordings/wire; baseline deletion of this path is
// owned by DEL-REC.
package artifacts
