package wire

import (
	"context"
	"fmt"
	"sync"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
)

// NewRecordingReplayArtifactsFactory constructs the one phase-aware
// RecordingReplayArtifacts capability used by Factory Sessions. The returned
// runtime capability loads portable or legacy replay input before a runtime
// ledger exists, then binds its finalized-recording operations to the exact
// canonical capability selected by runtimeBuilder once that ledger and its
// projection have been created. logger is the repository's injected
// structured logging abstraction; replay loading never logs a path, decoded
// payload, or raw recording bytes.
func NewRecordingReplayArtifactsFactory(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
	logger *zap.Logger,
	runtimeBuilder recordings.RecordingReplayArtifactsRuntimeBuilder,
) recordings.RecordingReplayArtifactsFactory {
	if logger == nil {
		logger = zap.NewNop()
	}
	return func() recordings.RecordingReplayArtifactsRuntime {
		return &phaseReplayArtifacts{
			input:          newReplayInputLoader(readFile, loadLegacy, logger),
			runtimeBuilder: runtimeBuilder,
		}
	}
}

// NewReplayInputLoader retains the focused test and compatibility entry point
// for callers that only exercise input loading. It returns the primary
// RecordingReplayArtifacts runtime capability rather than a second public
// capability; production composition uses NewRecordingReplayArtifactsFactory
// so the same instance can later bind canonical artifact operations.
func NewReplayInputLoader(
	readFile recordings.RecordingReadFile,
	loadLegacy recordings.ReplayArtifactLoader,
	logger *zap.Logger,
) recordings.RecordingReplayArtifactsRuntime {
	return NewRecordingReplayArtifactsFactory(readFile, loadLegacy, logger, nil)()
}

// phaseReplayArtifacts makes the runtime-opening and finalized-recording
// phases one narrow capability instance. It has no filesystem or lifecycle
// effects at construction. Runtime binding happens only after Factory
// Sessions has produced the request-scoped ledger and projection.
type phaseReplayArtifacts struct {
	input          *replayInputLoader
	runtimeBuilder recordings.RecordingReplayArtifactsRuntimeBuilder

	mu        sync.RWMutex
	artifacts recordings.RecordingReplayArtifacts
}

var _ recordings.RecordingReplayArtifactsRuntime = (*phaseReplayArtifacts)(nil)

func (capability *phaseReplayArtifacts) BindRecordingLifecycle(
	ledger recordings.Ledger,
	projection recordings.ProjectionService,
) (recordings.RecordingLifecycle, error) {
	if capability == nil || capability.runtimeBuilder == nil {
		return nil, fmt.Errorf("construct Recordings replay artifacts: runtime builder is required")
	}
	capability.mu.Lock()
	defer capability.mu.Unlock()
	if capability.artifacts != nil {
		return nil, fmt.Errorf("construct Recordings replay artifacts: runtime is already bound")
	}
	artifacts, lifecycle, err := capability.runtimeBuilder(ledger, projection)
	if err != nil {
		return nil, err
	}
	if artifacts == nil {
		return nil, fmt.Errorf("construct Recordings replay artifacts: runtime builder returned nil capability")
	}
	if lifecycle == nil {
		return nil, fmt.Errorf("construct Recordings replay artifacts: runtime builder returned nil lifecycle")
	}
	capability.artifacts = artifacts
	return lifecycle, nil
}

func (capability *phaseReplayArtifacts) LoadReplayInput(
	request recordings.LoadReplayInputRequest,
) (recordings.LoadReplayInputResult, error) {
	if capability == nil || capability.input == nil {
		return recordings.LoadReplayInputResult{}, &recordings.ReplayInputError{
			Kind:    recordings.ReplayInputErrorRead,
			Message: "Factory Session replay recording reader is required",
		}
	}
	return capability.input.LoadReplayInput(request)
}

func (capability *phaseReplayArtifacts) LoadReplay(
	request recordings.LoadReplayRequest,
) (recordings.LoadReplayResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.LoadReplayResult{}, err
	}
	return artifacts.LoadReplay(request)
}

func (capability *phaseReplayArtifacts) BuildArtifact(
	request recordings.BuildArtifactRequest,
) (recordings.BuildArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.BuildArtifactResult{}, err
	}
	return artifacts.BuildArtifact(request)
}

func (capability *phaseReplayArtifacts) ValidateArtifact(
	request recordings.ValidateArtifactRequest,
) (recordings.ValidateArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.ValidateArtifactResult{}, err
	}
	return artifacts.ValidateArtifact(request)
}

func (capability *phaseReplayArtifacts) EncodeArtifact(
	request recordings.EncodeArtifactRequest,
) (recordings.EncodeArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.EncodeArtifactResult{}, err
	}
	return artifacts.EncodeArtifact(request)
}

func (capability *phaseReplayArtifacts) DecodeArtifact(
	request recordings.DecodeArtifactRequest,
) (recordings.DecodeArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.DecodeArtifactResult{}, err
	}
	return artifacts.DecodeArtifact(request)
}

func (capability *phaseReplayArtifacts) SummarizeArtifact(
	request recordings.SummarizeArtifactRequest,
) (recordings.SummarizeArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.SummarizeArtifactResult{}, err
	}
	return artifacts.SummarizeArtifact(request)
}

func (capability *phaseReplayArtifacts) ExportArtifact(
	ctx context.Context,
	request recordings.ExportArtifactRequest,
) (recordings.ExportArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.ExportArtifactResult{}, err
	}
	return artifacts.ExportArtifact(ctx, request)
}

func (capability *phaseReplayArtifacts) ReadArtifact(
	ctx context.Context,
	request recordings.ReadArtifactRequest,
) (recordings.ReadArtifactResult, error) {
	artifacts, err := capability.boundArtifacts()
	if err != nil {
		return recordings.ReadArtifactResult{}, err
	}
	return artifacts.ReadArtifact(ctx, request)
}

func (capability *phaseReplayArtifacts) boundArtifacts() (recordings.RecordingReplayArtifacts, error) {
	if capability == nil {
		return nil, &recordings.ReplayArtifactError{
			Kind:    recordings.ReplayArtifactErrorUnavailable,
			Message: "Recordings replay artifacts capability is required",
		}
	}
	capability.mu.RLock()
	artifacts := capability.artifacts
	capability.mu.RUnlock()
	if artifacts == nil {
		return nil, &recordings.ReplayArtifactError{
			Kind:    recordings.ReplayArtifactErrorUnavailable,
			Message: "Recordings replay artifacts are not bound to a runtime",
		}
	}
	return artifacts, nil
}
