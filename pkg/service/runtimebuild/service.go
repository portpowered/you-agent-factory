package runtimebuild

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/factory"
	"go.uber.org/zap"
)

// BundleBuilder constructs a runnable runtime bundle from canonical build input.
type BundleBuilder func(ctx context.Context, input BuildInput) (any, error)

// Service owns the single runtime build path for session open and post-save activation.
type Service struct {
	cfg        Config
	clock      factory.Clock
	baseLogger *zap.Logger
	build      BundleBuilder
}

// New constructs a runtime-build collaborator with explicit dependencies.
func New(cfg Config, clock factory.Clock, baseLogger *zap.Logger, build BundleBuilder) *Service {
	return &Service{
		cfg:        cfg,
		clock:      clock,
		baseLogger: baseLogger,
		build:      build,
	}
}

// BuildFromLoadedConfig builds a runtime bundle from an already-loaded factory config.
// Startup and replacement flows both use this entry point.
func (s *Service) BuildFromLoadedConfig(ctx context.Context, input BuildInput) (any, error) {
	if s == nil || s.build == nil {
		return nil, fmt.Errorf("runtime build service is required")
	}
	return s.build(ctx, input)
}

// BuildReplacement loads runtime config from factoryDir and builds a replacement bundle.
// Session open, named activation, and post-save activation all route through this path.
func (s *Service) BuildReplacement(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
) (any, error) {
	if s == nil || s.build == nil {
		return nil, fmt.Errorf("runtime build service is required")
	}
	baseLogger := s.baseLogger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	loadedFactoryCfg, err := configload.LoadRuntimeConfigFromFactoryDir(factoryDir, s.cfg.WorkstationLoader)
	if err != nil {
		return nil, fmt.Errorf("load factory config: %w", err)
	}
	logger := NewSessionLogger(baseLogger, sessionID, folderPath, loadedFactoryCfg.FactoryDir())
	WarnPortableBundledReplacementReport(
		logger,
		"named factory activation replaced portable bundled files",
		loadedFactoryCfg.PortableBundledFileReplacements(),
	)
	loadedFactoryCfg.SetRuntimeBaseDir(s.cfg.ExecutionBaseDir)
	clock := factory.EnsureClock(s.clock)
	recordPath := SessionScopedRecordPath(s.cfg.RecordPath, sessionID)
	return s.build(ctx, BuildInput{
		Dir:                   factoryDir,
		FolderPath:            folderPath,
		SessionID:             sessionID,
		LoadedFactoryCfg:      loadedFactoryCfg,
		BaseLogger:            baseLogger,
		RuntimeInstanceID:     uuid.NewString(),
		Clock:                 clock,
		RecordPath:            recordPath,
		WorkflowID:            s.cfg.WorkflowID,
		ProviderOverride:      providerOverrideForMode(&s.cfg, nil),
		ProviderCommandRunner: providerCommandRunnerForMode(&s.cfg, loadedFactoryCfg),
		CommandRunnerOverride: commandRunnerOverrideForMode(&s.cfg, loadedFactoryCfg, nil),
	})
}

// SessionScopedRecordPath substitutes per-session recording tokens in record paths.
func SessionScopedRecordPath(basePath string, sessionID string) string {
	if strings.TrimSpace(basePath) == "" {
		return basePath
	}
	if strings.Contains(basePath, "__factory_session_id__") {
		return strings.ReplaceAll(basePath, "__factory_session_id__", sessionID)
	}
	if sessionID == "~default" {
		return basePath
	}
	ext := filepath.Ext(basePath)
	base := strings.TrimSuffix(basePath, ext)
	return base + "." + sessionID + ext
}
