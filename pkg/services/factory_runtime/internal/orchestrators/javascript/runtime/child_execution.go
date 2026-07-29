package workflowruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtimecontract"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

// ChildExecutionRequest is the typed child-agent request shared by host primitives
// and future dispatch bridges.
type ChildExecutionRequest struct {
	Prompt           string
	Label            string
	AgentID          string
	Preset           string
	ExecutorProvider string
	ModelProvider    string
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

// ResolveChildWorkerSettings deterministically fills each worker field from the
// highest-precedence source that supplies it. It performs no IO or mutation.
func ResolveChildWorkerSettings(req ChildExecutionRequest, agents map[string]interfaces.FactoryOrchestratorJavaScriptAgent, config WorkerSettingsConfig) (ChildExecutionRequest, error) {
	explicitPreset := strings.TrimSpace(req.Preset)
	explicitProvider := strings.TrimSpace(req.ModelProvider)
	explicitEffort := strings.TrimSpace(req.ReasoningEffort)
	factoryPreset := ""
	if agent, ok := agents[strings.TrimSpace(req.AgentID)]; ok {
		factoryPreset = strings.TrimSpace(agent.Preset)
	}
	selectedPreset, source := explicitPreset, "agent.run"
	if selectedPreset == "" {
		selectedPreset, source = factoryPreset, "factory agent"
	}
	preset := WorkerPreset{}
	if selectedPreset != "" {
		var ok bool
		preset, ok = config.Presets[selectedPreset]
		if !ok && source == "factory agent" {
			return ChildExecutionRequest{}, fmt.Errorf("agent.run() references unknown operator worker preset %q from %s", selectedPreset, source)
		}
	}
	req.Preset = selectedPreset
	req.ModelProvider = firstWorkerValue(req.ModelProvider, preset.ModelProvider, config.DefaultModelProvider)
	req.Model = firstWorkerValue(req.Model, preset.Model, config.DefaultModel)
	req.ReasoningEffort = firstWorkerValue(req.ReasoningEffort, preset.ReasoningEffort)
	if strings.EqualFold(strings.TrimSpace(req.ExecutorProvider), workerexecution.ExecutorProviderACP) {
		if _, err := workerexecution.RunnerIdentityForWorker(req.ExecutorProvider, req.ModelProvider); err != nil {
			return ChildExecutionRequest{}, fmt.Errorf("agent.run() has invalid ACP provider selection: %w", err)
		}
		req.ExecutorProvider = workerexecution.ExecutorProviderACP
		req.ModelProvider = strings.TrimSpace(req.ModelProvider)
	} else if provider, ok := interfaces.CanonicalizeOperatorWorkerModelProviderInput(req.ModelProvider); req.ModelProvider != "" {
		if !ok || interfaces.IsSymbolicWorkerModelProviderDefault(provider) {
			return ChildExecutionRequest{}, fmt.Errorf("agent.run() has unsupported effective modelProvider %q", req.ModelProvider)
		}
		if explicitProvider == "" {
			req.ModelProvider = provider
		}
	}
	if effort, ok := interfaces.CanonicalizeReasoningEffort(req.ReasoningEffort); req.ReasoningEffort != "" {
		if !ok {
			return ChildExecutionRequest{}, fmt.Errorf("agent.run() has unsupported effective reasoningEffort %q", req.ReasoningEffort)
		}
		if explicitEffort == "" {
			req.ReasoningEffort = effort
		}
	}
	return req, nil
}

func firstWorkerValue(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
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
		Preset:             req.Preset,
		ModelProvider:      req.ModelProvider,
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
		Preset:          req.Preset,
		ModelProvider:   req.ModelProvider,
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
	failed.FailureDetail = &workerexecution.FailureDetail{
		Reason:  workerexecution.WorkFailureTypeUnknown,
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
	// Factory agent definitions remain runtime configuration; they do not widen
	// the intentionally small per-child argument contract.
	_ = agents
	normalized, err := orchestratorcontract.NormalizeJavaScriptChild(spec)
	if err != nil {
		return ChildExecutionRequest{}, err
	}
	return ChildExecutionRequest{
		Prompt:           normalized.Prompt,
		Label:            normalized.Label,
		Preset:           normalized.Preset,
		ExecutorProvider: normalized.ExecutorProvider,
		ModelProvider:    normalized.ModelProvider,
		Model:            normalized.Model,
		ReasoningEffort:  normalized.ReasoningEffort,
		WorkflowName:     workflowName,
		ArgsSubject:      argsSubject,
	}, nil
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
