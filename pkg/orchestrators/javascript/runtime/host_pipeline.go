package workflowruntime

import (
	"fmt"
	"strconv"

	"github.com/dop251/goja"
)

func (g *runtimeGlobals) bindPipelineAPI() error {
	return g.vm.Set("pipeline", g.hostPipeline)
}

func (g *runtimeGlobals) hostPipeline(call goja.FunctionCall) goja.Value {
	items, err := g.pipelineItemsFromCall(call)
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	worker, err := g.pipelineStageFromCall(call, 1, "worker")
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	nextStage, hasNext, err := g.optionalPipelineStageFromCall(call, 2, "next")
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}

	results, err := g.executePipeline(items, worker, nextStage, hasNext)
	if err != nil {
		panic(g.vm.NewGoError(err))
	}

	promise, resolve, _ := g.vm.NewPromise()
	if err := resolve(g.vm.ToValue(results)); err != nil {
		panic(g.vm.NewGoError(fmt.Errorf("resolve pipeline promise: %w", err)))
	}
	return g.vm.ToValue(promise)
}

func (g *runtimeGlobals) pipelineItemsFromCall(call goja.FunctionCall) ([]any, error) {
	if len(call.Arguments) < 2 {
		return nil, fmt.Errorf("pipeline() requires items and worker arguments")
	}
	if goja.IsUndefined(call.Arguments[0]) {
		return nil, fmt.Errorf("pipeline() requires an array items argument")
	}
	arg := call.Arguments[0]
	obj := arg.ToObject(g.vm)
	if obj == nil {
		return nil, fmt.Errorf("pipeline() requires an array items argument")
	}
	lengthValue := obj.Get("length")
	if lengthValue == nil || goja.IsUndefined(lengthValue) {
		return nil, fmt.Errorf("pipeline() requires an array items argument")
	}
	length := int(lengthValue.ToInteger())
	items := make([]any, 0, length)
	for i := 0; i < length; i++ {
		itemValue := obj.Get(strconv.Itoa(i))
		if itemValue == nil || goja.IsUndefined(itemValue) || goja.IsNull(itemValue) {
			return nil, fmt.Errorf("pipeline() items must not contain null or undefined entries")
		}
		items = append(items, itemValue.Export())
	}
	return items, nil
}

func (g *runtimeGlobals) pipelineStageFromCall(call goja.FunctionCall, index int, role string) (goja.Callable, error) {
	if len(call.Arguments) <= index || goja.IsUndefined(call.Arguments[index]) {
		return nil, fmt.Errorf("pipeline() requires a %s function argument", role)
	}
	callable, ok := goja.AssertFunction(call.Arguments[index])
	if !ok {
		return nil, fmt.Errorf("pipeline() requires a %s function argument", role)
	}
	return callable, nil
}

func (g *runtimeGlobals) optionalPipelineStageFromCall(call goja.FunctionCall, index int, role string) (goja.Callable, bool, error) {
	if len(call.Arguments) <= index || goja.IsUndefined(call.Arguments[index]) {
		return nil, false, nil
	}
	callable, ok := goja.AssertFunction(call.Arguments[index])
	if !ok {
		return nil, false, fmt.Errorf("pipeline() %s argument must be a function when provided", role)
	}
	return callable, true, nil
}

func (g *runtimeGlobals) executePipeline(items []any, worker goja.Callable, nextStage goja.Callable, hasNext bool) ([]any, error) {
	results := make([]any, len(items))
	for index, item := range items {
		stageResults := make([]any, 0, 2)

		workerResult, workerErr := g.callPipelineWorker(worker, item, index)
		stageResults = append(stageResults, pipelineStageValue(0, workerResult, workerErr))
		if workerErr != nil {
			results[index] = pipelineItemResult(index, item, stageResults, ChildDispatchStatusFailed)
			continue
		}

		if !hasNext {
			results[index] = pipelineItemResult(index, item, stageResults, ChildDispatchStatusCompleted)
			continue
		}

		nextResult, nextErr := g.callPipelineNext(nextStage, workerResult, item, index)
		stageResults = append(stageResults, pipelineStageValue(1, nextResult, nextErr))
		status := ChildDispatchStatusCompleted
		if nextErr != nil {
			status = ChildDispatchStatusFailed
		}
		results[index] = pipelineItemResult(index, item, stageResults, status)
	}
	return results, nil
}

func (g *runtimeGlobals) callPipelineWorker(worker goja.Callable, item any, index int) (any, error) {
	value, err := worker(goja.Undefined(), g.vm.ToValue(item), g.vm.ToValue(index))
	if err != nil {
		return nil, err
	}
	return g.awaitParallelValue(value)
}

func (g *runtimeGlobals) callPipelineNext(nextStage goja.Callable, prior any, item any, index int) (any, error) {
	value, err := nextStage(goja.Undefined(), g.vm.ToValue(prior), g.vm.ToValue(item), g.vm.ToValue(index))
	if err != nil {
		return nil, err
	}
	return g.awaitParallelValue(value)
}

func pipelineStageValue(stageIndex int, result any, err error) map[string]any {
	value := map[string]any{
		"index": stageIndex,
	}
	if err != nil {
		value["status"] = ChildDispatchStatusFailed
		value["diagnostic"] = err.Error()
		return value
	}
	value["status"] = ChildDispatchStatusCompleted
	value["result"] = result
	return value
}

func pipelineItemResult(index int, item any, stages []any, status string) map[string]any {
	return map[string]any{
		"index":  index,
		"item":   item,
		"status": status,
		"stages": stages,
	}
}
