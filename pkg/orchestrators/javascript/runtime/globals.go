package workflowruntime

import (
	"fmt"

	"github.com/dop251/goja"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/policy"
)

type runtimeGlobals struct {
	vm           *goja.Runtime
	policy       workflowpolicy.EffectivePolicy
	finalValue   goja.Value
	finalSet     bool
	returned     goja.Value
	returnedSet  bool
}

func (g *runtimeGlobals) bindArgs(argsValue goja.Value) {
	g.vm.Set("args", argsValue)
}

func (g *runtimeGlobals) bindMeta(meta map[string]any) {
	g.vm.Set("meta", g.vm.ToValue(meta))
}

func (g *runtimeGlobals) bindWorkflowAPI() error {
	workflow := g.vm.NewObject()
	if err := workflow.Set("final", g.workflowFinal); err != nil {
		return fmt.Errorf("bind workflow.final: %w", err)
	}
	return g.vm.Set("workflow", workflow)
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
