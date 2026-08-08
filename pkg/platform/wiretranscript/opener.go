package wiretranscript

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"gopkg.in/natefinch/lumberjack.v2"
)

const (
	// ArtifactKind names this artifact family in reserved paths.
	ArtifactKind = "acp-wire"

	defaultMaxSizeMB  = 32
	defaultMaxBackups = 4
	defaultMaxAgeDays = 7
)

func unmarshalRecord(line []byte, record *Record) error {
	return json.Unmarshal(line, record)
}

// Root returns the directory ACP wire transcripts are written under for a
// given home directory, mirroring how runtime logs and metrics are rooted.
func Root(home string) string {
	return filepath.Join(home, ".you-agent-factory", ArtifactKind)
}

// Config controls the rolling-file policy for a transcript.
type Config struct {
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
}

// DefaultConfig bounds a transcript to roughly 160MB before compression, which
// keeps a long-lived connection's recording useful without letting it grow
// without limit on a customer's machine.
func DefaultConfig() Config {
	return Config{
		MaxSizeMB:  defaultMaxSizeMB,
		MaxBackups: defaultMaxBackups,
		MaxAgeDays: defaultMaxAgeDays,
		Compress:   true,
	}
}

func normalizeConfig(config Config) Config {
	defaults := DefaultConfig()
	if config.MaxSizeMB <= 0 {
		config.MaxSizeMB = defaults.MaxSizeMB
	}
	if config.MaxBackups <= 0 {
		config.MaxBackups = defaults.MaxBackups
	}
	if config.MaxAgeDays <= 0 {
		config.MaxAgeDays = defaults.MaxAgeDays
	}
	return config
}

// OpeningRequest fully describes one transcript file to create.
type OpeningRequest struct {
	RootDirectory string
	ConnectionID  string
	StartTimeUTC  time.Time
	Config        Config
}

// Opener reserves transcript paths and creates rolling writers.
type Opener struct {
	paths platformartifact.Reserver
	clock Clock
}

// NewOpener returns an Opener over a runtime-artifact path reserver and the
// caller's clock. The clock is required rather than defaulted so this package
// holds no hidden time source of its own.
func NewOpener(paths platformartifact.Reserver, clock Clock) (*Opener, error) {
	if paths == nil {
		return nil, fmt.Errorf("wire transcript path reserver is required")
	}
	if clock == nil {
		return nil, fmt.Errorf("wire transcript clock is required")
	}
	return &Opener{paths: paths, clock: clock}, nil
}

// Sink is one open transcript file.
type Sink struct {
	*Writer
	path string
}

// Path returns the transcript's location, which is what a customer is told to
// look at.
func (s *Sink) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// Open reserves a path and returns a rolling JSONL transcript sink.
func (opener *Opener) Open(request OpeningRequest) (*Sink, error) {
	if opener == nil || opener.paths == nil {
		return nil, fmt.Errorf("wire transcript opener is required")
	}
	if request.RootDirectory == "" {
		return nil, fmt.Errorf("wire transcript root is required")
	}
	if request.ConnectionID == "" {
		return nil, fmt.Errorf("wire transcript connection ID is required")
	}
	start := request.StartTimeUTC
	if start.IsZero() {
		start = opener.clock.Now()
	}
	start = start.UTC()

	path, err := opener.paths.Reserve(request.RootDirectory, start, ArtifactKind, request.ConnectionID)
	if err != nil {
		return nil, err
	}

	config := normalizeConfig(request.Config)
	rolling := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    config.MaxSizeMB,
		MaxBackups: config.MaxBackups,
		MaxAge:     config.MaxAgeDays,
		Compress:   config.Compress,
	}
	return &Sink{Writer: NewWriter(rolling, opener.clock), path: path}, nil
}
