package workflowruntime

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/dop251/goja"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
)

// runtimeGlobals binds the narrow default workflow surface: structured args, meta,
// progress/state host primitives, and workflow.final. Host filesystem, process,
// network, and shell globals are not injected; forbidden host access is rejected
// before execution.
type runtimeGlobals struct {
	vm                    *goja.Runtime
	policy                workflowpolicy.EffectivePolicy
	sessionID             string
	ctx                   context.Context
	records               *recordCollector
	childExecutor         ChildExecutor
	agents                map[string]interfaces.FactoryOrchestratorJavaScriptAgent
	workerSettings        WorkerSettingsConfig
	onArtifact            func(kind string, content json.RawMessage) error
	resumeCheckpointState map[string]any
	finalValue            goja.Value
	finalSet              bool
	returned              goja.Value
	returnedSet           bool
}

func (g *runtimeGlobals) bindArgs(argsValue goja.Value) {
	g.vm.Set("args", argsValue)
}

func (g *runtimeGlobals) bindMeta(meta map[string]any) {
	g.vm.Set("meta", g.vm.ToValue(meta))
}

func (g *runtimeGlobals) bindResumeCheckpointState(resume *ResumeContext) {
	if resume == nil {
		return
	}
	g.resumeCheckpointState = cloneJSONMap(resume.CheckpointState)
}

func (g *runtimeGlobals) bindWorkflowAPI() error {
	workflow := g.vm.NewObject()
	if err := workflow.Set("final", g.workflowFinal); err != nil {
		return fmt.Errorf("bind workflow.final: %w", err)
	}
	if err := g.bindExtendedWorkflowAPI(workflow); err != nil {
		return err
	}
	if err := g.vm.Set("workflow", workflow); err != nil {
		return fmt.Errorf("bind workflow: %w", err)
	}
	if err := g.bindAgentAPI(); err != nil {
		return err
	}
	if err := g.bindParallelAPI(); err != nil {
		return err
	}
	if err := g.bindPipelineAPI(); err != nil {
		return err
	}
	return g.bindHostPrimitives()
}

func (g *runtimeGlobals) workflowFinal(call goja.FunctionCall) goja.Value {
	if len(call.Arguments) > 0 {
		g.finalValue = call.Arguments[0]
		g.finalSet = true
	}
	return goja.Undefined()
}

func (g *runtimeGlobals) captureReturn(value goja.Value) {
	if value == nil || goja.IsUndefined(value) {
		return
	}
	g.returned = value
	g.returnedSet = true
}

// terminalValue returns the workflow terminal value when one is present.
// When both workflow.final and a returned value exist, workflow.final wins.
func (g *runtimeGlobals) terminalValue() (goja.Value, bool) {
	if g.finalSet {
		return g.finalValue, true
	}
	if g.returnedSet {
		return g.returned, true
	}
	return nil, false
}

func metaFromRequest(metadata map[string]string) map[string]any {
	meta := make(map[string]any, len(metadata))
	for key, value := range metadata {
		meta[key] = value
	}
	if _, ok := meta["name"]; !ok {
		meta["name"] = "simple-final"
	}
	return meta
}
