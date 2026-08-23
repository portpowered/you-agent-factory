package metrics

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	internalartifact "github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
	platformrollingfile "github.com/portpowered/infinite-you/pkg/platform/rollingfile"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

const (
	defaultRuntimeMetricsMaxSize = 100
	defaultRuntimeMetricsBackups = 20
	defaultRuntimeMetricsMaxAge  = 30
)

// RuntimeMetricsConfig controls rolling-file policy for runtime metrics.
// Values are in megabytes (MaxSize) and days (MaxAge).
type RuntimeMetricsConfig struct {
	// MaxSize sets the maximum size in megabytes before rotation.
	MaxSize int
	// MaxBackups controls how many backup files are retained.
	MaxBackups int
	// MaxAge controls how many days backup files are retained.
	MaxAge int
	// Compress enables gzip compression for rotated files.
	Compress bool
}

// RuntimeMetricsSink owns the file-backed JSONL metrics emitter and rolling
// writer for one live runtime/session bundle.
type RuntimeMetricsSink struct {
	mu           sync.Mutex
	writer       io.Closer
	encoder      *json.Encoder
	path         string
	rootDir      string
	startTimeUTC time.Time
	config       RuntimeMetricsConfig
	claim        io.Closer
	closed       bool
}

var errRuntimeMetricsSinkClosed = errors.New("runtime metrics sink closed")

// DefaultRuntimeMetricsConfig returns the production rolling-file policy used
// when callers do not set explicit runtime metrics limits.
func DefaultRuntimeMetricsConfig() RuntimeMetricsConfig {
	return RuntimeMetricsConfig{
		MaxSize:    defaultRuntimeMetricsMaxSize,
		MaxBackups: defaultRuntimeMetricsBackups,
		MaxAge:     defaultRuntimeMetricsMaxAge,
	}
}

type RuntimeMetricsOpeningRequest struct {
	SessionID         string
	RuntimeInstanceID string
	FolderPath        string
	FactoryDirectory  string
	RootDirectory     string
	StartTimeUTC      time.Time
	CollisionID       string
	Config            RuntimeMetricsConfig
}

type RuntimeMetricsOpener struct {
	paths        platformartifact.Reserver
	coordination RuntimeMetricsCoordination
}

func NewRuntimeMetricsOpener(
	paths platformartifact.Reserver,
	coordination ...RuntimeMetricsCoordination,
) (*RuntimeMetricsOpener, error) {
	if paths == nil {
		return nil, fmt.Errorf("runtime metrics path reserver is required")
	}
	selectedCoordination, err := selectRuntimeMetricsCoordination(coordination)
	if err != nil {
		return nil, err
	}
	return &RuntimeMetricsOpener{paths: paths, coordination: selectedCoordination}, nil
}

// Open creates a rolling metrics writer from fully selected inputs.
func (opener *RuntimeMetricsOpener) Open(request RuntimeMetricsOpeningRequest) (*RuntimeMetricsSink, error) {
	if opener == nil || opener.paths == nil || opener.coordination == nil {
		return nil, fmt.Errorf("runtime metrics opener is required")
	}
	if request.RuntimeInstanceID == "" {
		return nil, fmt.Errorf("runtime instance ID is required")
	}
	if request.RootDirectory == "" {
		return nil, fmt.Errorf("runtime metrics root is required")
	}
	if request.StartTimeUTC.IsZero() {
		return nil, fmt.Errorf("runtime metrics start time is required")
	}
	if request.CollisionID == "" {
		return nil, fmt.Errorf("runtime metrics collision ID is required")
	}

	rootLock, err := opener.coordination.LockRoot(context.Background(), request.RootDirectory)
	if err != nil {
		return nil, fmt.Errorf("coordinate runtime metrics startup: %w", err)
	}
	startTimeUTC := request.StartTimeUTC.UTC()
	path, err := opener.paths.Reserve(
		request.RootDirectory,
		startTimeUTC,
		string(internalartifact.RuntimeArtifactKindMetrics),
		internalartifact.RuntimeArtifactPathComponents(request.SessionID, request.RuntimeInstanceID, request.CollisionID),
	)
	if err != nil {
		return nil, errors.Join(err, rootLock.Close())
	}
	claim, err := opener.coordination.Claim(path)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("claim runtime metrics path %q: %w", path, err),
			rootLock.Close(),
		)
	}
	if err := rootLock.Close(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("release runtime metrics startup coordination: %w", err),
			claim.Close(),
		)
	}

	metricsConfig := normalizeRuntimeMetricsConfig(request.Config)
	writer := newRuntimeMetricsWriter(&platformrollingfile.Writer{
		Filename:   path,
		MaxSize:    metricsConfig.MaxSize,
		MaxBackups: metricsConfig.MaxBackups,
		MaxAge:     metricsConfig.MaxAge,
		Compress:   metricsConfig.Compress,
	})

	return &RuntimeMetricsSink{
		writer:       writer,
		encoder:      json.NewEncoder(writer),
		path:         path,
		rootDir:      request.RootDirectory,
		startTimeUTC: startTimeUTC,
		config:       metricsConfig,
		claim:        claim,
	}, nil
}

// Path returns the active runtime metrics path.
func (s *RuntimeMetricsSink) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// RootDir returns the runtime metrics root selected by caller configuration or
// the default home-directory policy.
func (s *RuntimeMetricsSink) RootDir() string {
	if s == nil {
		return ""
	}
	return s.rootDir
}

// StartTimeUTC returns the UTC timestamp used to organize the active metrics path.
func (s *RuntimeMetricsSink) StartTimeUTC() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.startTimeUTC
}

// Config returns the normalized rolling-file policy applied to the sink.
func (s *RuntimeMetricsSink) Config() RuntimeMetricsConfig {
	if s == nil {
		return RuntimeMetricsConfig{}
	}
	return s.config
}

// Close releases the runtime metrics writer.
func (s *RuntimeMetricsSink) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var writerErr error
	if s.writer != nil {
		writerErr = s.writer.Close()
	}
	var claimErr error
	if s.claim != nil {
		claimErr = s.claim.Close()
		s.claim = nil
	}
	return errors.Join(writerErr, claimErr)
}

// WriteMetric serializes one owner-projected record as a JSONL entry.
func (s *RuntimeMetricsSink) WriteMetric(ctx context.Context, record any) error {
	if s == nil {
		return nil
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errRuntimeMetricsSinkClosed
	}
	return s.encoder.Encode(record)
}

type runtimeMetricsWriter struct {
	mu     sync.Mutex
	writer *platformrollingfile.Writer
	closed bool
}

func newRuntimeMetricsWriter(writer *platformrollingfile.Writer) *runtimeMetricsWriter {
	return &runtimeMetricsWriter{writer: writer}
}

func (w *runtimeMetricsWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errRuntimeMetricsSinkClosed
	}
	return w.writer.Write(p)
}

func (w *runtimeMetricsWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.writer.Close()
}
func normalizeRuntimeMetricsConfig(config RuntimeMetricsConfig) RuntimeMetricsConfig {
	if config.MaxSize <= 0 {
		config.MaxSize = DefaultRuntimeMetricsConfig().MaxSize
	}
	if config.MaxBackups < 0 {
		config.MaxBackups = 0
	}
	if config.MaxAge < 0 {
		config.MaxAge = 0
	}
	if config.MaxBackups == 0 && config.MaxAge == 0 {
		defaults := DefaultRuntimeMetricsConfig()
		config.MaxBackups = defaults.MaxBackups
		config.MaxAge = defaults.MaxAge
	}
	return config
}

// RuntimeMetricsRoot returns the Metrics-owned default runtime metrics root.
func RuntimeMetricsRoot(home string) string {
	return filepath.Join(home, ".you-agent-factory", "metrics")
}
