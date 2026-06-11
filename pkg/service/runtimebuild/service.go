package runtimebuild

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/factory"
	"go.uber.org/zap"
)

func applyOperatorDefaultsToLoadedConfig(defaults operatorconfig.ResolvedDefaults, loaded *factoryconfig.LoadedFactoryConfig) error {
	if loaded == nil {
		return nil
	}
	if err := factoryconfig.ApplyOperatorDefaultsToLoadedConfig(loaded, defaults); err != nil {
		return fmt.Errorf("apply operator defaults: %w", err)
	}
	return factoryconfig.ValidateModelWorkerRuntimeProviders(loaded)
}

// BundleBuilder constructs a runnable runtime bundle from an immutable session
// build spec.
type BundleBuilder func(ctx context.Context, spec SessionBuildSpec) (any, error)

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

// Build builds a runtime bundle from an immutable session build spec.
func (s *Service) Build(ctx context.Context, spec SessionBuildSpec) (any, error) {
	if s == nil || s.build == nil {
		return nil, fmt.Errorf("runtime build service is required")
	}
	return s.build(ctx, spec)
}

// BuildSpec derives an immutable session build spec for startup, session open,
// named activation, and post-save activation.
func (s *Service) BuildSpec(
	ctx context.Context,
	input SessionSpecInput,
) (SessionBuildSpec, error) {
	if s == nil || s.build == nil {
		return SessionBuildSpec{}, fmt.Errorf("runtime build service is required")
	}
	baseLogger := s.baseLogger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	loadedFactoryCfg := input.LoadedFactoryCfg
	if loadedFactoryCfg == nil {
		var err error
		loadedFactoryCfg, err = configload.LoadRuntimeConfigFromFactoryDir(input.Dir, s.cfg.WorkstationLoader)
		if err != nil {
			return SessionBuildSpec{}, fmt.Errorf("load factory config: %w", err)
		}
	}
	logger := NewSessionLogger(baseLogger, input.SessionID, input.FolderPath, loadedFactoryCfg.FactoryDir())
	WarnPortableBundledReplacementReport(
		logger,
		"named factory activation replaced portable bundled files",
		loadedFactoryCfg.PortableBundledFileReplacements(),
	)
	loadedFactoryCfg.SetRuntimeBaseDir(input.ExecutionBaseDir)
	if s.cfg.ApplyOperatorDefaults {
		if err := applyOperatorDefaultsToLoadedConfig(s.cfg.OperatorDefaults, loadedFactoryCfg); err != nil {
			return SessionBuildSpec{}, err
		}
	}
	clock := factory.EnsureClock(s.clock)
	recordPath := SessionScopedRecordPath(s.cfg.RecordPath, input.SessionID)
	runtimeInstanceID := strings.TrimSpace(input.RuntimeInstanceID)
	if runtimeInstanceID == "" {
		runtimeInstanceID = uuid.NewString()
	}
	return SessionBuildSpec{
		Dir:                   input.Dir,
		FolderPath:            input.FolderPath,
		SessionID:             input.SessionID,
		ExecutionBaseDir:      input.ExecutionBaseDir,
		LoadedFactoryCfg:      loadedFactoryCfg,
		BaseLogger:            baseLogger,
		RuntimeInstanceID:     runtimeInstanceID,
		Clock:                 clock,
		RecordPath:            recordPath,
		WorkflowID:            s.cfg.WorkflowID,
		ProviderOverride:      providerOverrideForMode(&s.cfg, input.SideEffects),
		ProviderCommandRunner: providerCommandRunnerForMode(&s.cfg, loadedFactoryCfg),
		CommandRunnerOverride: commandRunnerOverrideForMode(&s.cfg, loadedFactoryCfg, input.SideEffects),
		AdditionalFactoryOpts: input.AdditionalFactoryOpts,
	}, nil
}

// BuildReplacementSpec loads runtime config from factoryDir and derives a build
// spec for session open, named activation, and post-save activation.
func (s *Service) BuildReplacementSpec(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
	executionBaseDir string,
) (SessionBuildSpec, error) {
	return s.BuildSpec(ctx, SessionSpecInput{
		Dir:              factoryDir,
		FolderPath:       folderPath,
		SessionID:        sessionID,
		ExecutionBaseDir: executionBaseDir,
	})
}

// BuildReplacement derives a build spec and constructs the replacement bundle.
func (s *Service) BuildReplacement(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
	executionBaseDir string,
) (any, error) {
	spec, err := s.BuildReplacementSpec(ctx, folderPath, factoryDir, sessionID, executionBaseDir)
	if err != nil {
		return nil, err
	}
	return s.Build(ctx, spec)
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
