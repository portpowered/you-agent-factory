package factory

import (
	"io"
	"time"

	"go.uber.org/zap"
)

// RuntimeLogStorageConfig is the Factory Runtime-owned rolling log policy.
type RuntimeLogStorageConfig struct {
	MaxSize    int
	MaxBackups int
	MaxAge     int
	Compress   bool
}

// RuntimeLogArtifact describes the file artifact owned by one runtime log sink.
type RuntimeLogArtifact struct {
	Path         string
	RootDir      string
	StartTimeUTC time.Time
	Config       RuntimeLogStorageConfig
}

// RuntimeLogSink is the exact logging capability retained by a hosted runtime.
type RuntimeLogSink interface {
	Logger() *zap.Logger
	Artifact() RuntimeLogArtifact
	io.Closer
}

// RuntimeLogScopeRequest contains the value selections for one private log
// scope. The owner retains path, clock, ID, and filesystem effects; callers
// provide session/runtime identity and the selected destination policy.
type RuntimeLogScopeRequest struct {
	SessionID         string
	RuntimeInstanceID string
	FolderPath        string
	FactoryDirectory  string
	RootDirectory     string
	Policy            RuntimeFileLoggingPolicy
	Config            RuntimeLogStorageConfig
}

// RuntimeLogOwner is the process-scoped observability root for runtime logs.
// Open extends the selected scoped logger with one operation-private sink.
// The logger retains the process backend and this opening's correlation.
// The owner has no lifecycle operation: closing a session closes only the
// returned scope.
type RuntimeLogOwner interface {
	Open(*zap.Logger, RuntimeLogScopeRequest) (RuntimeLogSink, error)
}

type RuntimeArtifactRoots struct {
	Logs    string
	Metrics string
}

// RuntimeArtifactRootResolver maps an explicit invocation home to explicit
// artifact roots at the composition boundary.
type RuntimeArtifactRootResolver func(string) RuntimeArtifactRoots
