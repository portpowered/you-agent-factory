package factory

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// SessionLoggerFactory annotates a base logger with Factory Session identity.
// Wire selects the implementation; session-opening code supplies only values.
type SessionLoggerFactory func(
	base *zap.Logger,
	sessionID string,
	folderPath string,
	factoryDir string,
) *zap.Logger

// NewSessionLogger annotates runtime logs with Factory Session and directory
// identity.
func NewSessionLogger(
	base *zap.Logger,
	sessionID string,
	folderPath string,
	factoryDir string,
) *zap.Logger {
	if base == nil {
		base = zap.NewNop()
	}
	return base.With(
		zap.String("session_id", sessionID),
		zap.String("folder_path", folderPath),
		zap.String("factory_dir", factoryDir),
	)
}

// WarnPortableBundledReplacementReport logs portable bundled-file replacement
// targets selected during runtime loading.
func WarnPortableBundledReplacementReport(
	logger *zap.Logger,
	message string,
	replacements []factorydefinitions.PortableBundledFileReplacement,
) {
	if logger == nil || len(replacements) == 0 {
		return
	}
	targets := make([]string, 0, len(replacements))
	for _, replacement := range replacements {
		targets = append(targets, replacement.TargetPath)
	}
	logger.Warn(message, zap.Strings("target_paths", targets))
}
