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
	mu             sync.Mutex
	writer         io.Closer
	encoder        *json.Encoder
	path           string
	rootDir        string
	startTimeUTC   time.Time
	config         RuntimeMetricsConfig
	claim          io.Closer
	retentionLease io.Closer
	closed         bool
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
	paths              platformartifact.Reserver
	coordination       RuntimeMetricsCoordination
	retentionLifecycle RuntimeMetricsRetentionLifecycle
}

func NewRuntimeMetricsOpener(
	paths platformartifact.Reserver,
	retentionLifecycle RuntimeMetricsRetentionLifecycle,
	coordination ...RuntimeMetricsCoordination,
) (*RuntimeMetricsOpener, error) {
	if paths == nil {
		return nil, fmt.Errorf("runtime metrics path reserver is required")
	}
	if retentionLifecycle == nil {
		return nil, fmt.Errorf("runtime metrics retention lifecycle is required")
	}
	selectedCoordination, err := selectRuntimeMetricsCoordination(coordination)
	if err != nil {
		return nil, err
	}
	return newRuntimeMetricsOpener(paths, selectedCoordination, retentionLifecycle), nil
}

// NewRuntimeMetricsOpenerWithRetention composes an opener with the
// process-scoped retention lifecycle selected by the composition root.
func NewRuntimeMetricsOpenerWithRetention(
	paths platformartifact.Reserver,
	retentionLifecycle RuntimeMetricsRetentionLifecycle,
	coordination ...RuntimeMetricsCoordination,
) (*RuntimeMetricsOpener, error) {
	return NewRuntimeMetricsOpener(paths, retentionLifecycle, coordination...)
}

func newRuntimeMetricsOpener(
	paths platformartifact.Reserver,
	coordination RuntimeMetricsCoordination,
	retentionLifecycle RuntimeMetricsRetentionLifecycle,
) *RuntimeMetricsOpener {
	return &RuntimeMetricsOpener{
		paths: paths, coordination: coordination, retentionLifecycle: retentionLifecycle,
	}
}

// Open creates a rolling metrics writer from fully selected inputs.
func (opener *RuntimeMetricsOpener) Open(request RuntimeMetricsOpeningRequest) (*RuntimeMetricsSink, error) {
	retentionLease, err := opener.retentionLifecycle.Start(context.Background(), RuntimeMetricsRetentionRequest{
		RootDirectory: request.RootDirectory,
		Config:        request.Config,
	})
	if err != nil {
		return nil, fmt.Errorf("start runtime metrics retention: %w", err)
	}
	if retentionLease == nil {
		return nil, fmt.Errorf("start runtime metrics retention: lifecycle returned nil lease")
	}
	rootLock, err := opener.coordination.LockRoot(context.Background(), request.RootDirectory)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("coordinate runtime metrics startup: %w", err),
			retentionLease.Close(),
		)
	}
	startTimeUTC := request.StartTimeUTC.UTC()
	path, err := opener.paths.Reserve(
		request.RootDirectory,
		startTimeUTC,
		string(internalartifact.RuntimeArtifactKindMetrics),
		internalartifact.RuntimeArtifactPathComponents(request.SessionID, request.RuntimeInstanceID, request.CollisionID),
	)
	if err != nil {
		return nil, errors.Join(err, rootLock.Close(), retentionLease.Close())
	}
	claim, err := opener.coordination.Claim(path)
	if err != nil {
		return nil, errors.Join(
			fmt.Errorf("claim runtime metrics path %q: %w", path, err),
			rootLock.Close(),
			retentionLease.Close(),
		)
	}
	if err := rootLock.Close(); err != nil {
		return nil, errors.Join(
			fmt.Errorf("release runtime metrics startup coordination: %w", err),
			claim.Close(),
			retentionLease.Close(),
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
		writer:         writer,
		encoder:        json.NewEncoder(writer),
		path:           path,
		rootDir:        request.RootDirectory,
		startTimeUTC:   startTimeUTC,
		config:         metricsConfig,
		claim:          claim,
		retentionLease: retentionLease,
	}, nil
}

// Close stops all process-scoped periodic retention loops. Live sink claims
// remain owned by their sinks and are released when those sinks close.
func (opener *RuntimeMetricsOpener) Close(ctx context.Context) error {
	if opener == nil || opener.retentionLifecycle == nil {
		return nil
	}
	return opener.retentionLifecycle.Close(ctx)
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
	var retentionErr error
	if s.retentionLease != nil {
		retentionErr = s.retentionLease.Close()
		s.retentionLease = nil
	}
	return errors.Join(writerErr, claimErr, retentionErr)
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
