package workflowruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// ChildExecutionRequest is the typed child-agent request shared by host primitives
// and future dispatch bridges.
type ChildExecutionRequest struct {
	Prompt           string
	Label            string
	AgentID          string
	Preset           string
	Model            string
	ReasoningEffort  string
	Command          string
	Sandbox          string
	WritableRoots    []string
	AllowNetwork     bool
	Concurrency      int
	OutputSchema     map[string]any
	WorkflowName     string
	ArgsSubject      string
	ReservedIdentity *ChildDispatchIdentity
}

// ChildDispatchIdentity reserves stable dispatch metadata before concurrent execution.
type ChildDispatchIdentity struct {
	DispatchID string
	ChildIndex int
}

// ChildExecutionResult is the typed child-agent result returned to host primitives.
type ChildExecutionResult struct {
	DispatchID         string
	ChildIndex         int
	Status             string
	ExecutionMode      string
	Diagnostic         string
	Output             map[string]any
	ArtifactRef        string
	ProviderSessionRef string
	Request            ChildExecutionRequest
}

// ChildExecutor executes one child-agent request and appends dispatch-like records.
type ChildExecutor interface {
	Execute(ctx context.Context, req ChildExecutionRequest) (ChildExecutionResult, error)
}

// FakeChildExecutor provides deterministic fake child execution for workflow tests.
type FakeChildExecutor struct {
	sessionID string
	records   ChildRecordSink
}

// NewFakeChildExecutor constructs one fake child executor for a workflow session.
func NewFakeChildExecutor(sessionID string, records ChildRecordSink) *FakeChildExecutor {
	return &FakeChildExecutor{
		sessionID: sessionID,
		records:   records,
	}
}

// Execute records queued, running, and completed child dispatch transitions and
// returns a deterministic fake child result derived from the request.
func (e *FakeChildExecutor) Execute(ctx context.Context, req ChildExecutionRequest) (ChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ChildExecutionResult{}, err
	}
	if strings.HasPrefix(req.Prompt, "fail:") {
		return e.executeFailed(ctx, req)
	}

	dispatchID, childIndex := e.childDispatchIdentity(req)
	providerSessionRef := fmt.Sprintf("fake-provider-session-%d", childIndex)
	artifactID := e.records.NextChildArtifactID()
	artifactRef := workflowresult.FormatArtifactURI(e.sessionID, artifactID)

	base := ChildDispatchRecord{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Label:              req.Label,
		PromptDigest:       textDigest(req.Prompt),
		Model:              req.Model,
		ReasoningEffort:    req.ReasoningEffort,
		Command:            req.Command,
		Sandbox:            req.Sandbox,
		SchemaDigest:       schemaDigest(req.OutputSchema),
		ExecutionMode:      ChildExecutionModeFake,
		ProviderSessionRef: providerSessionRef,
		ArtifactRef:        artifactRef,
	}

	e.records.AppendChildDispatch(base, ChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, ChildDispatchStatusRunning)
	output := fakeChildOutput(req, dispatchID, providerSessionRef, artifactRef)
	completed := base
	completed.Status = ChildDispatchStatusCompleted
	completed.Output = CloneOutputMap(output)
	e.records.Append(RuntimeRecord{
		Kind:          RecordKindChildDispatch,
		ChildDispatch: &completed,
	})
	return ChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             ChildDispatchStatusCompleted,
		ExecutionMode:      ChildExecutionModeFake,
		Output:             output,
		ArtifactRef:        artifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *FakeChildExecutor) executeFailed(ctx context.Context, req ChildExecutionRequest) (ChildExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return ChildExecutionResult{}, err
	}
	dispatchID, childIndex := e.childDispatchIdentity(req)
	base := ChildDispatchRecord{
		DispatchID:      dispatchID,
		ChildIndex:      childIndex,
		Label:           req.Label,
		PromptDigest:    textDigest(req.Prompt),
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		SchemaDigest:    schemaDigest(req.OutputSchema),
		ExecutionMode:   ChildExecutionModeFake,
	}
	e.records.AppendChildDispatch(base, ChildDispatchStatusQueued)
	e.records.AppendChildDispatch(base, ChildDispatchStatusRunning)
	failed := base
	failed.Status = ChildDispatchStatusFailed
	diagnostic := fmt.Sprintf("fake child failed: %s", strings.TrimPrefix(req.Prompt, "fail:"))
	failed.FailureDetail = &interfaces.FailureDetail{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: diagnostic,
	}
	e.records.Append(RuntimeRecord{
		Kind:          RecordKindChildDispatch,
		ChildDispatch: &failed,
	})
	return ChildExecutionResult{
		DispatchID:    dispatchID,
		ChildIndex:    childIndex,
		Status:        ChildDispatchStatusFailed,
		ExecutionMode: ChildExecutionModeFake,
		Diagnostic:    diagnostic,
		Request:       req,
	}, fmt.Errorf("%s", diagnostic)
}

func (e *FakeChildExecutor) childDispatchIdentity(req ChildExecutionRequest) (string, int) {
	if req.ReservedIdentity != nil {
		return req.ReservedIdentity.DispatchID, req.ReservedIdentity.ChildIndex
	}
	return e.records.NextChildDispatchIdentity()
}

func fakeChildOutput(req ChildExecutionRequest, dispatchID, providerSessionRef, artifactRef string) map[string]any {
	text := fmt.Sprintf(
		"fake:%s:%s:%s:%s",
		req.WorkflowName,
		req.Label,
		req.Prompt,
		req.ArgsSubject,
	)
	return map[string]any{
		"text":            text,
		"subject":         req.ArgsSubject,
		"schemaValidated": req.OutputSchema != nil,
	}
}

func childExecutionRequestFromSpec(spec map[string]any, workflowName, argsSubject string, agents map[string]interfaces.FactoryOrchestratorJavaScriptAgent) (ChildExecutionRequest, error) {
	prompt := stringField(spec, "prompt")
	if prompt == "" {
		return ChildExecutionRequest{}, fmt.Errorf(`agent.run() requires a string "prompt" property`)
	}
	outputSchema, err := outputSchemaFromSpec(spec)
	if err != nil {
		return ChildExecutionRequest{}, err
	}
	writableRoots, err := stringSliceField(spec, "writableRoots")
	if err != nil {
		return ChildExecutionRequest{}, err
	}
	allowNetwork, err := boolField(spec, "allowNetwork")
	if err != nil {
		return ChildExecutionRequest{}, err
	}
	if !allowNetwork {
		allowNetwork, err = boolField(spec, "network")
		if err != nil {
			return ChildExecutionRequest{}, err
		}
	}
	concurrency, err := intField(spec, "concurrency")
	if err != nil {
		return ChildExecutionRequest{}, err
	}
	agentID := strings.TrimSpace(stringField(spec, "agentId"))
	preset := strings.TrimSpace(stringField(spec, "preset"))
	if agentID != "" {
		agent, ok := agents[agentID]
		if !ok {
			return ChildExecutionRequest{}, fmt.Errorf(`agent.run() references unknown factory agent %q`, agentID)
		}
		if preset == "" {
			preset = strings.TrimSpace(agent.Preset)
		}
	}
	return ChildExecutionRequest{
		Prompt:          prompt,
		Label:           stringField(spec, "label"),
		AgentID:         agentID,
		Preset:          preset,
		Model:           stringField(spec, "model"),
		ReasoningEffort: stringField(spec, "reasoningEffort"),
		Command:         stringField(spec, "command"),
		Sandbox:         stringField(spec, "sandbox"),
		WritableRoots:   writableRoots,
		AllowNetwork:    allowNetwork,
		Concurrency:     concurrency,
		OutputSchema:    outputSchema,
		WorkflowName:    workflowName,
		ArgsSubject:     argsSubject,
	}, nil
}

func stringSliceField(spec map[string]any, key string) ([]string, error) {
	value, ok := spec[key]
	if !ok || value == nil {
		return nil, nil
	}
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...), nil
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf(`agent.run() requires %q to be an array of strings`, key)
			}
			out = append(out, text)
		}
		return out, nil
	default:
		return nil, fmt.Errorf(`agent.run() requires %q to be an array of strings`, key)
	}
}

func boolField(spec map[string]any, key string) (bool, error) {
	value, ok := spec[key]
	if !ok || value == nil {
		return false, nil
	}
	allowed, ok := value.(bool)
	if !ok {
		return false, fmt.Errorf(`agent.run() requires %q to be a boolean`, key)
	}
	return allowed, nil
}

func intField(spec map[string]any, key string) (int, error) {
	value, ok := spec[key]
	if !ok || value == nil {
		return 0, nil
	}
	switch typed := value.(type) {
	case int:
		return typed, nil
	case int32:
		return int(typed), nil
	case int64:
		return int(typed), nil
	case float32:
		return int(typed), nil
	case float64:
		return int(typed), nil
	default:
		return 0, fmt.Errorf(`agent.run() requires %q to be a number`, key)
	}
}

func outputSchemaFromSpec(spec map[string]any) (map[string]any, error) {
	if schema, ok := spec["outputSchema"]; ok && schema != nil {
		return exportJSONMap(schema)
	}
	if schema, ok := spec["schema"]; ok && schema != nil {
		return exportJSONMap(schema)
	}
	return nil, nil
}

func schemaDigest(schema map[string]any) string {
	if schema == nil {
		return ""
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		return ""
	}
	return contentDigest(raw)
}

func textDigest(text string) string {
	return contentDigest([]byte(text))
}

// TextDigest returns a stable digest for one child prompt.
func TextDigest(text string) string {
	return textDigest(text)
}

// SchemaDigest returns a stable digest for one child output schema.
func SchemaDigest(schema map[string]any) string {
	return schemaDigest(schema)
}

func failedChildResultValue(label, executionMode string, err error) map[string]any {
	if executionMode == "" {
		executionMode = ChildExecutionModeFake
	}
	value := map[string]any{
		"status":        ChildDispatchStatusFailed,
		"executionMode": executionMode,
		"diagnostic":    err.Error(),
	}
	if label != "" {
		value["label"] = label
	}
	return value
}

func childResultValueMap(result ChildExecutionResult) map[string]any {
	req := result.Request
	executionMode := result.ExecutionMode
	if executionMode == "" {
		executionMode = ChildExecutionModeFake
	}
	value := map[string]any{
		"status":             result.Status,
		"dispatchId":         result.DispatchID,
		"childIndex":         result.ChildIndex,
		"executionMode":      executionMode,
		"providerSessionRef": result.ProviderSessionRef,
		"promptDigest":       textDigest(req.Prompt),
		"output":             result.Output,
	}
	if result.Diagnostic != "" {
		value["diagnostic"] = result.Diagnostic
	}
	if req.Label != "" {
		value["label"] = req.Label
	}
	if req.Model != "" {
		value["model"] = req.Model
	}
	if req.ReasoningEffort != "" {
		value["reasoningEffort"] = req.ReasoningEffort
	}
	if req.Command != "" {
		value["command"] = req.Command
	}
	if req.Sandbox != "" {
		value["sandbox"] = req.Sandbox
	}
	if digest := schemaDigest(req.OutputSchema); digest != "" {
		value["schemaDigest"] = digest
	}
	if result.ArtifactRef != "" {
		value["artifactRef"] = result.ArtifactRef
	}
	return value
}

type childRecordSink struct {
	records *recordCollector
}

func childRecordSinkFromCollector(records *recordCollector) ChildRecordSink {
	if records == nil {
		return childRecordSink{records: newRecordCollector()}
	}
	return childRecordSink{records: records}
}

func (s childRecordSink) Append(record RuntimeRecord) {
	s.records.append(record)
}

func (s childRecordSink) AppendChildDispatch(base ChildDispatchRecord, status string) {
	record := base
	record.Status = status
	s.Append(RuntimeRecord{
		Kind:          RecordKindChildDispatch,
		ChildDispatch: &record,
	})
}

func (s childRecordSink) NextChildDispatchIdentity() (string, int) {
	return s.records.nextChildDispatchIdentity()
}

func (s childRecordSink) NextChildArtifactID() string {
	return s.records.nextChildArtifactID()
}

// ResumeContext carries durable checkpoint and completed child-dispatch facts used
// to reconstruct one interrupted JavaScript workflow session without rerunning
// already satisfied child work by default.
type ResumeContext struct {
	CompletedDispatchIDs  []string
	CompletedChildResults map[string]ChildExecutionResult
	CheckpointState       map[string]any
}

// ResumingChildExecutor wraps one child executor and replays completed dispatch
// results from resume context instead of re-executing satisfied child work.
type ResumingChildExecutor struct {
	base   ChildExecutor
	resume ResumeContext
	next   int
}

// NewResumingChildExecutor constructs one resume-aware child executor wrapper.
func NewResumingChildExecutor(base ChildExecutor, resume ResumeContext) *ResumingChildExecutor {
	if base == nil {
		base = NewFakeChildExecutor("", childRecordSinkFromCollector(newRecordCollector()))
	}
	next := 0
	if len(resume.CheckpointState) > 0 {
		next = len(resume.CompletedDispatchIDs)
	}
	return &ResumingChildExecutor{
		base:   base,
		resume: resume,
		next:   next,
	}
}

// Execute returns cached completed child results or delegates to the wrapped executor.
func (e *ResumingChildExecutor) Execute(ctx context.Context, req ChildExecutionRequest) (ChildExecutionResult, error) {
	if e == nil {
		return ChildExecutionResult{}, fmt.Errorf("resuming child executor is nil")
	}
	e.next++
	dispatchID := fmt.Sprintf("dispatch-%d", e.next)
	if result, ok := e.resume.CompletedChildResults[dispatchID]; ok {
		return result, nil
	}
	req.ReservedIdentity = &ChildDispatchIdentity{
		DispatchID: dispatchID,
		ChildIndex: e.next,
	}
	return e.base.Execute(ctx, req)
}

// CompletedChildResultsFromRecords builds replayable child execution results from
// the latest completed child-dispatch runtime records.
func CompletedChildResultsFromRecords(records []RuntimeRecord) map[string]ChildExecutionResult {
	latest := make(map[string]ChildDispatchRecord)
	for _, record := range records {
		if record.Kind != RecordKindChildDispatch || record.ChildDispatch == nil {
			continue
		}
		child := *record.ChildDispatch
		if child.Status != ChildDispatchStatusCompleted {
			continue
		}
		latest[child.DispatchID] = child
	}
	if len(latest) == 0 {
		return nil
	}
	out := make(map[string]ChildExecutionResult, len(latest))
	for dispatchID, child := range latest {
		out[dispatchID] = childExecutionResultFromRecord(child)
	}
	return out
}

func childExecutionResultFromRecord(child ChildDispatchRecord) ChildExecutionResult {
	executionMode := child.ExecutionMode
	if executionMode == "" {
		executionMode = ChildExecutionModeFake
	}
	output := CloneOutputMap(child.Output)
	if len(output) == 0 {
		output = syntheticChildOutputFromRecord(child, executionMode)
	}
	return ChildExecutionResult{
		DispatchID:         child.DispatchID,
		ChildIndex:         child.ChildIndex,
		Status:             child.Status,
		ExecutionMode:      executionMode,
		ArtifactRef:        child.ArtifactRef,
		ProviderSessionRef: child.ProviderSessionRef,
		Output:             output,
		Request: ChildExecutionRequest{
			Label: child.Label,
			Model: child.Model,
		},
	}
}

func syntheticChildOutputFromRecord(child ChildDispatchRecord, executionMode string) map[string]any {
	return map[string]any{
		"text": fmt.Sprintf(
			"fake:%s:%s:%s",
			child.Label,
			child.DispatchID,
			childExecutionModeLabel(executionMode),
		),
		"label": child.Label,
	}
}

func CloneOutputMap(output map[string]any) map[string]any {
	if len(output) == 0 {
		return nil
	}
	cloned, err := exportJSONMap(output)
	if err != nil {
		return output
	}
	return cloned
}

func childExecutionModeLabel(mode string) string {
	if mode == "" {
		return ChildExecutionModeFake
	}
	return mode
}

// ResumeContextFromCheckpointSummary builds runtime resume facts from one durable
// checkpoint summary and prior runtime records.
func ResumeContextFromCheckpointSummary(
	summary CompletedCheckpointSummary,
	records []RuntimeRecord,
) ResumeContext {
	completed := CompletedChildResultsFromRecords(records)
	if len(summary.CompletedDispatchIDs) > 0 && completed == nil {
		completed = make(map[string]ChildExecutionResult, len(summary.CompletedDispatchIDs))
	}
	for _, dispatchID := range summary.CompletedDispatchIDs {
		if _, ok := completed[dispatchID]; ok {
			continue
		}
		completed[dispatchID] = ChildExecutionResult{
			DispatchID:    dispatchID,
			Status:        ChildDispatchStatusCompleted,
			ExecutionMode: ChildExecutionModeFake,
			Output: map[string]any{
				"text": fmt.Sprintf("replayed:%s", dispatchID),
			},
		}
	}
	return ResumeContext{
		CompletedDispatchIDs:  append([]string(nil), summary.CompletedDispatchIDs...),
		CompletedChildResults: completed,
		CheckpointState:       cloneJSONMap(summary.CheckpointState),
	}
}

// CompletedCheckpointSummary is the minimal checkpoint summary shape consumed by
// the JavaScript runtime resume path.
type CompletedCheckpointSummary struct {
	CompletedDispatchIDs []string
	CheckpointState      map[string]any
}

func cloneJSONMap(value map[string]any) map[string]any {
	if len(value) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
