package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"gopkg.in/natefinch/lumberjack.v2"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	defaultRuntimeLogDirName = ".you-agent-factory"
	legacyRuntimeLogDirName  = ".agent-factory"
	runtimeLogSubdirName     = "logs"
	runtimeLogExtension      = ".log"
	runtimeLogMonthLayout    = "2006-01"
	runtimeLogDateLayout     = "2006-01-02"
	runtimeLogTimeLayout     = "150405.000000000"
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

// BuildRuntimeLogger creates a zap logger that writes runtime records to a
// bounded rolling JSON log file.
func BuildRuntimeLogger(base *zap.Logger, runtimeInstanceID, runtimeLogDir string, config RuntimeLogConfig) (*RuntimeLogSink, error) {
	if base == nil {
		base = zap.NewNop()
	}
	if runtimeInstanceID == "" {
		return nil, fmt.Errorf("runtime instance ID is required")
	}
	if runtimeLogDir == "" {
		dir, err := defaultRuntimeLogDir()
		if err != nil {
			return nil, err
		}
		runtimeLogDir = dir
	}
	startTimeUTC := time.Now().UTC()
	path := runtimeLogPath(runtimeLogDir, runtimeInstanceID, startTimeUTC, uuid.NewString())
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create runtime log dir %s: %w", filepath.Dir(path), err)
	}

	runtimeLogConfig := normalizeRuntimeLogConfig(config)
	writer := &lumberjack.Logger{
		Filename:   path,
		MaxSize:    runtimeLogConfig.MaxSize,
		MaxBackups: runtimeLogConfig.MaxBackups,
		MaxAge:     runtimeLogConfig.MaxAge,
		Compress:   runtimeLogConfig.Compress,
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	fileCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		zapcore.AddSync(writer),
		zapcore.InfoLevel,
	)
	logger := base.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewTee(core, fileCore)
	})).With(zap.String("runtime_instance_id", runtimeInstanceID))

	return &RuntimeLogSink{
		logger:       logger,
		writer:       writer,
		path:         path,
		rootDir:      runtimeLogDir,
		startTimeUTC: startTimeUTC,
		config:       runtimeLogConfig,
	}, nil
}

func runtimeLogPath(rootDir, runtimeInstanceID string, startTime time.Time, uniqueID string) string {
	startTime = startTime.UTC()
	filename := fmt.Sprintf(
		"%s-%s-%s%s",
		startTime.Format(runtimeLogTimeLayout),
		safeRuntimeLogPathComponent(runtimeInstanceID),
		safeRuntimeLogPathComponent(uniqueID),
		runtimeLogExtension,
	)
	return filepath.Join(
		rootDir,
		startTime.Format(runtimeLogMonthLayout),
		startTime.Format(runtimeLogDateLayout),
		filename,
	)
}

func safeRuntimeLogPathComponent(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(value))
	lastUnderscore := false
	for _, r := range value {
		if r == '-' || r == '_' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	trimmed := strings.Trim(b.String(), "_.-")
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
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

func defaultRuntimeLogDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home for runtime logs: %w", err)
	}
	runtimeLogDir := canonicalRuntimeLogDir(home)
	if err := migrateLegacyRuntimeLogDir(home, runtimeLogDir); err != nil {
		return "", err
	}
	return runtimeLogDir, nil
}

func canonicalRuntimeLogDir(home string) string {
	return filepath.Join(home, defaultRuntimeLogDirName, runtimeLogSubdirName)
}

func legacyRuntimeLogDir(home string) string {
	return filepath.Join(home, legacyRuntimeLogDirName, runtimeLogSubdirName)
}

func migrateLegacyRuntimeLogDir(home, runtimeLogDir string) error {
	legacyLogDir := legacyRuntimeLogDir(home)
	shouldMigrate, err := shouldMigrateLegacyRuntimeLogDir(legacyLogDir, runtimeLogDir)
	if err != nil {
		return err
	}
	if !shouldMigrate {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(runtimeLogDir), 0o755); err != nil {
		return fmt.Errorf("create canonical runtime log parent %s: %w", filepath.Dir(runtimeLogDir), err)
	}
	if err := os.Rename(legacyLogDir, runtimeLogDir); err != nil {
		return fmt.Errorf("migrate legacy runtime logs from %s to %s: %w", legacyLogDir, runtimeLogDir, err)
	}
	return nil
}

func shouldMigrateLegacyRuntimeLogDir(legacyLogDir, runtimeLogDir string) (bool, error) {
	if _, err := os.Stat(legacyLogDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat legacy runtime log dir %s: %w", legacyLogDir, err)
	}

	if _, err := os.Stat(runtimeLogDir); err == nil {
		return false, nil
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("stat runtime log dir %s: %w", runtimeLogDir, err)
	}

	return true, nil
}
