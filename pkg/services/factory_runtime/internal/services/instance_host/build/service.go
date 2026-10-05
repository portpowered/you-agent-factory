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

// BuildDefaults contains detached preparation configuration.
type BuildDefaults struct {
	WorkerModelProvider   string
	WorkerModel           string
	ApplyOperatorDefaults bool
	RecordPath            string
	WorkflowID            string
}

// SessionBuildValues contains an exclusively owned candidate and session facts.
type SessionBuildValues struct {
	Dir                                    string
	FolderPath                             string
	SessionID                              string
	ExecutionBaseDir                       string
	RuntimeInstanceID                      string
	LoadedFactoryCfg                       factorydefinitions.MutableLoadedFactorySource
	PreserveCompatibilityDefaultRecordPath bool
}

// PreparedSessionValues contains prepared data without activation effects.
type PreparedSessionValues struct {
	Dir               string
	FolderPath        string
	SessionID         string
	ExecutionBaseDir  string
	RuntimeInstanceID string
	LoadedFactoryCfg  factory.LoadedConfig
	RecordPath        string
	WorkflowID        string
}

// Service prepares session-owned candidates using fixed process collaborators.
type Service struct {
	workstationLoader factorydefinitions.WorkstationLoader
	loadFactory       factory.LoadedFactoryLoader
	newID             factory.IDGenerator
	baseLogger        *zap.Logger
}

// New constructs inert preparation with explicitly selected collaborators.
func New(
	workstationLoader factorydefinitions.WorkstationLoader,
	loadFactory factory.LoadedFactoryLoader,
	newID factory.IDGenerator,
	baseLogger *zap.Logger,
) *Service {
	return &Service{workstationLoader: workstationLoader, loadFactory: loadFactory, newID: newID, baseLogger: baseLogger}
}

// Prepare preserves candidate identity; callers discard a failed candidate.
// It starts no resources and retains the existing downstream cancellation policy.
func (s *Service) Prepare(ctx context.Context, defaults BuildDefaults, values SessionBuildValues) (PreparedSessionValues, error) {
	loaded := values.LoadedFactoryCfg
	if loaded == nil {
		var err error
		loaded, err = s.loadFactory(values.Dir, s.workstationLoader)
		if err != nil {
			return PreparedSessionValues{}, fmt.Errorf("load factory config: %w", err)
		}
	}
	logger := NewSessionLogger(s.baseLogger, values.SessionID, values.FolderPath, loaded.FactoryDir())
	WarnPortableBundledReplacementReport(logger, "named factory activation replaced portable bundled files", loaded.PortableBundledFileReplacements())
	loaded.SetRuntimeBaseDir(values.ExecutionBaseDir)
	if defaults.ApplyOperatorDefaults {
		if err := applyOperatorDefaultsToLoadedConfig(defaults.WorkerModelProvider, defaults.WorkerModel, loaded); err != nil {
			return PreparedSessionValues{}, err
		}
	}
	recordSessionID := values.SessionID
	if values.PreserveCompatibilityDefaultRecordPath {
		recordSessionID = "~default"
	}
	runtimeID := strings.TrimSpace(values.RuntimeInstanceID)
	if runtimeID == "" {
		runtimeID = s.newID()
	}
	return PreparedSessionValues{
		Dir: values.Dir, FolderPath: values.FolderPath, SessionID: values.SessionID,
		ExecutionBaseDir: values.ExecutionBaseDir, RuntimeInstanceID: runtimeID,
		LoadedFactoryCfg: loaded, RecordPath: SessionScopedRecordPath(defaults.RecordPath, recordSessionID), WorkflowID: defaults.WorkflowID,
	}, nil
}

// PrepareSpec derives scoped build data directly through the fixed preparation
// owner. Selections already belong to this opening; preparation neither binds
// another service nor substitutes its clock, logger, or execution effects.
func (s *Service) PrepareSpec(
	ctx context.Context,
	defaults BuildDefaults,
	values SessionBuildValues,
	selections SessionBuildSpec,
) (SessionBuildSpec, error) {
	prepared, err := s.Prepare(ctx, defaults, values)
	if err != nil {
		return SessionBuildSpec{}, err
	}
	selections.Dir = prepared.Dir
	selections.FolderPath = prepared.FolderPath
	selections.SessionID = prepared.SessionID
	selections.ExecutionBaseDir = prepared.ExecutionBaseDir
	selections.RuntimeInstanceID = prepared.RuntimeInstanceID
	selections.LoadedFactoryCfg = prepared.LoadedFactoryCfg
	selections.RecordPath = prepared.RecordPath
	selections.WorkflowID = prepared.WorkflowID
	selections.SubmissionHooks = append([]factory.SubmissionHook(nil), selections.SubmissionHooks...)
	return selections, nil
}

// CompatibilityBuild retains T15 activation and effect selection until its caller migration.
type CompatibilityBuild struct {
	preparation           *Service
	defaults              BuildDefaults
	providerOverride      providers.Service
	providerCommandRunner platformprocess.CommandRunner
	scriptCommandRunner   platformprocess.CommandRunner
	mockWorkersConfig     *workers.MockWorkersConfig
	newMockCommandRunner  MockCommandRunnerFactory
	clock                 factory.Clock
	baseLogger            *zap.Logger
	build                 BundleBuilder
	petriMutationRecorder factory.PetriMutationRecorder
}

// BindCompatibility binds the T15 activation bridge without exposing its effect
// types through the fixed preparation service exported by owner Wire.
func BindCompatibility(
	s *Service,
	defaults BuildDefaults,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	newMockCommandRunner MockCommandRunnerFactory,
	clock factory.Clock,
	baseLogger *zap.Logger,
	build BundleBuilder,
	petriMutationRecorder factory.PetriMutationRecorder,
) (*CompatibilityBuild, error) {
	switch {
	case clock == nil:
		return nil, fmt.Errorf("construct runtime build service: clock is required")
	case s == nil || s.newID == nil:
		return nil, fmt.Errorf("construct runtime build service: ID generator is required")
	case baseLogger == nil || s.baseLogger == nil:
		return nil, fmt.Errorf("construct runtime build service: logger is required")
	case build == nil:
		return nil, fmt.Errorf("construct runtime build service: runtime builder is required")
	case s.loadFactory == nil:
		return nil, fmt.Errorf("construct runtime build service: Factory Definition loader is required")
	}
	return &CompatibilityBuild{preparation: s, defaults: defaults,
		providerOverride: providerOverride, providerCommandRunner: providerCommandRunner,
		scriptCommandRunner: scriptCommandRunner, mockWorkersConfig: mockWorkersConfig,
		newMockCommandRunner: newMockCommandRunner, clock: clock, baseLogger: baseLogger,
		build: build, petriMutationRecorder: petriMutationRecorder}, nil
}

// Build builds a runtime bundle from an immutable session build spec.
func (s *CompatibilityBuild) Build(ctx context.Context, spec SessionBuildSpec) (*factoryhost.Bundle, error) {
	if s == nil || s.build == nil {
		return nil, fmt.Errorf("runtime build service is required")
	}
	spec.PetriMutationRecorder = s.petriMutationRecorder
	return s.build(ctx, spec)
}

// BuildSpec derives an immutable session build spec for startup, session open,
// named activation, and post-save activation.
func (s *CompatibilityBuild) BuildSpec(
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
	spec, err := s.preparation.PrepareSpec(ctx, s.defaults, SessionBuildValues{
		Dir: dir, FolderPath: folderPath, SessionID: sessionID, ExecutionBaseDir: executionBaseDir,
		LoadedFactoryCfg: loadedFactoryCfg, RuntimeInstanceID: runtimeInstanceID,
		PreserveCompatibilityDefaultRecordPath: preserveCompatibilityDefaultRecordPath,
	}, SessionBuildSpec{
		BaseLogger: s.baseLogger, Clock: s.clock,
		ProviderOverride:    providerOverrideForMode(s.providerOverride, replayProvider),
		ReplayCommandRunner: replayCommandRunner,
		SubmissionHooks:     submissionHooks, CompletionPlanner: completionPlanner,
		PetriMutationRecorder: s.petriMutationRecorder,
	})
	if err != nil {
		return SessionBuildSpec{}, err
	}
	spec.ProviderCommandRunner = providerCommandRunnerForMode(s.mockWorkersConfig, s.providerCommandRunner, spec.LoadedFactoryCfg, s.newMockCommandRunner)
	spec.CommandRunnerOverride = commandRunnerOverrideForMode(s.mockWorkersConfig, s.scriptCommandRunner, spec.LoadedFactoryCfg, replayCommandRunner, s.newMockCommandRunner)
	return spec, nil
}

// BuildReplacementSpec loads runtime config from factoryDir and derives a build
// spec for session open, named activation, and post-save activation.
func (s *CompatibilityBuild) BuildReplacementSpec(
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
func (s *CompatibilityBuild) BuildReplacement(
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
