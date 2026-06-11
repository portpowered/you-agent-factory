package workflowruntime

import (
	"encoding/json"
	"fmt"

	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// ChildExecutionRequest is the typed child-agent request shared by host primitives
// and future dispatch bridges.
type ChildExecutionRequest struct {
	Prompt          string
	Label           string
	Model           string
	ReasoningEffort string
	Command         string
	Sandbox         string
	OutputSchema    map[string]any
	WorkflowName    string
	ArgsSubject     string
}

// ChildExecutionResult is the typed child-agent result returned to host primitives.
type ChildExecutionResult struct {
	DispatchID         string
	ChildIndex         int
	Status             string
	Output             map[string]any
	ArtifactRef        string
	ProviderSessionRef string
	Request            ChildExecutionRequest
}

// ChildExecutor executes one child-agent request and appends dispatch-like records.
type ChildExecutor interface {
	Execute(req ChildExecutionRequest) (ChildExecutionResult, error)
}

// FakeChildExecutor provides deterministic fake child execution for workflow tests.
type FakeChildExecutor struct {
	sessionID string
	records   *recordCollector
}

// NewFakeChildExecutor constructs one fake child executor for a workflow session.
func NewFakeChildExecutor(sessionID string, records *recordCollector) *FakeChildExecutor {
	return &FakeChildExecutor{
		sessionID: sessionID,
		records:   records,
	}
}

// Execute records queued, running, and completed child dispatch transitions and
// returns a deterministic fake child result derived from the request.
func (e *FakeChildExecutor) Execute(req ChildExecutionRequest) (ChildExecutionResult, error) {
	dispatchID := e.records.nextChildDispatchID()
	childIndex := e.records.childDispatchCount
	providerSessionRef := fmt.Sprintf("fake-provider-session-%d", childIndex)
	artifactID := e.records.nextChildArtifactID()
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
		ExecutionMode:      childExecutionModeFake,
		ProviderSessionRef: providerSessionRef,
		ArtifactRef:        artifactRef,
	}

	e.appendChildDispatch(base, ChildDispatchStatusQueued)
	e.appendChildDispatch(base, ChildDispatchStatusRunning)
	completed := base
	completed.Status = ChildDispatchStatusCompleted
	e.records.append(RuntimeRecord{
		Kind:          RecordKindChildDispatch,
		ChildDispatch: &completed,
	})

	output := fakeChildOutput(req, dispatchID, providerSessionRef, artifactRef)
	return ChildExecutionResult{
		DispatchID:         dispatchID,
		ChildIndex:         childIndex,
		Status:             ChildDispatchStatusCompleted,
		Output:             output,
		ArtifactRef:        artifactRef,
		ProviderSessionRef: providerSessionRef,
		Request:            req,
	}, nil
}

func (e *FakeChildExecutor) appendChildDispatch(base ChildDispatchRecord, status string) {
	record := base
	record.Status = status
	e.records.append(RuntimeRecord{
		Kind:          RecordKindChildDispatch,
		ChildDispatch: &record,
	})
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
		"text": text,
		"subject": req.ArgsSubject,
		"schemaValidated": req.OutputSchema != nil,
	}
}

func childExecutionRequestFromSpec(spec map[string]any, workflowName, argsSubject string) (ChildExecutionRequest, error) {
	prompt := stringField(spec, "prompt")
	if prompt == "" {
		return ChildExecutionRequest{}, fmt.Errorf(`agent.run() requires a string "prompt" property`)
	}
	outputSchema, err := outputSchemaFromSpec(spec)
	if err != nil {
		return ChildExecutionRequest{}, err
	}
	return ChildExecutionRequest{
		Prompt:          prompt,
		Label:           stringField(spec, "label"),
		Model:           stringField(spec, "model"),
		ReasoningEffort: stringField(spec, "reasoningEffort"),
		Command:         stringField(spec, "command"),
		Sandbox:         stringField(spec, "sandbox"),
		OutputSchema:    outputSchema,
		WorkflowName:    workflowName,
		ArgsSubject:     argsSubject,
	}, nil
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

func childResultValueMap(result ChildExecutionResult) map[string]any {
	req := result.Request
	value := map[string]any{
		"status":             result.Status,
		"dispatchId":         result.DispatchID,
		"childIndex":         result.ChildIndex,
		"executionMode":      childExecutionModeFake,
		"providerSessionRef": result.ProviderSessionRef,
		"promptDigest":       textDigest(req.Prompt),
		"output":             result.Output,
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
