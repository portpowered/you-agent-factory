package factory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// RuntimeActivationInputs are the remaining value selections needed by the
// Factory Sessions opener after Definitions has resolved a RuntimeSnapshot.
// The type deliberately duplicates no service or implementation contract: it
// carries only paths, policy strings, limits, mock-worker configuration
// values, and detached resume facts selected by the Recordings root.
type RuntimeActivationInputs struct {
	Definition          RuntimeActivationDefinitionInputs
	Session             RuntimeActivationSessionInputs
	Workers             RuntimeActivationWorkerInputs
	Recordings          RuntimeActivationRecordingInputs
	ResumeInput         recordings.LoadResumeInputResult
	RecoveryInput       RuntimeActivationRecoveryInput
	ModelCacheDirectory string
	OperatorDefaults    RuntimeActivationOperatorDefaults
}

// RuntimeActivationRecoveryInput carries already-selected recovery facts.
// CheckpointContinuation identifies the deferred, inspection-owned opening;
// it suppresses operator defaults, runtime stream publication and sidecars.
// It contains no live owner, callbacks or runtime handles.
type RuntimeActivationRecoveryInput struct {
	WorldState             *factorydefinitions.FactoryWorldState
	EventHistory           []factorydefinitions.FactoryEvent
	ReplayArtifact         *factorydefinitions.ReplayArtifact
	CheckpointContinuation bool
}

type RuntimeActivationDefinitionInputs struct {
	Directory        string
	SourcePath       string
	ExecutionBaseDir string
}

type RuntimeActivationSessionInputs struct {
	// CanonicalSessionID is the preallocated runtime identity for an automatic
	// default recording and its runtime metrics; the public session alias
	// remains separate.
	CanonicalSessionID string
	// CanonicalSessionIDGenerated preserves whether Factory Sessions allocated
	// the canonical identity before Runtime activation.
	CanonicalSessionIDGenerated bool
	PersistencePolicy           string
	BackendScopeID              string
	SystemConfigHome            string
	SystemConfigPath            string
	WorkFile                    string
	Host                        RuntimeActivationHostInputs
}

type RuntimeActivationHostInputs struct {
	Directory   string
	RuntimeMode factorydefinitions.RuntimeMode
	WorkFile    string
	MockWorkers bool
	Host        string
	Port        int
	AutoPort    bool
	Pprof       bool
}

type RuntimeActivationWorkerInputs struct {
	RunnerID                          string
	Worktree                          string
	WorkerReasoningEffort             string
	MockWorkers                       *RuntimeActivationMockWorkersConfig
	InvocationSkipPermissionsOverride *bool
	SkipBuiltInPrerequisiteValidation bool
}

type RuntimeActivationMockWorkersConfig struct {
	MockWorkers             []RuntimeActivationMockWorker
	UnmatchedDispatchPolicy string
}

type RuntimeActivationMockWorker struct {
	ID              string
	WorkerName      string
	WorkstationName string
	WorkInputs      []RuntimeActivationMockWorkInput
	RunType         string
	ScriptConfig    *RuntimeActivationMockScript
	RejectConfig    *RuntimeActivationMockReject
	GateConfig      *RuntimeActivationMockGate
	Usage           *RuntimeActivationMockUsage
}

type RuntimeActivationMockGate struct {
	ArrivedFile string
	ReleaseFile string
	Timeout     string
}

type RuntimeActivationMockUsage struct {
	Provider              string
	Model                 string
	InputTokens           *int64
	OutputTokens          *int64
	CachedInputTokens     *int64
	ReasoningOutputTokens *int64
}

type RuntimeActivationMockWorkInput struct {
	WorkID      string
	WorkType    string
	State       string
	InputName   string
	TraceID     string
	Channel     string
	PayloadHash string
}

type RuntimeActivationMockScript struct {
	Command          string
	Args             []string
	Env              map[string]string
	WorkingDirectory string
	Stdin            string
	Timeout          string
}

type RuntimeActivationMockReject struct {
	Stdout   string
	Stderr   string
	ExitCode *int
}

type RuntimeActivationRecordingInputs struct {
	RecordPath    string
	ReplayPath    string
	ResumePath    string
	WorkflowID    string
	FlushInterval time.Duration
}

type RuntimeActivationOperatorDefaults struct {
	WorkerModelProvider string
	WorkerModel         string
	ConfigPath          string
}

// Clone detaches all nested slices, maps, and pointer values before the
// Runtime root stores an activation request.
func (inputs RuntimeActivationInputs) Clone() (RuntimeActivationInputs, error) {
	cloned := inputs
	var err error
	cloned.RecoveryInput, err = cloneRuntimeActivationRecovery(inputs.RecoveryInput)
	if err != nil {
		return RuntimeActivationInputs{}, err
	}
	if inputs.Workers.InvocationSkipPermissionsOverride != nil {
		value := *inputs.Workers.InvocationSkipPermissionsOverride
		cloned.Workers.InvocationSkipPermissionsOverride = &value
	}
	if inputs.Workers.MockWorkers == nil {
		return cloned, nil
	}
	mock := &RuntimeActivationMockWorkersConfig{
		UnmatchedDispatchPolicy: inputs.Workers.MockWorkers.UnmatchedDispatchPolicy,
		MockWorkers:             make([]RuntimeActivationMockWorker, len(inputs.Workers.MockWorkers.MockWorkers)),
	}
	for index, worker := range inputs.Workers.MockWorkers.MockWorkers {
		clonedWorker := worker
		clonedWorker.WorkInputs = append([]RuntimeActivationMockWorkInput(nil), worker.WorkInputs...)
		if worker.ScriptConfig != nil {
			script := *worker.ScriptConfig
			script.Args = append([]string(nil), worker.ScriptConfig.Args...)
			script.Env = make(map[string]string, len(worker.ScriptConfig.Env))
			for key, value := range worker.ScriptConfig.Env {
				script.Env[key] = value
			}
			clonedWorker.ScriptConfig = &script
		}
		if worker.RejectConfig != nil {
			reject := *worker.RejectConfig
			if worker.RejectConfig.ExitCode != nil {
				value := *worker.RejectConfig.ExitCode
				reject.ExitCode = &value
			}
			clonedWorker.RejectConfig = &reject
		}
		if worker.GateConfig != nil {
			gate := *worker.GateConfig
			clonedWorker.GateConfig = &gate
		}
		if worker.Usage != nil {
			usage := *worker.Usage
			usage.InputTokens = cloneRuntimeActivationInt64Pointer(worker.Usage.InputTokens)
			usage.OutputTokens = cloneRuntimeActivationInt64Pointer(worker.Usage.OutputTokens)
			usage.CachedInputTokens = cloneRuntimeActivationInt64Pointer(worker.Usage.CachedInputTokens)
			usage.ReasoningOutputTokens = cloneRuntimeActivationInt64Pointer(worker.Usage.ReasoningOutputTokens)
			clonedWorker.Usage = &usage
		}
		mock.MockWorkers[index] = clonedWorker
	}
	cloned.Workers.MockWorkers = mock
	return cloned, nil
}

func cloneRuntimeActivationInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}

// The activation boundary detaches the selected projection without changing
// persisted formats. ReplayArtifact's in-memory metadata is carried separately
// because its JSON representation deliberately omits those fields.
func cloneRuntimeActivationRecovery(input RuntimeActivationRecoveryInput) (RuntimeActivationRecoveryInput, error) {
	if input.WorldState == nil && input.EventHistory == nil && input.ReplayArtifact == nil {
		return input, nil
	}
	type recoveryValues struct {
		Recovery    RuntimeActivationRecoveryInput
		Factory     *factorydefinitions.FactorySnapshot
		Diagnostics factorydefinitions.ReplayDiagnostics
		WallClock   *factorydefinitions.ReplayWallClockMetadata
	}
	values := recoveryValues{Recovery: input}
	if input.ReplayArtifact != nil {
		values.Factory = input.ReplayArtifact.Factory
		values.Diagnostics = input.ReplayArtifact.Diagnostics
		values.WallClock = input.ReplayArtifact.WallClock
	}
	data, err := json.Marshal(values)
	if err != nil {
		return RuntimeActivationRecoveryInput{}, fmt.Errorf("detach runtime recovery input: %w", err)
	}
	var cloned recoveryValues
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(&cloned); err != nil {
		return RuntimeActivationRecoveryInput{}, fmt.Errorf("detach runtime recovery input: %w", err)
	}
	if cloned.Recovery.ReplayArtifact != nil {
		cloned.Recovery.ReplayArtifact.Factory = cloned.Factory
		cloned.Recovery.ReplayArtifact.Diagnostics = cloned.Diagnostics
		cloned.Recovery.ReplayArtifact.WallClock = cloned.WallClock
	}
	return cloned.Recovery, nil
}
