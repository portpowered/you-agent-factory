package logging

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sync"
	"time"

	internalartifact "github.com/portpowered/infinite-you/pkg/platform/internal/runtimeartifact"
	platformartifact "github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
	"gopkg.in/natefinch/lumberjack.v2"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	legacyRuntimeLogDirName  = ".agent-factory"
	runtimeLogSubdirName     = "logs"
	runtimeLogExtension      = internalartifact.RuntimeArtifactExtension
	runtimeLogTimeLayout     = internalartifact.RuntimeArtifactTimeLayout
	defaultRuntimeLogMaxSize = 100
	defaultRuntimeLogBackups = 20
	defaultRuntimeLogMaxAge  = 30

	RuntimeLogAppenderZapRollingFile    = "zap_rolling_file"
	RuntimeEnvLogChannelRecord          = "record"
	RuntimeSuccessCommandOutputPolicy   = "suppressed"
	RuntimeFailureCommandOutputPolicy   = "included"
	RuntimeVerboseCommandOutputPolicy   = "details_on_verbose"
	RuntimeRecordCommandDiagnosticsMode = "preserved"
)

// RuntimeLogConfig controls rolling-file policy for the runtime logger.
// Values are in megabytes (MaxSize) and days (MaxAge).
type RuntimeLogConfig struct {
	// MaxSize sets the maximum size in megabytes of a log file before rotate.
	MaxSize int
	// MaxBackups controls how many backup files are retained.
	MaxBackups int
	// MaxAge controls how many days to retain backup files.
	MaxAge int
	// Compress enables gzip compression for rotated log files.
	Compress bool
}

// DefaultRuntimeLogConfig returns the production rolling-file policy used when
// callers do not set explicit runtime log limits.
func DefaultRuntimeLogConfig() RuntimeLogConfig {
	return RuntimeLogConfig{
		MaxSize:    defaultRuntimeLogMaxSize,
		MaxBackups: defaultRuntimeLogBackups,
		MaxAge:     defaultRuntimeLogMaxAge,
	}
}

// RuntimeLogSink owns the file-backed runtime logger and its rolling writer.
type RuntimeLogSink struct {
	logger       *zap.Logger
	writer       io.Closer
	path         string
	rootDir      string
	startTimeUTC time.Time
	config       RuntimeLogConfig
}

var errRuntimeLogSinkClosed = errors.New("runtime log sink closed")

type runtimeLogWriter struct {
	mu     sync.Mutex
	writer *lumberjack.Logger
	closed bool
}

func newRuntimeLogWriter(writer *lumberjack.Logger) *runtimeLogWriter {
	return &runtimeLogWriter{writer: writer}
}

func (w *runtimeLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errRuntimeLogSinkClosed
	}
	return w.writer.Write(p)
}

func (w *runtimeLogWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	return nil
}

func (w *runtimeLogWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true
	return w.writer.Close()
}

// Logger returns the zap logger enriched with runtime logging fields and the
// rolling file core.
func (s *RuntimeLogSink) Logger() *zap.Logger {
	if s == nil || s.logger == nil {
		return zap.NewNop()
	}
	return s.logger
}

// Path returns the active runtime log path.
func (s *RuntimeLogSink) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

// RootDir returns the runtime log root selected by caller configuration or the
// default home-directory policy.
func (s *RuntimeLogSink) RootDir() string {
	if s == nil {
		return ""
	}
	return s.rootDir
}

// StartTimeUTC returns the UTC timestamp used to organize the active log path.
func (s *RuntimeLogSink) StartTimeUTC() time.Time {
	if s == nil {
		return time.Time{}
	}
	return s.startTimeUTC
}

// Config returns the normalized rolling-file policy applied to the sink.
func (s *RuntimeLogSink) Config() RuntimeLogConfig {
	if s == nil {
		return RuntimeLogConfig{}
	}
	return s.config
}

// Artifact returns the sink metadata without exposing its writer.
func (s *RuntimeLogSink) Artifact() RuntimeLogArtifact {
	if s == nil {
		return RuntimeLogArtifact{}
	}
	return RuntimeLogArtifact{
		Path: s.path, RootDir: s.rootDir, StartTimeUTC: s.startTimeUTC, Config: s.config,
	}
}

type RuntimeLogArtifact struct {
	Path         string
	RootDir      string
	StartTimeUTC time.Time
	Config       RuntimeLogConfig
}

// Close releases the runtime log writer.
func (s *RuntimeLogSink) Close() error {
	if s == nil || s.writer == nil {
		return nil
	}
	if err := s.writer.Close(); err != nil {
		return err
	}
	return nil
}

type zapLogger struct {
	l       *zap.Logger
	verbose bool
}

func (l *zapLogger) Debug(msg string, keysAndValues ...any) {
	kv := mapZapFields(keysAndValues...)
	l.l.Debug(msg, kv...)
}

func (l *zapLogger) Info(msg string, keysAndValues ...any) {
	kv := mapZapFields(keysAndValues...)
	l.l.Info(msg, kv...)
}

func (l *zapLogger) Warn(msg string, keysAndValues ...any) {
	kv := mapZapFields(keysAndValues...)
	l.l.Warn(msg, kv...)
}

func (l *zapLogger) Error(msg string, keysAndValues ...any) {
	kv := mapZapFields(keysAndValues...)
	l.l.Error(msg, kv...)
}

func (l *zapLogger) Verbose(msg string, keysAndValues ...any) {
	if !l.verbose {
		return
	}
	kv := mapZapFields(keysAndValues...)
	l.l.Info(msg, kv...)
}

// NewZapLogger adapts a zap logger to the factory logging interface.
func NewZapLogger(l *zap.Logger, verbose bool) Logger {
	if l == nil {
		l = zap.NewNop()
	}
	return &zapLogger{l: l, verbose: verbose}
}

func mapZapFields(keysAndValues ...any) []zap.Field {
	kv := make([]zap.Field, 0, len(keysAndValues))
	for i := 0; i < len(keysAndValues); i += 2 {
		kv = append(kv, zap.Any(keysAndValues[i].(string), keysAndValues[i+1]))
	}
	return kv
}

type RuntimeLogOpeningRequest struct {
	BaseLogger        *zap.Logger
	RuntimeInstanceID string
	RootDirectory     string
	StartTimeUTC      time.Time
	CollisionID       string
	Config            RuntimeLogConfig
}

type RuntimeLogOpener struct{ paths platformartifact.Reserver }

func NewRuntimeLogOpener(paths platformartifact.Reserver) (*RuntimeLogOpener, error) {
	if paths == nil {
		return nil, fmt.Errorf("runtime log path reserver is required")
	}
	return &RuntimeLogOpener{paths: paths}, nil
}

// Open creates a rolling runtime log sink from fully selected inputs.
func (opener *RuntimeLogOpener) Open(request RuntimeLogOpeningRequest) (*RuntimeLogSink, error) {
	if opener == nil || opener.paths == nil {
		return nil, fmt.Errorf("runtime log opener is required")
	}
	if request.BaseLogger == nil {
		return nil, fmt.Errorf("base logger is required")
	}
	if request.RuntimeInstanceID == "" {
		return nil, fmt.Errorf("runtime instance ID is required")
	}
	if request.RootDirectory == "" {
		return nil, fmt.Errorf("runtime log root is required")
	}
	if request.StartTimeUTC.IsZero() {
		return nil, fmt.Errorf("runtime log start time is required")
	}
	if request.CollisionID == "" {
		return nil, fmt.Errorf("runtime log collision ID is required")
	}
	startTimeUTC := request.StartTimeUTC.UTC()
	path, err := opener.paths.Reserve(
		request.RootDirectory,
		startTimeUTC,
		string(internalartifact.RuntimeArtifactKindLog),
		internalartifact.RuntimeArtifactPathComponents(request.RuntimeInstanceID, request.CollisionID),
	)
	if err != nil {
		return nil, err
	}

	runtimeLogConfig := normalizeRuntimeLogConfig(request.Config)
	rollingWriter := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    runtimeLogConfig.MaxSize,
		MaxBackups: runtimeLogConfig.MaxBackups,
		MaxAge:     runtimeLogConfig.MaxAge,
		Compress:   runtimeLogConfig.Compress,
	}
	writer := newRuntimeLogWriter(rollingWriter)

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(writer),
		zapcore.InfoLevel,
	)
	logger := request.BaseLogger.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, fileCore)
	})).With(zap.String("runtime_instance_id", request.RuntimeInstanceID))

	return &RuntimeLogSink{
		logger:       logger,
		writer:       writer,
		path:         path,
		rootDir:      request.RootDirectory,
		startTimeUTC: startTimeUTC,
		config:       runtimeLogConfig,
	}, nil
}
func normalizeRuntimeLogConfig(config RuntimeLogConfig) RuntimeLogConfig {
	if config.MaxSize <= 0 {
		config.MaxSize = DefaultRuntimeLogConfig().MaxSize
	}
	if config.MaxBackups < 0 {
		config.MaxBackups = 0
	}
	if config.MaxAge < 0 {
		config.MaxAge = 0
	}
	if config.MaxBackups == 0 && config.MaxAge == 0 {
		defaults := DefaultRuntimeLogConfig()
		config.MaxBackups = defaults.MaxBackups
		config.MaxAge = defaults.MaxAge
	}
	return config
}

func canonicalRuntimeLogDir(home string) string {
	return RuntimeLogsRoot(home)
}

// RuntimeLogsRoot returns the Logging-owned default runtime log root.
func RuntimeLogsRoot(home string) string {
	return filepath.Join(home, ".you-agent-factory", runtimeLogSubdirName)
}

func legacyRuntimeLogDir(home string) string {
	return filepath.Join(home, legacyRuntimeLogDirName, runtimeLogSubdirName)
}
