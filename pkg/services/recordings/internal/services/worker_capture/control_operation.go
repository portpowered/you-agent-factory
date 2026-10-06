package worker_capture

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
)

var (
	ErrWorkerControlConflict         = errors.New("recordings: Worker control operation conflict")
	ErrInvalidWorkerControlOperation = errors.New("recordings: invalid Worker control operation")
	controlDigest                    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

// WorkerControlTarget freezes capture and execution authority; it never contains
// a live handle. Replaying a value does not grant authority to execute it.
type WorkerControlTarget struct {
	RecordingID           string `json:"recordingId"`
	WorkerSessionID       string `json:"workerSessionId"`
	FactorySessionID      string `json:"factorySessionId,omitempty"`
	RecordingGenerationID string `json:"recordingGenerationId"`
	OwnerEpoch            string `json:"ownerEpoch"`
	ExpectedAttemptID     string `json:"expectedAttemptId"`
}

type WorkerControlOperation struct {
	Version                  int    `json:"version"`
	RequestID                string `json:"requestId"`
	WorkerSessionID          string `json:"workerSessionId"`
	ExpectedAttemptID        string `json:"expectedAttemptId"`
	Action                   string `json:"action"`
	Phase                    string `json:"phase"`
	InputDigest              string `json:"inputDigest"`
	SuccessorWorkerSessionID string `json:"successorWorkerSessionId,omitempty"`
	ResumeMode               string `json:"resumeMode,omitempty"`
}

type WorkerControlOperationKey struct {
	RecordingID      string `json:"recordingId"`
	WorkerSessionID  string `json:"workerSessionId"`
	FactorySessionID string `json:"factorySessionId,omitempty"`
	RequestID        string `json:"requestId"`
}

// WorkerControlOperationRecord contains only detached, safely encoded snapshots.
// Result is interpreted by Worker Sessions, not by the recording journal.
type WorkerControlOperationRecord struct {
	Target           WorkerControlTarget    `json:"target"`
	Revision         uint64                 `json:"revision"`
	Operation        WorkerControlOperation `json:"operation"`
	InputArtifactRef string                 `json:"inputArtifactRef,omitempty"`
	Result           json.RawMessage        `json:"result,omitempty"`
	FailureCode      string                 `json:"failureCode,omitempty"`
}

func (record WorkerControlOperationRecord) Validate() error {
	t, op := record.Target, record.Operation
	if !t.valid() {
		return ErrInvalidWorkerControlOperation
	}
	if op.Version != 1 || record.Revision == 0 || op.RequestID == "" || op.WorkerSessionID != t.WorkerSessionID || op.ExpectedAttemptID != t.ExpectedAttemptID {
		return ErrInvalidWorkerControlOperation
	}
	if err := op.validateSemantics(); err != nil {
		return err
	}
	return record.validateSnapshot()
}

func (target WorkerControlTarget) valid() bool {
	return target.RecordingID != "" && target.WorkerSessionID != "" && target.RecordingGenerationID != "" && target.OwnerEpoch != "" && target.ExpectedAttemptID != ""
}

func (op WorkerControlOperation) validateSemantics() error {
	if !controlDigest.MatchString(op.InputDigest) || !slices.Contains([]string{"cancel", "terminate", "kill", "interrupt"}, op.Action) {
		return ErrInvalidWorkerControlOperation
	}
	if !slices.Contains([]string{"INTENT", "SOURCE_STOPPED", "SUCCESSOR_ADMITTED", "COMPLETED", "FAILED"}, op.Phase) {
		return ErrInvalidWorkerControlOperation
	}
	if op.ResumeMode != "" && op.ResumeMode != "provider" && op.ResumeMode != "recorded" {
		return ErrInvalidWorkerControlOperation
	}
	if op.Action == "interrupt" && (op.SuccessorWorkerSessionID == "" || op.ResumeMode == "") {
		return ErrInvalidWorkerControlOperation
	}
	return nil
}

func (record WorkerControlOperationRecord) validateSnapshot() error {
	op := record.Operation
	if len(record.Result) > 1<<20 || (len(record.Result) > 0 && (!json.Valid(record.Result) || bytes.TrimSpace(record.Result)[0] != '{')) {
		return ErrInvalidWorkerControlOperation
	}
	if (op.Phase == "COMPLETED" || op.Phase == "FAILED") && len(record.Result) == 0 {
		return ErrInvalidWorkerControlOperation
	}
	if op.Phase == "FAILED" && record.FailureCode == "" {
		return ErrInvalidWorkerControlOperation
	}
	if op.Action == "interrupt" && record.InputArtifactRef == "" {
		return ErrInvalidWorkerControlOperation
	}
	return nil
}

// ValidateIntent adds the initial-phase requirements to the envelope contract.
func (record WorkerControlOperationRecord) ValidateIntent() error {
	if err := record.Validate(); err != nil {
		return err
	}
	if record.Revision != 1 || record.Operation.Phase != "INTENT" || len(record.Result) != 0 || record.FailureCode != "" {
		return ErrInvalidWorkerControlOperation
	}
	return nil
}

// SameIntent compares the immutable request, including the reserved successor.
func (record WorkerControlOperationRecord) SameIntent(other WorkerControlOperationRecord) bool {
	op, candidate := record.Operation, other.Operation
	op.Phase, candidate.Phase = "", ""
	return record.Target == other.Target && op == candidate && record.InputArtifactRef == other.InputArtifactRef
}

// CanAdvance keeps completed/failed snapshots absorbing and admission staged.
func (record WorkerControlOperationRecord) CanAdvance(next WorkerControlOperationRecord, expected uint64) bool {
	if !record.SameIntent(next) || record.Revision != expected || expected == ^uint64(0) || next.Revision != expected+1 {
		return false
	}
	switch record.Operation.Phase {
	case "INTENT":
		return slices.Contains([]string{"SOURCE_STOPPED", "COMPLETED", "FAILED"}, next.Operation.Phase)
	case "SOURCE_STOPPED":
		return next.Operation.Phase == "FAILED" || next.Operation.Phase == "COMPLETED" || (record.Operation.Action == "interrupt" && next.Operation.Phase == "SUCCESSOR_ADMITTED")
	case "SUCCESSOR_ADMITTED":
		return next.Operation.Phase == "COMPLETED" || next.Operation.Phase == "FAILED"
	default:
		return false
	}
}

func (record WorkerControlOperationRecord) Detached() WorkerControlOperationRecord {
	record.Result = slices.Clone(record.Result)
	return record
}
