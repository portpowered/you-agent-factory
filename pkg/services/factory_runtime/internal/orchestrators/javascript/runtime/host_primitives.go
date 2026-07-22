package workflowruntime

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dop251/goja"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime/runtimecontract"
)

func (g *runtimeGlobals) bindHostPrimitives() error {
	if err := g.vm.Set("phase", g.hostPhase); err != nil {
		return fmt.Errorf("bind phase: %w", err)
	}
	if err := g.vm.Set("log", g.hostLog); err != nil {
		return fmt.Errorf("bind log: %w", err)
	}
	return nil
}

func (g *runtimeGlobals) bindExtendedWorkflowAPI(workflow *goja.Object) error {
	members := map[string]func(goja.FunctionCall) goja.Value{
		"log":         g.hostWorkflowLog,
		"artifact":    g.hostWorkflowArtifact,
		"checkpoint":  g.hostWorkflowCheckpoint,
		"resumeState": g.hostWorkflowResumeState,
		"budget":      g.hostWorkflowBudget,
	}
	for name, fn := range members {
		if err := workflow.Set(name, fn); err != nil {
			return fmt.Errorf("bind workflow.%s: %w", name, err)
		}
	}
	return nil
}

func (g *runtimeGlobals) hostPhase(call goja.FunctionCall) goja.Value {
	name, ok := g.requiredStringArg(call, 0, "phase")
	if !ok {
		panic(g.vm.NewTypeError("phase() requires a string name"))
	}
	g.records.append(RuntimeRecord{
		Kind:  RecordKindPhase,
		Phase: &PhaseRecord{Name: name},
	})
	return goja.Undefined()
}

func (g *runtimeGlobals) hostLog(call goja.FunctionCall) goja.Value {
	record, ok := g.logRecordFromCall(call, "log")
	if !ok {
		panic(g.vm.NewTypeError("log() requires a string message"))
	}
	g.records.append(RuntimeRecord{
		Kind: RecordKindLog,
		Log:  record,
	})
	return goja.Undefined()
}

func (g *runtimeGlobals) hostWorkflowLog(call goja.FunctionCall) goja.Value {
	record, ok := g.logRecordFromCall(call, "workflow.log")
	if !ok {
		panic(g.vm.NewTypeError("workflow.log() requires a string message"))
	}
	g.records.append(RuntimeRecord{
		Kind: RecordKindLog,
		Log:  record,
	})
	return goja.Undefined()
}

func (g *runtimeGlobals) hostWorkflowArtifact(call goja.FunctionCall) goja.Value {
	spec, ok := g.requiredObjectArg(call, 0, "workflow.artifact")
	if !ok {
		panic(g.vm.NewTypeError("workflow.artifact() requires an object argument"))
	}
	kind := stringField(spec, "kind")
	label := stringField(spec, "label")
	if kind == "" || label == "" {
		panic(g.vm.NewTypeError(`workflow.artifact() requires string "kind" and "label" properties`))
	}

	content := spec["content"]
	contentRaw, err := json.Marshal(content)
	if err != nil {
		panic(g.vm.NewGoError(fmt.Errorf("workflow.artifact content must be JSON-compatible: %w", err)))
	}
	if err := g.denyArtifactSize(int64(len(contentRaw))); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	if g.onArtifact != nil {
		if err := g.onArtifact(kind, contentRaw); err != nil {
			panic(g.vm.NewGoError(err))
		}
	}

	artifactID := g.records.nextArtifactID()
	uri := workflowresult.FormatArtifactURI(g.sessionID, artifactID)
	visibility := stringField(spec, "visibility")
	if visibility == "" {
		visibility = "WORKFLOW_RUNTIME"
	}

	record := ArtifactRecord{
		ID:          artifactID,
		URI:         uri,
		Kind:        kind,
		Label:       label,
		Visibility:  visibility,
		ContentHash: contentDigest(contentRaw),
		SizeBytes:   int64(len(contentRaw)),
	}
	g.records.append(RuntimeRecord{
		Kind:     RecordKindArtifact,
		Artifact: &record,
	})
	return g.vm.ToValue(uri)
}

func (g *runtimeGlobals) hostWorkflowResumeState(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
		panic(g.vm.NewTypeError("workflow.resumeState() does not accept arguments"))
	}
	if len(g.resumeCheckpointState) == 0 {
		return goja.Undefined()
	}
	return g.vm.ToValue(g.resumeCheckpointState)
}

func (g *runtimeGlobals) hostWorkflowCheckpoint(call goja.FunctionCall) goja.Value {
	spec, ok := g.requiredObjectArg(call, 0, "workflow.checkpoint")
	if !ok {
		panic(g.vm.NewTypeError("workflow.checkpoint() requires an object argument"))
	}
	label := stringField(spec, "label")
	if label == "" {
		panic(g.vm.NewTypeError(`workflow.checkpoint() requires a string "label" property`))
	}

	state, err := exportJSONMap(spec["state"])
	if err != nil {
		panic(g.vm.NewGoError(fmt.Errorf("workflow.checkpoint state must be JSON-compatible: %w", err)))
	}

	checkpointID := g.records.nextCheckpointID()
	checkpoint := CheckpointRecord{
		ID:      checkpointID,
		Label:   label,
		Summary: checkpointSummary(label, state),
		State:   state,
	}
	g.records.append(RuntimeRecord{
		Kind:       RecordKindCheckpoint,
		Checkpoint: &checkpoint,
	})
	return goja.Undefined()
}

func (g *runtimeGlobals) hostWorkflowBudget(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) > 0 && !goja.IsUndefined(call.Arguments[0]) {
		panic(g.vm.NewTypeError("workflow.budget() does not accept arguments"))
	}
	budget := budgetFromPolicy(g.policy)
	g.records.append(RuntimeRecord{
		Kind:   RecordKindBudget,
		Budget: &budget,
	})
	return g.vm.ToValue(budgetValueMap(budget))
}

func (g *runtimeGlobals) logRecordFromCall(call goja.FunctionCall, primitive string) (*LogRecord, bool) {
	message, ok := g.requiredStringArg(call, 0, primitive)
	if !ok {
		return nil, false
	}
	record := &LogRecord{Message: message}
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) {
		fields, err := exportJSONMap(call.Arguments[1].Export())
		if err != nil {
			panic(g.vm.NewGoError(fmt.Errorf("%s fields must be JSON-compatible: %w", primitive, err)))
		}
		record.Fields = fields
	}
	return record, true
}

func budgetFromPolicy(policy workflowpolicy.EffectivePolicy) BudgetRecord {
	return BudgetRecord{
		MaxAgents:               policy.MaxAgents,
		Concurrency:             policy.Concurrency,
		SandboxMode:             policy.SandboxMode,
		MaxRunDurationMs:        policy.MaxRunDurationMs,
		MaxWorkerDurationMs:     policy.MaxWorkerDurationMs,
		MaxOutputBytesPerWorker: policy.MaxOutputBytesPerWorker,
		MaxArtifactBytes:        policy.MaxArtifactBytes,
		MaxTokens:               policy.MaxTokens,
	}
}

func budgetValueMap(budget BudgetRecord) map[string]any {
	value := map[string]any{
		"maxAgents":   budget.MaxAgents,
		"concurrency": budget.Concurrency,
	}
	if budget.SandboxMode != "" {
		value["sandboxMode"] = budget.SandboxMode
	}
	if budget.MaxRunDurationMs != nil {
		value["maxRunDurationMs"] = *budget.MaxRunDurationMs
	}
	if budget.MaxWorkerDurationMs != nil {
		value["maxWorkerDurationMs"] = *budget.MaxWorkerDurationMs
	}
	if budget.MaxOutputBytesPerWorker != nil {
		value["maxOutputBytesPerWorker"] = *budget.MaxOutputBytesPerWorker
	}
	if budget.MaxArtifactBytes != nil {
		value["maxArtifactBytes"] = *budget.MaxArtifactBytes
	}
	if budget.MaxTokens != nil {
		value["maxTokens"] = *budget.MaxTokens
	}
	return value
}

func (g *runtimeGlobals) requiredStringArg(call goja.FunctionCall, index int, primitive string) (string, bool) {
	if len(call.Arguments) <= index || goja.IsUndefined(call.Arguments[index]) {
		return "", false
	}
	exported := call.Arguments[index].Export()
	text, ok := exported.(string)
	if !ok {
		panic(g.vm.NewTypeError(fmt.Sprintf("%s() requires a string argument", primitive)))
	}
	return strings.TrimSpace(text), text != ""
}

func (g *runtimeGlobals) requiredObjectArg(call goja.FunctionCall, index int, primitive string) (map[string]any, bool) {
	if len(call.Arguments) <= index || goja.IsUndefined(call.Arguments[index]) {
		return nil, false
	}
	exported := call.Arguments[index].Export()
	value, ok := exported.(map[string]any)
	if !ok {
		panic(g.vm.NewTypeError(fmt.Sprintf("%s() requires an object argument", primitive)))
	}
	return value, true
}

func stringField(spec map[string]any, key string) string {
	value, ok := spec[key]
	if !ok || value == nil {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
