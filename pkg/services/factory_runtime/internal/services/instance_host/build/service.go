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
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

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

// PrepareExecutionSpec applies opening-specific execution precedence after
// preparing a detached candidate. Replay and mock effects stay scoped to the
// request; no compatibility builder is needed to select them.
func (s *Service) PrepareExecutionSpec(
	ctx context.Context,
	defaults BuildDefaults,
	values SessionBuildValues,
	selections SessionBuildSpec,
	providerOverride providers.Service,
	providerCommandRunner platformprocess.CommandRunner,
	scriptCommandRunner platformprocess.CommandRunner,
	mockWorkersConfig *workers.MockWorkersConfig,
	newMockCommandRunner MockCommandRunnerFactory,
) (SessionBuildSpec, error) {
	selections.ProviderOverride = providerOverrideForMode(providerOverride, selections.ProviderOverride)
	spec, err := s.PrepareSpec(ctx, defaults, values, selections)
	if err != nil {
		return SessionBuildSpec{}, err
	}
	spec.ProviderCommandRunner = providerCommandRunnerForMode(mockWorkersConfig, providerCommandRunner, spec.LoadedFactoryCfg, newMockCommandRunner)
	spec.CommandRunnerOverride = commandRunnerOverrideForMode(mockWorkersConfig, scriptCommandRunner, spec.LoadedFactoryCfg, spec.ReplayCommandRunner, newMockCommandRunner)
	return spec, nil
}

// SessionScopedRecordPath preserves the selected default path and scopes
// non-default explicit paths by session identity.
func SessionScopedRecordPath(basePath string, sessionID string) string {
	return factory.RecordingPath(basePath).ForSession(sessionID)
}
