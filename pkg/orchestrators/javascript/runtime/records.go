package workflowruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const (
	RecordKindPhase         = "phase"
	RecordKindLog           = "log"
	RecordKindArtifact      = "artifact"
	RecordKindCheckpoint    = "checkpoint"
	RecordKindBudget        = "budget"
	RecordKindChildDispatch = "child_dispatch"
)

const (
	ChildDispatchStatusQueued    = "QUEUED"
	ChildDispatchStatusRunning   = "RUNNING"
	ChildDispatchStatusCompleted = "COMPLETED"
	ChildDispatchStatusFailed    = "FAILED"
)

// ChildExecutionModeFake marks deterministic in-process child execution used before
// real provider dispatch is available.
const ChildExecutionModeFake = "fake"

// ChildExecutionModeLive marks provider-backed child execution routed through the
// durable dispatch bridge.
const ChildExecutionModeLive = "live-provider"

// ChildExecutionFailureReason is the default durable dispatch failure reason for
// bridged child execution failures.
const ChildExecutionFailureReason = "CHILD_EXECUTION_FAILED"

// RuntimeRecord is one ordered host-effect record emitted during workflow execution.
// Records are typed so they can later map into factory session events, dispatches,
// and artifacts without changing workflow source syntax.
type RuntimeRecord struct {
	Sequence      int                  `json:"sequence"`
	Kind          string               `json:"kind"`
	Phase         *PhaseRecord         `json:"phase,omitempty"`
	Log           *LogRecord           `json:"log,omitempty"`
	Artifact      *ArtifactRecord      `json:"artifact,omitempty"`
	Checkpoint    *CheckpointRecord    `json:"checkpoint,omitempty"`
	Budget        *BudgetRecord        `json:"budget,omitempty"`
	ChildDispatch *ChildDispatchRecord `json:"childDispatch,omitempty"`
}

// UnmarshalJSON rejects unknown or structurally inconsistent durable records.
// Runtime records are an orchestrator-owned stream: accepting a new kind as an
// empty record would silently discard replay data on older binaries.
func (r *RuntimeRecord) UnmarshalJSON(data []byte) error {
	type runtimeRecordJSON RuntimeRecord
	var decoded runtimeRecordJSON
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("decode JavaScript runtime record: %w", err)
	}
	if err := validateRuntimeRecordPayload(RuntimeRecord(decoded)); err != nil {
		return err
	}
	*r = RuntimeRecord(decoded)
	return nil
}

func validateRuntimeRecordPayload(record RuntimeRecord) error {
	payloadPresent := map[string]bool{
		RecordKindPhase:         record.Phase != nil,
		RecordKindLog:           record.Log != nil,
		RecordKindArtifact:      record.Artifact != nil,
		RecordKindCheckpoint:    record.Checkpoint != nil,
		RecordKindBudget:        record.Budget != nil,
		RecordKindChildDispatch: record.ChildDispatch != nil,
	}
	present, known := payloadPresent[record.Kind]
	if !known {
		return fmt.Errorf("decode JavaScript runtime record: unsupported kind %q", record.Kind)
	}
	if !present {
		return fmt.Errorf("decode JavaScript runtime record kind %q: matching payload is required", record.Kind)
	}
	for kind, hasPayload := range payloadPresent {
		if hasPayload && kind != record.Kind {
			return fmt.Errorf("decode JavaScript runtime record kind %q: unexpected %s payload", record.Kind, kind)
		}
	}
	return nil
}

// PhaseRecord captures one workflow phase transition.
type PhaseRecord struct {
	Name string `json:"name"`
}

// LogRecord captures one structured workflow log line.
type LogRecord struct {
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

// ArtifactRecord captures artifact metadata for one workflow.artifact call.
type ArtifactRecord struct {
	ID          string `json:"id"`
	URI         string `json:"uri"`
	Kind        string `json:"kind"`
	Label       string `json:"label"`
	Visibility  string `json:"visibility,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`
	SizeBytes   int64  `json:"sizeBytes,omitempty"`
}

// CheckpointRecord captures checkpoint metadata without raw VM internals.
type CheckpointRecord struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Summary string         `json:"summary"`
	State   map[string]any `json:"state,omitempty"`
}

// ChildDispatchRecord captures dispatch-like child execution metadata for one
// status transition. Multiple records per child prove queued/running/completed
// ordering without starting real provider sessions.
type ChildDispatchRecord struct {
	DispatchID            string                          `json:"dispatchId"`
	ChildIndex            int                             `json:"childIndex"`
	Attempt               int                             `json:"attempt,omitempty"`
	Status                string                          `json:"status"`
	Label                 string                          `json:"label,omitempty"`
	PromptDigest          string                          `json:"promptDigest,omitempty"`
	Preset                string                          `json:"preset,omitempty"`
	ModelProvider         string                          `json:"modelProvider,omitempty"`
	Model                 string                          `json:"model,omitempty"`
	ReasoningEffort       string                          `json:"reasoningEffort,omitempty"`
	Command               string                          `json:"command,omitempty"`
	Sandbox               string                          `json:"sandbox,omitempty"`
	SchemaDigest          string                          `json:"schemaDigest,omitempty"`
	RunnerID              string                          `json:"runnerId,omitempty"`
	ExecutionMode         string                          `json:"executionMode,omitempty"`
	Provider              string                          `json:"provider,omitempty"`
	ProviderSessionRef    string                          `json:"providerSessionRef,omitempty"`
	ArtifactRef           string                          `json:"artifactRef,omitempty"`
	Output                map[string]any                  `json:"output,omitempty"`
	FailureDetail         *workerexecution.FailureDetail  `json:"failureDetail,omitempty"`
	Retryable             *bool                           `json:"retryable,omitempty"`
	FailureClassification workerexecution.WorkFailureType `json:"failureClassification,omitempty"`
}

// BudgetRecord captures effective policy budget values observed by the runtime.
type BudgetRecord struct {
	MaxAgents               int    `json:"maxAgents"`
	Concurrency             int    `json:"concurrency"`
	SandboxMode             string `json:"sandboxMode,omitempty"`
	MaxRunDurationMs        *int64 `json:"maxRunDurationMs,omitempty"`
	MaxWorkerDurationMs     *int64 `json:"maxWorkerDurationMs,omitempty"`
	MaxOutputBytesPerWorker *int64 `json:"maxOutputBytesPerWorker,omitempty"`
	MaxArtifactBytes        *int64 `json:"maxArtifactBytes,omitempty"`
	MaxTokens               *int64 `json:"maxTokens,omitempty"`
}

type recordCollector struct {
	mu                 sync.Mutex
	sequence           int
	records            []RuntimeRecord
	artifactCount      int
	checkpointCount    int
	childDispatchCount int
}

func newRecordCollector() *recordCollector {
	return &recordCollector{}
}

func (c *recordCollector) append(record RuntimeRecord) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sequence++
	record.Sequence = c.sequence
	c.records = append(c.records, record)
}

func (c *recordCollector) list() []RuntimeRecord {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.records) == 0 {
		return nil
	}
	out := make([]RuntimeRecord, len(c.records))
	copy(out, c.records)
	return out
}

func (c *recordCollector) nextArtifactID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.artifactCount++
	return fmt.Sprintf("artifact-%d", c.artifactCount)
}

func (c *recordCollector) nextCheckpointID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkpointCount++
	return fmt.Sprintf("checkpoint-%d", c.checkpointCount)
}

func (c *recordCollector) nextChildDispatchIdentity() (string, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.childDispatchCount++
	index := c.childDispatchCount
	return fmt.Sprintf("dispatch-%d", index), index
}

func (c *recordCollector) childDispatchCountValue() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.childDispatchCount
}

func (c *recordCollector) nextChildArtifactID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return fmt.Sprintf("child-artifact-%d", c.childDispatchCount)
}

func contentDigest(payload []byte) string {
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func checkpointSummary(label string, state map[string]any) string {
	fieldCount := 0
	if state != nil {
		fieldCount = len(state)
	}
	if label != "" {
		return fmt.Sprintf("checkpoint %q with %d state field(s)", label, fieldCount)
	}
	return fmt.Sprintf("checkpoint with %d state field(s)", fieldCount)
}

func exportJSONMap(value any) (map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}
