package workflowruntime

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/dop251/goja"
	workflowpolicy "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/orchestratorcontract"
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
	result, err := g.executeChild(req)
	if err != nil {
		panic(g.vm.NewGoError(err))
	}
	promise, resolve, _ := g.vm.NewPromise()
	if err := resolve(g.vm.ToValue(childResultValueMap(result))); err != nil {
		panic(g.vm.NewGoError(fmt.Errorf("resolve agent.run promise: %w", err)))
	}
	return g.vm.ToValue(promise)
}

// executeChild keeps JavaScript callback execution serialized while allowing
// a pipeline child to spend its waiting time outside the non-thread-safe Goja
// VM. The next pipeline item can acquire the gate and dispatch its own child
// during that interval.
func (g *runtimeGlobals) executeChild(req ChildExecutionRequest) (ChildExecutionResult, error) {
	if g.pipelineVMCallMu == nil {
		return g.childExecutor.Execute(g.ctx, req)
	}

	g.pipelineVMCallMu.Unlock()
	result, err := g.childExecutor.Execute(g.ctx, req)
	g.pipelineVMCallMu.Lock()
	return result, err
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
	return workflowpolicy.ValidateChildRequest(g.policy, g.childPolicyRequest(req))
}

func (g *runtimeGlobals) childPolicyRequest(req ChildExecutionRequest) workflowpolicy.ChildRequest {
	request := childPolicyRequest(req)
	request.FactoryName = g.factoryName
	return request
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
		SkipPermissions: childSkipPermissions(req),
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

	results, err := g.executeParallel(items)
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

func (g *runtimeGlobals) executeParallel(items []parallelItem) ([]any, error) {
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
		if err := g.executeParallelAgentSpecs(specItems, results); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (g *runtimeGlobals) executeParallelAgentSpecs(items []parallelItem, results []any) error {
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
			select {
			case g.parallelGate <- struct{}{}:
				defer func() { <-g.parallelGate }()
			case <-g.ctx.Done():
				results[item.index] = failedChildResultValue("", "", g.ctx.Err())
				return
			}

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
				return nil, fmt.Errorf("%s", promiseRejectionMessage(g.vm, reason))
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
	stages, err := g.pipelineStagesFromCall(call)
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}

	previousPipelineVMCallMu := g.pipelineVMCallMu
	g.pipelineVMCallMu = &sync.Mutex{}
	defer func() {
		g.pipelineVMCallMu = previousPipelineVMCallMu
	}()

	results, err := g.executePipeline(items, stages)
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

func (g *runtimeGlobals) pipelineStagesFromCall(call goja.FunctionCall) ([]goja.Callable, error) {
	if len(call.Arguments) < 2 {
		return nil, fmt.Errorf("pipeline() requires items and worker arguments")
	}

	stages := make([]goja.Callable, 0, len(call.Arguments)-1)
	for argumentIndex := 1; argumentIndex < len(call.Arguments); argumentIndex++ {
		argument := call.Arguments[argumentIndex]
		// Keep the established one-stage compatibility behavior for an explicit
		// undefined optional next argument. Undefined is invalid once another
		// variadic stage follows it.
		if argumentIndex == 2 && len(call.Arguments) == 3 && goja.IsUndefined(argument) {
			continue
		}
		if goja.IsUndefined(argument) {
			return nil, pipelineStageFunctionError(argumentIndex)
		}
		callable, ok := goja.AssertFunction(argument)
		if !ok {
			return nil, pipelineStageFunctionError(argumentIndex)
		}
		stages = append(stages, callable)
	}
	return stages, nil
}

func pipelineStageFunctionError(argumentIndex int) error {
	switch argumentIndex {
	case 1:
		return fmt.Errorf("pipeline() requires a worker function argument")
	case 2:
		return fmt.Errorf("pipeline() next argument must be a function when provided")
	default:
		return fmt.Errorf("pipeline() stage %d argument must be a function", argumentIndex)
	}
}

func (g *runtimeGlobals) executePipeline(items []any, stages []goja.Callable) ([]any, error) {
	results := make([]any, len(items))
	var wg sync.WaitGroup
	start := make([]chan struct{}, len(items))
	entered := make([]chan struct{}, len(items))
	for index, item := range items {
		start[index] = make(chan struct{})
		entered[index] = make(chan struct{})
		wg.Add(1)
		go func(index int, item any, allow <-chan struct{}, stageEntered chan<- struct{}) {
			defer wg.Done()
			<-allow
			results[index] = g.executePipelineItem(item, index, stages, stageEntered)
		}(index, item, start[index], entered[index])
	}
	for index := range items {
		close(start[index])
		<-entered[index]
	}
	wg.Wait()
	return results, nil
}

func (g *runtimeGlobals) executePipelineItem(item any, index int, stages []goja.Callable, firstStageEntered chan<- struct{}) map[string]any {
	stageResults := make([]any, 0, len(stages))
	var previousResult any
	status := ChildDispatchStatusCompleted

	for stageIndex, stage := range stages {
		stageEntered := firstStageEntered
		if stageIndex > 0 {
			stageEntered = nil
		}
		stageResult, stageErr := g.callPipelineStage(stage, stageIndex, previousResult, item, index, stageEntered)
		stageResults = append(stageResults, pipelineStageValue(stageIndex, stageResult, stageErr))
		if stageErr != nil {
			status = ChildDispatchStatusFailed
			break
		}
		previousResult = stageResult
	}

	return pipelineItemResult(index, item, stageResults, status)
}

func (g *runtimeGlobals) callPipelineStage(stage goja.Callable, stageIndex int, prior any, item any, index int, entered chan<- struct{}) (any, error) {
	if g.pipelineVMCallMu != nil {
		g.pipelineVMCallMu.Lock()
		defer g.pipelineVMCallMu.Unlock()
	}
	if entered != nil {
		close(entered)
	}

	var (
		value goja.Value
		err   error
	)
	if stageIndex == 0 {
		value, err = stage(goja.Undefined(), g.vm.ToValue(item), g.vm.ToValue(index))
	} else {
		value, err = stage(goja.Undefined(), g.vm.ToValue(prior), g.vm.ToValue(item), g.vm.ToValue(index))
	}
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
