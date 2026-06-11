package workflowruntime

import (
	"fmt"

	"github.com/dop251/goja"
)

func (g *runtimeGlobals) bindAgentAPI() error {
	agent := g.vm.NewObject()
	if err := agent.Set("run", g.hostAgentRun); err != nil {
		return fmt.Errorf("bind agent.run: %w", err)
	}
	return g.vm.Set("agent", agent)
}

func (g *runtimeGlobals) hostAgentRun(call goja.FunctionCall) goja.Value {
	spec, ok := g.requiredObjectArg(call, 0, "agent.run")
	if !ok {
		panic(g.vm.NewTypeError("agent.run() requires an object argument"))
	}
	req, err := childExecutionRequestFromSpec(spec, g.workflowName(), g.argsSubject())
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	if err := g.denyChildSlots(1); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	if err := g.denyChildRequest(req); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	result, err := g.childExecutor.Execute(req)
	if err != nil {
		panic(g.vm.NewGoError(err))
	}
	promise, resolve, _ := g.vm.NewPromise()
	if err := resolve(g.vm.ToValue(childResultValueMap(result))); err != nil {
		panic(g.vm.NewGoError(fmt.Errorf("resolve agent.run promise: %w", err)))
	}
	return g.vm.ToValue(promise)
}

func (g *runtimeGlobals) workflowName() string {
	metaValue := g.vm.Get("meta")
	if metaValue == nil || goja.IsUndefined(metaValue) || goja.IsNull(metaValue) {
		return ""
	}
	meta, ok := metaValue.Export().(map[string]any)
	if !ok {
		return ""
	}
	name, _ := meta["name"].(string)
	return name
}

func (g *runtimeGlobals) argsSubject() string {
	argsValue := g.vm.Get("args")
	if argsValue == nil || goja.IsUndefined(argsValue) || goja.IsNull(argsValue) {
		return ""
	}
	args, ok := argsValue.Export().(map[string]any)
	if !ok {
		return ""
	}
	subject, _ := args["subject"].(string)
	return subject
}
