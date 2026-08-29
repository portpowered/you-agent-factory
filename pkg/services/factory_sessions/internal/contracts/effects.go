package contracts

import (
	"context"
	"io"
	"io/fs"
)

// Effect-port contracts are owned here so the Sessions service root can publish
// them as type aliases without adding extra root InterfaceType declarations
// (pkg-structure requires exactly one named service interface aside from
// recorded deletion-only debt for the Sessions service root).

type ExecutionOpeningFileSystem interface {
	Getwd() (string, error)
	Stat(string) (fs.FileInfo, error)
}

type DirectoryInspection interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
}

type CursorPersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	Remove(string) error
	Rename(string, string) error
}

type CursorPersistenceTemporaryFile interface {
	io.Writer
	Name() string
	Chmod(fs.FileMode) error
	Sync() error
	Close() error
}

type CursorPersistenceCreateTemporaryFile func(string, string) (CursorPersistenceTemporaryFile, error)

type RuntimePersistenceFileSystem interface {
	MkdirAll(string, fs.FileMode) error
	ReadFile(string) ([]byte, error)
	WriteFile(string, []byte, fs.FileMode) error
}

type InvocationMetricsRecorder interface {
	RecordInvocationMetric(InvocationMetric)
}

// RuntimeMetricsScope is the Factory Sessions-owned identity set used to
// select one live or resumed runtime metrics lineage. The requested ID is the
// public selector; retained IDs are canonical persisted identities only.
type RuntimeMetricsScope struct {
	RequestedFactorySessionID string
	RetainedFactorySessionIDs []string
}

// RuntimeMetricsScopeResolver resolves a public Factory Session selector to
// the exact retained canonical identities that base metrics and Costs must
// query. It is kept in the implementation-facing contracts package so the
// public Sessions root can publish it as an alias without adding another
// named service interface.
type RuntimeMetricsScopeResolver interface {
	ResolveRuntimeMetricsScope(context.Context, string) (RuntimeMetricsScope, error)
}

type ModelPullMetricsRecorder interface {
	RecordModelPullMetric(InvocationMetric)
}

type ModelHostDiagnosticLogger interface {
	Info(string, map[string]string)
	Warn(string, map[string]string)
}

type ModelHostMetricsRecorder interface {
	RecordMetric(string, map[string]string)
}

type InvocationArtifactFileSystem interface {
	Open(string) (io.ReadCloser, error)
	Create(string) (io.WriteCloser, error)
}

type InvocationArtifactExporter interface {
	ExportInvocationArtifact(sourcePath, destinationPath string) error
}
