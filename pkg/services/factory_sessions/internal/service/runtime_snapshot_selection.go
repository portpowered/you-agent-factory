package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/logicaltarget"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// RuntimeSnapshotSelection selects detached live or recorded inputs without owning runtime state.
type RuntimeSnapshotSelection struct {
	resolveSnapshot    factorydefinitions.RuntimeSnapshotOperation
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder
	replayInputs       recordings.ReplayInputLoader
	resolveCurrentDir  factorydefinitions.CurrentFactoryDirectoryResolver
	resolveHome        factorysessions.HomeDirectoryResolver
}

// NewRuntimeSnapshotSelection stores the fixed selection capabilities without executing them.
func NewRuntimeSnapshotSelection(
	resolveSnapshot factorydefinitions.RuntimeSnapshotOperation,
	decodeReplayConfig factorydefinitions.ReplayRuntimeConfigDecoder,
	replayInputs recordings.ReplayInputLoader,
	resolveCurrentDir factorydefinitions.CurrentFactoryDirectoryResolver,
	resolveHome factorysessions.HomeDirectoryResolver,
) *RuntimeSnapshotSelection {
	return &RuntimeSnapshotSelection{resolveSnapshot: resolveSnapshot, decodeReplayConfig: decodeReplayConfig,
		replayInputs: replayInputs, resolveCurrentDir: resolveCurrentDir, resolveHome: resolveHome}
}

func (s *RuntimeSnapshotSelection) Resolve(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	recording recordings.RuntimeSelection,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	resumeInput *recordings.LoadResumeInputResult,
	sessionID string,
) (activationSnapshotResolution, error) {
	if replaySnapshot, ok, err := s.resolveLegacyReplaySnapshot(ctx, definition, recording, sessionID, preloadedReplayInput); err != nil {
		return activationSnapshotResolution{}, err
	} else if ok {
		return activationSnapshotResolution{
			snapshot:       replaySnapshot,
			factoryDir:     strings.TrimSpace(replaySnapshot.FactoryDir),
			runtimeBaseDir: firstNonEmptyString(definition.ExecutionBaseDir, replaySnapshot.RuntimeBaseDir),
		}, nil
	}
	if resumeSnapshot, ok, err := s.resolveLegacyResumeSnapshot(ctx, definition, recording, sessionID, resumeInput); err != nil {
		return activationSnapshotResolution{}, err
	} else if ok {
		return activationSnapshotResolution{
			snapshot:       resumeSnapshot,
			factoryDir:     strings.TrimSpace(resumeSnapshot.FactoryDir),
			runtimeBaseDir: firstNonEmptyString(definition.ExecutionBaseDir, resumeSnapshot.RuntimeBaseDir),
		}, nil
	}
	factoryDir, sourcePath, err := s.resolveActivationDefinitionSource(definition)
	if err != nil {
		return activationSnapshotResolution{}, err
	}
	if factoryDir == "" && sourcePath == "" {
		return activationSnapshotResolution{}, fmt.Errorf("open Factory Runtime: Factory Definition directory is required")
	}
	runtimeBaseDir := firstNonEmptyString(definition.ExecutionBaseDir, factoryDir, sourcePath)
	snapshot, err := s.resolveActivationDefinitionSnapshot(ctx, sourcePath, runtimeBaseDir, definition, recording.WorkflowID, sessionID)
	if err != nil {
		return activationSnapshotResolution{}, err
	}
	return activationSnapshotResolution{
		snapshot:       snapshot,
		factoryDir:     factoryDir,
		sourcePath:     sourcePath,
		runtimeBaseDir: runtimeBaseDir,
	}, nil
}

func (s *RuntimeSnapshotSelection) resolveActivationDefinitionSnapshot(
	ctx context.Context,
	sourcePath, runtimeBaseDir string,
	definition factorydefinitions.RuntimeSelection,
	workflowID string,
	sessionID string,
) (factorydefinitions.RuntimeSnapshot, error) {
	if s.resolveSnapshot == nil {
		return factorydefinitions.RuntimeSnapshot{}, runtimeSnapshotResolverUnavailable()
	}
	resolved, err := s.resolveSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
		// SourcePath is the concrete authored file for direct layouts. Do not
		// send the retained directory as FactoryDir as the Definitions root
		// rejects two distinct source identities.
		SourcePath:       sourcePath,
		ExecutionBaseDir: runtimeBaseDir,
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: sessionID,
			WorkflowID:       workflowID,
			Arguments:        work.CloneInvocationArguments(definition.InvocationArguments),
		},
	})
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	snapshot, err := resolved.Snapshot.Clone()
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, fmt.Errorf("open Factory Runtime: detach resolved Factory Definition snapshot: %w", err)
	}
	return snapshot, nil
}

func (s *RuntimeSnapshotSelection) resolveLegacyReplaySnapshot(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	recording recordings.RuntimeSelection,
	sessionID string,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (factorydefinitions.RuntimeSnapshot, bool, error) {
	if !legacyReplayRequested(recording.ReplayPath, preloadedReplayInput, s.replayInputs != nil) {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	input, err := s.loadReplayInputForActivation(recording.ReplayPath, preloadedReplayInput)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf("open Factory Runtime: load replay input for activation: %w", err)
	}
	if input.Portable != nil || input.Legacy == nil || input.Legacy.Factory == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	snapshot, err := s.resolveLegacyFactorySnapshot(ctx, definition, recording.WorkflowID, sessionID, input.Legacy.Factory, "replay")
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s *RuntimeSnapshotSelection) resolveLegacyResumeSnapshot(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	recording recordings.RuntimeSelection,
	sessionID string,
	resumeInput *recordings.LoadResumeInputResult,
) (factorydefinitions.RuntimeSnapshot, bool, error) {
	if resumeInput == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, nil
	}
	input := resumeInput.Input
	if input.Portable != nil || input.Legacy == nil || input.Legacy.Factory == nil {
		return factorydefinitions.RuntimeSnapshot{}, false, fmt.Errorf(
			"open Factory Runtime: resume recording Factory Definition is required",
		)
	}
	snapshot, err := s.resolveLegacyFactorySnapshot(ctx, definition, recording.WorkflowID, sessionID, input.Legacy.Factory, "resume")
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, false, err
	}
	return snapshot, true, nil
}

func (s *RuntimeSnapshotSelection) resolveLegacyFactorySnapshot(
	ctx context.Context,
	definition factorydefinitions.RuntimeSelection,
	workflowID string,
	sessionID string,
	factoryJSON *factorydefinitions.FactorySnapshot,
	intent string,
) (factorydefinitions.RuntimeSnapshot, error) {
	if s.resolveSnapshot == nil {
		return factorydefinitions.RuntimeSnapshot{}, runtimeSnapshotResolverUnavailable()
	}
	replayConfig, err := s.decodeLegacyReplayConfig(factoryJSON)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	factoryDir, runtimeBaseDir := legacyReplayPaths(definition.ExecutionBaseDir, replayConfig)
	resolved, err := s.resolveSnapshot(ctx, factorydefinitions.ResolveRuntimeSnapshotRequest{
		Canonical:        append([]byte(nil), []byte(*factoryJSON)...),
		ExecutionBaseDir: runtimeBaseDir,
		Invocation: factorydefinitions.RuntimeSnapshotInvocationContext{
			FactorySessionID: sessionID,
			WorkflowID:       workflowID,
		},
	})
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	snapshot, err := resolved.Snapshot.Clone()
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, fmt.Errorf(
			"open Factory Runtime: detach %s Factory Definition snapshot: %w",
			intent,
			err,
		)
	}
	if strings.TrimSpace(snapshot.FactoryDir) == "" {
		snapshot.FactoryDir = factoryDir
	}
	if strings.TrimSpace(snapshot.RuntimeBaseDir) == "" {
		snapshot.RuntimeBaseDir = runtimeBaseDir
	}
	return snapshot, nil
}

func legacyReplayRequested(
	replayPath string,
	preloadedReplayInput *recordings.LoadReplayInputResult,
	replayInputsAvailable bool,
) bool {
	return strings.TrimSpace(replayPath) != "" && (preloadedReplayInput != nil || replayInputsAvailable)
}

func (s *RuntimeSnapshotSelection) loadReplayInputForActivation(
	replayPath string,
	preloadedReplayInput *recordings.LoadReplayInputResult,
) (recordings.LoadReplayInputResult, error) {
	if preloadedReplayInput != nil {
		return *preloadedReplayInput, nil
	}
	loaded, err := s.replayInputs.LoadReplayInput(
		recordings.LoadReplayInputRequest{Path: replayPath},
	)
	if err != nil {
		return recordings.LoadReplayInputResult{}, err
	}
	return loaded, nil
}

func (s *RuntimeSnapshotSelection) decodeLegacyReplayConfig(
	factoryJSON *factorydefinitions.FactorySnapshot,
) (factorydefinitions.ReplayRuntimeConfig, error) {
	if s.decodeReplayConfig == nil {
		return nil, fmt.Errorf("open Factory Runtime: replay Factory Definition decoder is required")
	}
	replayConfig, err := s.decodeReplayConfig(factoryJSON)
	if err != nil {
		return nil, fmt.Errorf("open Factory Runtime: decode replay Factory Definition: %w", err)
	}
	if replayConfig == nil {
		return nil, fmt.Errorf("open Factory Runtime: replay Factory Definition is empty")
	}
	return replayConfig, nil
}

func legacyReplayPaths(
	executionBaseDir string,
	replayConfig factorydefinitions.ReplayRuntimeConfig,
) (string, string) {
	factoryDir := strings.TrimSpace(replayConfig.FactoryDir())
	runtimeBaseDir := firstNonEmptyString(executionBaseDir, replayConfig.RuntimeBaseDir())
	factoryDir = firstNonEmptyString(factoryDir, runtimeBaseDir, ".")
	runtimeBaseDir = firstNonEmptyString(runtimeBaseDir, factoryDir)
	return factoryDir, runtimeBaseDir
}

func (s *RuntimeSnapshotSelection) resolveActivationDefinitionSource(
	definition factorydefinitions.RuntimeSelection,
) (string, string, error) {
	if strings.TrimSpace(definition.SourcePath) != "" {
		sourcePath := strings.TrimSpace(definition.SourcePath)
		if !strings.HasPrefix(sourcePath, "~") && s.resolveHome == nil {
			return "", sourcePath, nil
		}
		resolved, err := absolutizeActivationPath(sourcePath, s.resolveHome)
		if err != nil {
			return "", "", fmt.Errorf("open Factory Runtime: resolve Factory source: %w", err)
		}
		return "", resolved, nil
	}
	factoryDir := strings.TrimSpace(definition.Directory)
	if factoryDir == "" {
		return "", "", nil
	}
	if s.resolveCurrentDir != nil {
		resolved, err := s.resolveCurrentDir(factoryDir)
		if err != nil {
			return "", "", fmt.Errorf("open Factory Runtime: resolve Factory directory: %w", err)
		}
		factoryDir = resolved
	}
	resolved, err := absolutizeActivationPath(factoryDir, s.resolveHome)
	if err != nil {
		return "", "", fmt.Errorf("open Factory Runtime: resolve Factory directory: %w", err)
	}
	// The authored loader treats a directory as a split layout and therefore
	// requires body-only worker definitions. Opening has historically accepted
	// a direct factory.json with topology-only workers (including mock-worker
	// runs), so resolve the concrete source file while retaining the directory
	// as the snapshot's Factory identity.
	return resolved, filepath.Join(resolved, factorydefinitions.FactoryConfigFile), nil
}

func absolutizeActivationPath(
	path string,
	resolveHome factorysessions.HomeDirectoryResolver,
) (string, error) {
	if resolveHome == nil && path != "~" && !strings.HasPrefix(path, "~/") && !strings.HasPrefix(path, `~\`) {
		resolved, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("resolve Factory directory %q: %w", path, err)
		}
		return filepath.Clean(resolved), nil
	}
	return logicaltarget.AbsolutizeFactoryDirectory(path, resolveHome)
}

func runtimeSnapshotResolverUnavailable() error {
	return &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticUnavailable,
			Field:   "resolver",
			Message: "Factory Definitions runtime snapshot resolver is unavailable",
		},
		Cause: factorydefinitions.ErrRuntimeSnapshotResolverUnavailable,
	}
}
