package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrConflictingWorkRequestTraceFields reports conflicting current and legacy
// trace identities on one Work request.
var ErrConflictingWorkRequestTraceFields = errors.New("currentChainingTraceId and traceId must match when both are provided")

// PreparedFactoryRequestBatch is a detached, validated canonical batch and
// its original representation bytes. The byte slice is copied before it is
// returned so transports cannot mutate Work-owned preparation state.
type PreparedFactoryRequestBatch struct {
	Request       WorkRequest
	CanonicalJSON []byte
}

// FactoryRequestBatchPreparation is the exact Work-owned decode and admission
// role consumed by batch transports.
type FactoryRequestBatchPreparation interface {
	PrepareFactoryRequestBatch(context.Context, []byte) (PreparedFactoryRequestBatch, error)
}

type factoryRequestBatchPreparation struct{}

// NewFactoryRequestBatchPreparation constructs the pure canonical batch
// preparation role. Wire injects it at transport composition.
func NewFactoryRequestBatchPreparation() FactoryRequestBatchPreparation {
	return factoryRequestBatchPreparation{}
}

func (factoryRequestBatchPreparation) PrepareFactoryRequestBatch(
	ctx context.Context,
	data []byte,
) (PreparedFactoryRequestBatch, error) {
	if ctx == nil {
		return PreparedFactoryRequestBatch{}, errors.New("Factory Request Batch preparation context is required")
	}
	if err := ctx.Err(); err != nil {
		return PreparedFactoryRequestBatch{}, err
	}
	request, err := ParseCanonicalWorkRequestJSON(data)
	if err != nil {
		return PreparedFactoryRequestBatch{}, err
	}
	if request.Type != WorkRequestTypeFactoryRequestBatch {
		return PreparedFactoryRequestBatch{}, fmt.Errorf("batch type must be %q", WorkRequestTypeFactoryRequestBatch)
	}
	if strings.TrimSpace(request.RequestID) == "" {
		return PreparedFactoryRequestBatch{}, errors.New("batch requestId is required")
	}
	if len(request.Works) == 0 {
		return PreparedFactoryRequestBatch{}, errors.New("batch works must contain at least one item")
	}
	return PreparedFactoryRequestBatch{
		Request:       request,
		CanonicalJSON: append([]byte(nil), data...),
	}, nil
}

type canonicalWorkRequestJSON struct {
	WorkTypeID             json.RawMessage             `json:"work_type_id"`
	TargetState            json.RawMessage             `json:"target_state"`
	CurrentChainingTraceID json.RawMessage             `json:"currentChainingTraceId"`
	LegacyCurrentChaining  json.RawMessage             `json:"current_chaining_trace_id"`
	TraceID                json.RawMessage             `json:"traceId"`
	LegacyTraceID          json.RawMessage             `json:"trace_id"`
	Works                  []canonicalWorkRequestEntry `json:"works"`
}

type canonicalWorkRequestEntry struct {
	WorkTypeID             json.RawMessage `json:"work_type_id"`
	TargetState            json.RawMessage `json:"target_state"`
	CurrentChainingTraceID json.RawMessage `json:"currentChainingTraceId"`
	LegacyCurrentChaining  json.RawMessage `json:"current_chaining_trace_id"`
	TraceID                json.RawMessage `json:"traceId"`
	LegacyTraceID          json.RawMessage `json:"trace_id"`
}

// ParseCanonicalWorkRequestJSON parses the public Work request contract and
// rejects retired field aliases at the Work service boundary.
func ParseCanonicalWorkRequestJSON(data []byte) (WorkRequest, error) {
	if err := ValidateCanonicalWorkRequestJSON(data); err != nil {
		return WorkRequest{}, err
	}
	var request WorkRequest
	if err := json.Unmarshal(data, &request); err != nil {
		return WorkRequest{}, err
	}
	return request, nil
}

// ValidateCanonicalWorkRequestJSON validates public Work request field names
// and trace aliases without constructing runtime state.
func ValidateCanonicalWorkRequestJSON(data []byte) error {
	var raw canonicalWorkRequestJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := validateRetiredWorkRequestFieldAliases(raw); err != nil {
		return err
	}
	return validateConflictingWorkRequestTraceFields(raw)
}

// ResolveWorkRequestCurrentChainingTraceID returns the effective chaining
// trace, preferring the current field while preserving traceId fallback.
func ResolveWorkRequestCurrentChainingTraceID(current, legacy string) string {
	if current != "" {
		return current
	}
	return legacy
}

// ValidateWorkRequestTraceFields rejects conflicting current and legacy trace
// values when both are populated.
func ValidateWorkRequestTraceFields(current, legacy string) error {
	if current != "" && legacy != "" && current != legacy {
		return ErrConflictingWorkRequestTraceFields
	}
	return nil
}

// ValidateWorkRequestTraceFieldAliases validates the public and retired JSON
// spellings before the request is decoded.
func ValidateWorkRequestTraceFieldAliases(currentRaw, legacyCurrentRaw, traceRaw, legacyTraceRaw json.RawMessage) error {
	if currentRaw == nil {
		currentRaw = legacyCurrentRaw
	}
	if traceRaw == nil {
		traceRaw = legacyTraceRaw
	}
	if currentRaw == nil || traceRaw == nil {
		return nil
	}
	var current string
	if err := json.Unmarshal(currentRaw, &current); err != nil {
		return fmt.Errorf("parse currentChainingTraceId: %w", err)
	}
	var legacy string
	if err := json.Unmarshal(traceRaw, &legacy); err != nil {
		return fmt.Errorf("parse traceId: %w", err)
	}
	return ValidateWorkRequestTraceFields(current, legacy)
}

// WorkRequestFromSubmitRequests projects normalized submissions into the
// canonical FACTORY_REQUEST_BATCH contract.
func WorkRequestFromSubmitRequests(requests []SubmitRequest) WorkRequest {
	if len(requests) == 0 {
		return WorkRequest{Type: WorkRequestTypeFactoryRequestBatch}
	}

	requestID := sharedSubmitRequestID(requests)
	usedNames := make(map[string]int, len(requests))
	works := make([]Work, 0, len(requests))
	for i, request := range requests {
		itemRequestID := request.RequestID
		if itemRequestID == "" {
			itemRequestID = requestID
		}
		works = append(works, Work{
			Name:                     uniqueSubmitWorkName(request, i, usedNames),
			WorkID:                   request.WorkID,
			RequestID:                itemRequestID,
			WorkTypeID:               request.WorkTypeID,
			State:                    request.TargetState,
			ChainingTraceDepth:       request.ChainingTraceDepth,
			CurrentChainingTraceID:   ResolveWorkRequestCurrentChainingTraceID(request.CurrentChainingTraceID, request.TraceID),
			PreviousChainingTraceIDs: append([]string(nil), request.PreviousChainingTraceIDs...),
			TraceID:                  request.TraceID,
			Content:                  CloneWorkContentParts(request.Content),
			Payload:                  ClonePayload(request.Payload),
			Tags:                     cloneNonEmptyTags(request.Tags),
			ExecutionID:              request.ExecutionID,
			RuntimeRelations:         cloneNonEmptyRelations(request.Relations),
			InvocationArguments:      CloneInvocationArguments(request.InvocationArguments),
		})
	}
	return WorkRequest{
		RequestID:              requestID,
		CurrentChainingTraceID: ResolveWorkRequestCurrentChainingTraceID(requests[0].CurrentChainingTraceID, requests[0].TraceID),
		Type:                   WorkRequestTypeFactoryRequestBatch,
		Works:                  works,
	}
}

func validateRetiredWorkRequestFieldAliases(raw canonicalWorkRequestJSON) error {
	if raw.WorkTypeID != nil {
		return fmt.Errorf("work_type_id is not supported; use workTypeName")
	}
	if raw.TargetState != nil {
		return fmt.Errorf("target_state is not supported; use state")
	}
	for i := range raw.Works {
		if raw.Works[i].WorkTypeID != nil {
			return fmt.Errorf("works[%d].work_type_id is not supported; use workTypeName", i)
		}
		if raw.Works[i].TargetState != nil {
			return fmt.Errorf("works[%d].target_state is not supported; use state", i)
		}
	}
	return nil
}

func validateConflictingWorkRequestTraceFields(raw canonicalWorkRequestJSON) error {
	if err := ValidateWorkRequestTraceFieldAliases(raw.CurrentChainingTraceID, raw.LegacyCurrentChaining, raw.TraceID, raw.LegacyTraceID); err != nil {
		return err
	}
	for i := range raw.Works {
		entry := raw.Works[i]
		if err := ValidateWorkRequestTraceFieldAliases(entry.CurrentChainingTraceID, entry.LegacyCurrentChaining, entry.TraceID, entry.LegacyTraceID); err != nil {
			return fmt.Errorf("works[%d].%w", i, err)
		}
	}
	return nil
}

func sharedSubmitRequestID(requests []SubmitRequest) string {
	var shared string
	for _, request := range requests {
		if request.RequestID == "" {
			continue
		}
		if shared == "" {
			shared = request.RequestID
			continue
		}
		if shared != request.RequestID {
			return ""
		}
	}
	return shared
}

func uniqueSubmitWorkName(request SubmitRequest, index int, used map[string]int) string {
	base := request.Name
	if base == "" && request.Tags != nil {
		base = request.Tags["_work_name"]
	}
	if base == "" {
		base = request.WorkID
	}
	if base == "" {
		base = fmt.Sprintf("work-%d", index+1)
	}
	count := used[base]
	used[base] = count + 1
	if count == 0 {
		return base
	}
	return fmt.Sprintf("%s-%d", base, count+1)
}

func cloneNonEmptyTags(tags map[string]string) map[string]string {
	if len(tags) == 0 {
		return nil
	}
	return CloneTags(tags)
}

func cloneNonEmptyRelations(relations []Relation) []Relation {
	if len(relations) == 0 {
		return nil
	}
	return CloneRelations(relations)
}

// WorkRequestSubmitResultFromNormalized builds accepted-request metadata from
// normalized submit requests, including per-work identifiers for batch upserts
// and primary-work fields for unary submit responses.
func WorkRequestSubmitResultFromNormalized(requestID string, work []SubmitRequest, accepted bool) WorkRequestSubmitResult {
	result := WorkRequestSubmitResult{
		RequestID: requestID,
		Accepted:  accepted,
	}
	if len(work) == 0 {
		return result
	}
	primary := work[0]
	result.TraceID = primary.TraceID
	result.WorkID = primary.WorkID
	result.Name = primary.Name
	result.WorkTypeName = primary.WorkTypeID
	works := make([]WorkRequestSubmittedWork, 0, len(work))
	for _, req := range work {
		works = append(works, WorkRequestSubmittedWork{
			Name:         SubmitWorkName(req),
			WorkTypeName: req.WorkTypeID,
			WorkID:       req.WorkID,
		})
	}
	result.Works = works
	return result
}
