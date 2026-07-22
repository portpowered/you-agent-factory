package workflowruntime

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dop251/goja"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/orchestratorcontract"
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
	req, err := childExecutionRequestFromSpec(spec, g.workflowName(), g.argsSubject(), g.agents)
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	req, err = ResolveChildWorkerSettings(req, g.agents, g.workerSettings)
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	if err := g.denyChildSlots(1); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	if err := g.denyChildRequest(req); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	result, err := g.childExecutor.Execute(g.ctx, req)
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

func (g *runtimeGlobals) denyChildSlots(count int) error {
	if count <= 0 {
		return nil
	}
	current := g.records.childDispatchCountValue()
	if current+count > g.policy.MaxAgents {
		return fmt.Errorf(
			"policy denied: requested fanout %d exceeds maxAgents %d",
			count,
			g.policy.MaxAgents,
		)
	}
	return nil
}

func (g *runtimeGlobals) denyChildRequest(req ChildExecutionRequest) error {
	return workflowpolicy.ValidateChildRequest(g.policy, childPolicyRequest(req))
}

func (g *runtimeGlobals) denyArtifactSize(sizeBytes int64) error {
	if g.policy.MaxArtifactBytes == nil || *g.policy.MaxArtifactBytes <= 0 {
		return nil
	}
	if sizeBytes <= *g.policy.MaxArtifactBytes {
		return nil
	}
	return fmt.Errorf(
		"policy denied: artifact content size %d exceeds maxArtifactBytes %d",
		sizeBytes,
		*g.policy.MaxArtifactBytes,
	)
}

func childPolicyRequest(req ChildExecutionRequest) workflowpolicy.ChildRequest {
	return workflowpolicy.ChildRequest{
		Label:           req.Label,
		Model:           req.Model,
		ReasoningEffort: req.ReasoningEffort,
		Command:         req.Command,
		Sandbox:         req.Sandbox,
		WritableRoots:   append([]string(nil), req.WritableRoots...),
		AllowNetwork:    req.AllowNetwork,
		Concurrency:     req.Concurrency,
	}
}

type parallelItemKind int

const (
	parallelItemAgentSpec parallelItemKind = iota
	parallelItemFunction
)

type parallelItem struct {
	index             int
	kind              parallelItemKind
	spec              map[string]any
	request           ChildExecutionRequest
	requestValidation error
	value             goja.Value
}

func (g *runtimeGlobals) bindParallelAPI() error {
	return g.vm.Set("parallel", g.hostParallel)
}

func (g *runtimeGlobals) hostParallel(call goja.FunctionCall) goja.Value {
	items, err := g.parallelItemsFromCall(call)
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	dispatchableCount := g.normalizeParallelAgentSpecs(items)
	if err := g.denyChildSlots(dispatchableCount); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}

	concurrency := g.effectiveParallelConcurrency(dispatchableCount)
	results, err := g.executeParallel(items, concurrency)
	if err != nil {
		panic(g.vm.NewGoError(err))
	}

	promise, resolve, _ := g.vm.NewPromise()
	if err := resolve(g.vm.ToValue(results)); err != nil {
		panic(g.vm.NewGoError(fmt.Errorf("resolve parallel promise: %w", err)))
	}
	return g.vm.ToValue(promise)
}

func (g *runtimeGlobals) parallelItemsFromCall(call goja.FunctionCall) ([]parallelItem, error) {
	if len(call.Arguments) == 0 || goja.IsUndefined(call.Arguments[0]) {
		return nil, fmt.Errorf("parallel() requires an array argument")
	}
	arg := call.Arguments[0]
	obj := arg.ToObject(g.vm)
	if obj == nil {
		return nil, fmt.Errorf("parallel() requires an array argument")
	}
	lengthValue := obj.Get("length")
	if lengthValue == nil || goja.IsUndefined(lengthValue) {
		return nil, fmt.Errorf("parallel() requires an array argument")
	}
	length := int(lengthValue.ToInteger())
	items := make([]parallelItem, 0, length)
	for i := 0; i < length; i++ {
		itemValue := obj.Get(strconv.Itoa(i))
		item, err := g.classifyParallelItem(i, itemValue)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (g *runtimeGlobals) classifyParallelItem(index int, value goja.Value) (parallelItem, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return parallelItem{}, fmt.Errorf("parallel() items must not contain null or undefined entries")
	}
	if spec, ok := value.Export().(map[string]any); ok && stringField(spec, "prompt") != "" {
		return parallelItem{
			index: index,
			kind:  parallelItemAgentSpec,
			spec:  spec,
		}, nil
	}
	if _, ok := goja.AssertFunction(value); ok {
		return parallelItem{
			index: index,
			kind:  parallelItemFunction,
			value: value,
		}, nil
	}
	return parallelItem{}, fmt.Errorf("parallel() items must be agent run specs or functions")
}

func (g *runtimeGlobals) effectiveParallelConcurrency(itemCount int) int {
	concurrency := g.policy.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	if itemCount < concurrency {
		return itemCount
	}
	return concurrency
}

// normalizeParallelAgentSpecs applies the runtime child contract before policy
// accounts for dispatchable parallel work. Invalid object specs remain in the
// result set as failures, but do not consume fanout budget or dispatch identity.
func (g *runtimeGlobals) normalizeParallelAgentSpecs(items []parallelItem) int {
	dispatchableCount := 0
	for index := range items {
		if items[index].kind == parallelItemFunction {
			dispatchableCount++
			continue
		}
		items[index].request, items[index].requestValidation = childExecutionRequestFromSpec(
			items[index].spec,
			g.workflowName(),
			g.argsSubject(),
			g.agents,
		)
		if items[index].requestValidation == nil {
			dispatchableCount++
		}
	}
	return dispatchableCount
}

func (g *runtimeGlobals) executeParallel(items []parallelItem, concurrency int) ([]any, error) {
	results := make([]any, len(items))
	if len(items) == 0 {
		return results, nil
	}

	specItems := make([]parallelItem, 0, len(items))
	functionItems := make([]parallelItem, 0)
	for _, item := range items {
		switch item.kind {
		case parallelItemAgentSpec:
			if item.requestValidation != nil {
				results[item.index] = failedChildResultValue("", "", item.requestValidation)
				continue
			}
			specItems = append(specItems, item)
		case parallelItemFunction:
			functionItems = append(functionItems, item)
		}
	}

	if len(functionItems) > 0 {
		if err := g.executeParallelFunctions(functionItems, results); err != nil {
			return nil, err
		}
	}
	if len(specItems) > 0 {
		if err := g.executeParallelAgentSpecs(specItems, concurrency, results); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (g *runtimeGlobals) executeParallelAgentSpecs(items []parallelItem, concurrency int, results []any) error {
	sem := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	identityByIndex := make(map[int]*ChildDispatchIdentity, len(items))
	for _, item := range items {
		dispatchID, childIndex := g.records.nextChildDispatchIdentity()
		identityByIndex[item.index] = &ChildDispatchIdentity{
			DispatchID: dispatchID,
			ChildIndex: childIndex,
		}
	}

	for _, item := range items {
		wg.Add(1)
		go func(item parallelItem) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := g.ctx.Err(); err != nil {
				results[item.index] = failedChildResultValue("", "", err)
				return
			}

			// Lower-index items wait longer so completion order can differ from input order.
			time.Sleep(time.Duration(len(results)-item.index) * time.Millisecond)

			if err := g.ctx.Err(); err != nil {
				results[item.index] = failedChildResultValue("", "", err)
				return
			}

			req, err := ResolveChildWorkerSettings(item.request, g.agents, g.workerSettings)
			if err != nil {
				results[item.index] = failedChildResultValue(req.Label, "", err)
				return
			}
			if err := g.denyChildRequest(req); err != nil {
				results[item.index] = failedChildResultValue(req.Label, "", err)
				return
			}
			req.ReservedIdentity = identityByIndex[item.index]
			result, err := g.childExecutor.Execute(g.ctx, req)
			if err != nil {
				results[item.index] = failedChildResultValue(req.Label, result.ExecutionMode, err)
				return
			}
			results[item.index] = childResultValueMap(result)
		}(item)
	}
	wg.Wait()
	return nil
}

func (g *runtimeGlobals) executeParallelFunctions(items []parallelItem, results []any) error {
	for _, item := range items {
		value, err := g.invokeParallelFunction(item)
		if err != nil {
			return err
		}
		results[item.index] = value
	}
	return nil
}

func (g *runtimeGlobals) invokeParallelFunction(item parallelItem) (any, error) {
	callable, ok := goja.AssertFunction(item.value)
	if !ok {
		return nil, fmt.Errorf("parallel() items must be agent run specs or functions")
	}
	value, err := callable(goja.Undefined())
	if err != nil {
		return nil, err
	}
	return g.awaitParallelValue(value)
}

func (g *runtimeGlobals) awaitParallelValue(value goja.Value) (any, error) {
	if value == nil || goja.IsUndefined(value) || goja.IsNull(value) {
		return nil, nil
	}
	exported := value.Export()
	if _, ok := exported.(*goja.Promise); ok {
		return g.resolvePromiseValue(value)
	}
	return exported, nil
}

func (g *runtimeGlobals) resolvePromiseValue(value goja.Value) (any, error) {
	for {
		state := value.Export()
		promise, ok := state.(*goja.Promise)
		if !ok {
			return value.Export(), nil
		}
		switch promise.State() {
		case goja.PromiseStateFulfilled:
			return promise.Result().Export(), nil
		case goja.PromiseStateRejected:
			reason := promise.Result()
			if reason != nil {
				return nil, fmt.Errorf("%v", reason.Export())
			}
			return nil, fmt.Errorf("parallel child rejected")
		default:
			// Promises created by host primitives resolve before control returns.
			return promise.Result().Export(), nil
		}
	}
}

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
