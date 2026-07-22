package operatorsettings

import (
	"io"
	"io/fs"
)

// TemporaryFile is the exact atomic-write handle used by Operator Settings.
type TemporaryFile interface {
	io.Writer
	Name() string
	Sync() error
	Close() error
}

// FileSystem is the exact storage boundary for the operator configuration.
type FileSystem interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, fs.FileMode) error
	Remove(string) error
	Chmod(string, fs.FileMode) error
	Rename(string, string) error
}

// CreateTemporaryFile reserves the atomic-write file selected by Wire.
type CreateTemporaryFile func(string, string) (TemporaryFile, error)

// IDGenerator supplies the opaque component of a local backend scope ID.
type IDGenerator func() string

// ConfigDecoder maps serialized global configuration into domain values.
// Wire supplies the OpenAPI-generated contract decoder at the transport boundary.
type ConfigDecoder func([]byte) (Config, error)

// ConfigEncoder maps domain values into the serialized generated global
// configuration contract. Wire supplies the transport-boundary implementation.
type ConfigEncoder func(Config) ([]byte, error)

// ConfigLoader loads the operator-owned configuration from an explicit path.
type ConfigLoader func(string) (Config, error)

// BackendScopeEnsurer resolves and persists the local backend identity.
type BackendScopeEnsurer func(string) (ResolvedBackendScope, error)
