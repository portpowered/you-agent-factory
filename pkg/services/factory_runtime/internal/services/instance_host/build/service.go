// Package runtimebuild owns immutable Factory Session runtime specifications
// and construction. Process composition injects its dependencies through wire.
package runtimebuild

import (
	"context"
	"fmt"
	"strings"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

// BundleBuilder constructs a runnable runtime bundle from an immutable session
// build spec.
type BundleBuilder func(ctx context.Context, spec SessionBuildSpec) (*factoryhost.Bundle, error)

// Service owns the single runtime build path for session open and post-save activation.
type Service struct {
	defaultWorkerModelProvider string
	defaultWorkerModel         string
	applyOperatorDefaults      bool
	recordPath                 string
	workflowID                 string
	workstationLoader          factorydefinitions.WorkstationLoader
	loadFactory                factory.LoadedFactoryLoader
	providerOverride           providers.Service
	providerCommandRunner      platformprocess.CommandRunner
	scriptCommandRunner        platformprocess.CommandRunner
	mockWorkersConfig          *workers.MockWorkersConfig
	newMockCommandRunner       MockCommandRunnerFactory
	clock                      factory.Clock
	newID                      factory.IDGenerator
	baseLogger                 *zap.Logger
	build                      BundleBuilder
	petriMutationRecorder      factory.PetriMutationRecorder
}

// New constructs a runtime-build collaborator with explicit dependencies.
// Construction validates process-owned dependencies before any runtime bundle
// or lifecycle can be started.
func New(
	defaultWorkerModelProvider string,
	defaultWorkerModel string,
	applyOperatorDefaults bool,
	recordPath string,
	workflowID string,
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factory.LoadedFactoryLoader,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	newMockCommandRunner MockCommandRunnerFactory,
	clock factory.Clock,
	newID factory.IDGenerator,
	baseLogger *zap.Logger,
	build BundleBuilder,
	petriMutationRecorder factory.PetriMutationRecorder,
) (*Service, error) {
	switch {
	case clock == nil:
		return nil, fmt.Errorf("construct runtime build service: clock is required")
	case newID == nil:
		return nil, fmt.Errorf("construct runtime build service: ID generator is required")
	case baseLogger == nil:
		return nil, fmt.Errorf("construct runtime build service: logger is required")
	case build == nil:
		return nil, fmt.Errorf("construct runtime build service: runtime builder is required")
	case loadFactory == nil:
		return nil, fmt.Errorf("construct runtime build service: Factory Definition loader is required")
	}
	return &Service{
		defaultWorkerModelProvider: defaultWorkerModelProvider,
		defaultWorkerModel:         defaultWorkerModel,
		applyOperatorDefaults:      applyOperatorDefaults,
		recordPath:                 recordPath,
		workflowID:                 workflowID,
		workstationLoader:          workstationLoader,
		loadFactory:                loadFactory,
		providerOverride:           providerOverride,
		providerCommandRunner:      providerCommandRunner,
		scriptCommandRunner:        scriptCommandRunner,
		mockWorkersConfig:          mockWorkersConfig,
		newMockCommandRunner:       newMockCommandRunner,
		clock:                      clock,
		newID:                      newID,
		baseLogger:                 baseLogger,
		build:                      build,
		petriMutationRecorder:      petriMutationRecorder,
	}, nil
}

// Build builds a runtime bundle from an immutable session build spec.
func (s *Service) Build(ctx context.Context, spec SessionBuildSpec) (*factoryhost.Bundle, error) {
	if s == nil || s.build == nil {
		return nil, fmt.Errorf("runtime build service is required")
	}
	spec.PetriMutationRecorder = s.petriMutationRecorder
	return s.build(ctx, spec)
}

// BuildSpec derives an immutable session build spec for startup, session open,
// named activation, and post-save activation.
func (s *Service) BuildSpec(
	ctx context.Context,
	dir string,
	folderPath string,
	sessionID string,
	executionBaseDir string,
	loadedFactoryCfg factorydefinitions.MutableLoadedFactorySource,
	runtimeInstanceID string,
	replayProvider providers.Service,
	replayCommandRunner platformprocess.CommandRunner,
	submissionHooks []factory.SubmissionHook,
	completionPlanner factory.CompletionDeliveryPlanner,
	preserveCompatibilityDefaultRecordPath bool,
) (SessionBuildSpec, error) {
	if s == nil || s.build == nil {
		return SessionBuildSpec{}, fmt.Errorf("runtime build service is required")
	}
	baseLogger := s.baseLogger
	if baseLogger == nil {
		baseLogger = zap.NewNop()
	}
	if loadedFactoryCfg == nil {
		var err error
		loadedFactoryCfg, err = s.loadFactory(dir, s.workstationLoader)
		if err != nil {
			return SessionBuildSpec{}, fmt.Errorf("load factory config: %w", err)
		}
	}
	logger := NewSessionLogger(baseLogger, sessionID, folderPath, loadedFactoryCfg.FactoryDir())
	WarnPortableBundledReplacementReport(
		logger,
		"named factory activation replaced portable bundled files",
		loadedFactoryCfg.PortableBundledFileReplacements(),
	)
	loadedFactoryCfg.SetRuntimeBaseDir(executionBaseDir)
	if s.applyOperatorDefaults {
		if err := applyOperatorDefaultsToLoadedConfig(
			s.defaultWorkerModelProvider,
			s.defaultWorkerModel,
			loadedFactoryCfg,
		); err != nil {
			return SessionBuildSpec{}, err
		}
	}
	recordSessionID := sessionID
	if preserveCompatibilityDefaultRecordPath {
		recordSessionID = "~default"
	}
	recordPath := SessionScopedRecordPath(s.recordPath, recordSessionID)
	runtimeInstanceID = strings.TrimSpace(runtimeInstanceID)
	if runtimeInstanceID == "" {
		runtimeInstanceID = s.newID()
	}
	return SessionBuildSpec{
		Dir:                   dir,
		FolderPath:            folderPath,
		SessionID:             sessionID,
		ExecutionBaseDir:      executionBaseDir,
		LoadedFactoryCfg:      loadedFactoryCfg,
		BaseLogger:            baseLogger,
		RuntimeInstanceID:     runtimeInstanceID,
		Clock:                 s.clock,
		RecordPath:            recordPath,
		WorkflowID:            s.workflowID,
		ProviderOverride:      providerOverrideForMode(s.providerOverride, replayProvider),
		ProviderCommandRunner: providerCommandRunnerForMode(s.mockWorkersConfig, s.providerCommandRunner, loadedFactoryCfg, s.newMockCommandRunner),
		CommandRunnerOverride: commandRunnerOverrideForMode(s.mockWorkersConfig, s.scriptCommandRunner, loadedFactoryCfg, replayCommandRunner, s.newMockCommandRunner),
		ReplayCommandRunner:   replayCommandRunner,
		SubmissionHooks:       append([]factory.SubmissionHook(nil), submissionHooks...),
		CompletionPlanner:     completionPlanner,
		PetriMutationRecorder: s.petriMutationRecorder,
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
	return s.BuildSpec(
		ctx, factoryDir, folderPath, sessionID, executionBaseDir,
		nil, "", nil, nil, nil, nil, false,
	)
}

// BuildReplacement derives a build spec and constructs the replacement bundle.
func (s *Service) BuildReplacement(
	ctx context.Context,
	folderPath string,
	factoryDir string,
	sessionID string,
	executionBaseDir string,
) (factory.RuntimeRecord, error) {
	spec, err := s.BuildReplacementSpec(ctx, folderPath, factoryDir, sessionID, executionBaseDir)
	if err != nil {
		return nil, err
	}
	return s.Build(ctx, spec)
}

// SessionScopedRecordPath preserves the selected default path and scopes
// non-default explicit paths by session identity.
func SessionScopedRecordPath(basePath string, sessionID string) string {
	return factory.RecordingPath(basePath).ForSession(sessionID)
}
