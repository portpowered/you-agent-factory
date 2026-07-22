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

// RuntimeLogSinkFactory opens one explicitly configured runtime log sink.
// Wire owns the Platform adapter and all ambient clock, ID, home, path, and
// filesystem dependencies used by that adapter.
type RuntimeLogSinkFactory func(
	base *zap.Logger,
	runtimeInstanceID string,
	rootDir string,
	config RuntimeLogStorageConfig,
) (RuntimeLogSink, error)

type RuntimeArtifactRoots struct {
	Logs    string
	Metrics string
}

// RuntimeArtifactRootResolver maps an explicit invocation home to explicit
// artifact roots at the composition boundary.
type RuntimeArtifactRootResolver func(string) RuntimeArtifactRoots
