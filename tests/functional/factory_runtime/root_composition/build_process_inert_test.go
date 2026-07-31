package root_composition_test

import (
	"context"
	"errors"
	"io/fs"
	"sync/atomic"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var errRecordingFactoryRuntimeEffect = errors.New("recording Factory Runtime effect invoked during BuildProcess")

// TestFactoryRuntimeEffectsRemainInertThroughRootBuildProcessConstruction proves
// root.BuildProcess composes Factory Runtime without invoking control,
// observation, dispatch-plan, or representative JavaScript/orchestration
// external effects before runtime lifecycle starts.
func TestFactoryRuntimeEffectsRemainInertThroughRootBuildProcessConstruction(t *testing.T) {
	t.Parallel()

	recorder := newFactoryRuntimeEffectRecorder()
	_ = support.BuildProcess(t, recorder.edges())

	if got := recorder.totalControl(); got != 0 {
		t.Fatalf("control effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalObservation(); got != 0 {
		t.Fatalf("observation effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalDispatchPlan(); got != 0 {
		t.Fatalf("dispatch-plan effect calls = %d during BuildProcess, want 0", got)
	}
	if got := recorder.totalJavaScriptOrchestration(); got != 0 {
		t.Fatalf(
			"JavaScript/orchestration effect calls = %d during BuildProcess, want 0",
			got,
		)
	}
}

type factoryRuntimeEffectRecorder struct {
	idGeneratorCalls atomic.Int32
	directoryMkdir   atomic.Int32
	directoryStat    atomic.Int32
	inputReadDir     atomic.Int32
	inputReadFile    atomic.Int32
	inputStat        atomic.Int32
	inputWalk        atomic.Int32
	dispatchRecord   atomic.Int32
	workflowReadDir  atomic.Int32
	workflowReadFile atomic.Int32
	workflowStat     atomic.Int32
	workflowSymlink  atomic.Int32
	workflowHome     atomic.Int32
	scriptCommand    atomic.Int32
}

func newFactoryRuntimeEffectRecorder() *factoryRuntimeEffectRecorder {
	return &factoryRuntimeEffectRecorder{}
}

func (recorder *factoryRuntimeEffectRecorder) edges() serviceedges.Edges {
	return serviceedges.Edges{
		FactoryRuntimeIDGenerator:                   recorder.recordID,
		FactoryRuntimeDirectories:                   &factoryRuntimeDirectoryRecorder{recorder: recorder},
		FactoryRuntimeInputs:                        &factoryRuntimeInputRecorder{recorder: recorder},
		FactoryRuntimeInputDirectoryWalker:          recorder.recordInputWalk,
		DispatchRecorder:                            recorder.recordDispatch,
		FactoryRuntimeWorkflowSources:               &factoryRuntimeWorkflowSourceRecorder{recorder: recorder},
		FactoryRuntimeWorkflowSourceResolveSymlinks: recorder.recordWorkflowSymlink,
		FactoryRuntimeWorkflowHome:                  recorder.recordWorkflowHome,
		ScriptCommandRunner:                         &factoryRuntimeScriptCommandRecorder{recorder: recorder},
	}
}

func (recorder *factoryRuntimeEffectRecorder) totalControl() int32 {
	return recorder.idGeneratorCalls.Load() +
		recorder.directoryMkdir.Load() +
		recorder.directoryStat.Load()
}

func (recorder *factoryRuntimeEffectRecorder) totalObservation() int32 {
	return recorder.inputReadDir.Load() +
		recorder.inputReadFile.Load() +
		recorder.inputStat.Load() +
		recorder.inputWalk.Load()
}

func (recorder *factoryRuntimeEffectRecorder) totalDispatchPlan() int32 {
	return recorder.dispatchRecord.Load()
}

func (recorder *factoryRuntimeEffectRecorder) totalJavaScriptOrchestration() int32 {
	return recorder.workflowReadDir.Load() +
		recorder.workflowReadFile.Load() +
		recorder.workflowStat.Load() +
		recorder.workflowSymlink.Load() +
		recorder.workflowHome.Load() +
		recorder.scriptCommand.Load()
}

func (recorder *factoryRuntimeEffectRecorder) recordID() string {
	recorder.idGeneratorCalls.Add(1)
	return "factory-runtime-edge-id"
}

func (recorder *factoryRuntimeEffectRecorder) recordInputWalk(string, fs.WalkDirFunc) error {
	recorder.inputWalk.Add(1)
	return errRecordingFactoryRuntimeEffect
}

func (recorder *factoryRuntimeEffectRecorder) recordDispatch(recordings.FactoryDispatchRecord) {
	recorder.dispatchRecord.Add(1)
}

func (recorder *factoryRuntimeEffectRecorder) recordWorkflowSymlink(string) (string, error) {
	recorder.workflowSymlink.Add(1)
	return "", errRecordingFactoryRuntimeEffect
}

func (recorder *factoryRuntimeEffectRecorder) recordWorkflowHome() (string, error) {
	recorder.workflowHome.Add(1)
	return "", errRecordingFactoryRuntimeEffect
}

type factoryRuntimeDirectoryRecorder struct {
	recorder *factoryRuntimeEffectRecorder
}

func (adapter *factoryRuntimeDirectoryRecorder) MkdirAll(string, fs.FileMode) error {
	adapter.recorder.directoryMkdir.Add(1)
	return errRecordingFactoryRuntimeEffect
}

func (adapter *factoryRuntimeDirectoryRecorder) Stat(string) (fs.FileInfo, error) {
	adapter.recorder.directoryStat.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

type factoryRuntimeInputRecorder struct {
	recorder *factoryRuntimeEffectRecorder
}

func (adapter *factoryRuntimeInputRecorder) ReadDir(string) ([]fs.DirEntry, error) {
	adapter.recorder.inputReadDir.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

func (adapter *factoryRuntimeInputRecorder) ReadFile(string) ([]byte, error) {
	adapter.recorder.inputReadFile.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

func (adapter *factoryRuntimeInputRecorder) Stat(string) (fs.FileInfo, error) {
	adapter.recorder.inputStat.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

type factoryRuntimeWorkflowSourceRecorder struct {
	recorder *factoryRuntimeEffectRecorder
}

func (adapter *factoryRuntimeWorkflowSourceRecorder) ReadDir(string) ([]fs.DirEntry, error) {
	adapter.recorder.workflowReadDir.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

func (adapter *factoryRuntimeWorkflowSourceRecorder) ReadFile(string) ([]byte, error) {
	adapter.recorder.workflowReadFile.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

func (adapter *factoryRuntimeWorkflowSourceRecorder) Stat(string) (fs.FileInfo, error) {
	adapter.recorder.workflowStat.Add(1)
	return nil, errRecordingFactoryRuntimeEffect
}

type factoryRuntimeScriptCommandRecorder struct {
	recorder *factoryRuntimeEffectRecorder
}

func (adapter *factoryRuntimeScriptCommandRecorder) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	adapter.recorder.scriptCommand.Add(1)
	return platformprocess.CommandResult{}, errRecordingFactoryRuntimeEffect
}
