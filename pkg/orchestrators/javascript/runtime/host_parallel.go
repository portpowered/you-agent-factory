package workflowruntime

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/dop251/goja"
)

type parallelItemKind int

const (
	parallelItemAgentSpec parallelItemKind = iota
	parallelItemFunction
)

type parallelItem struct {
	index int
	kind  parallelItemKind
	spec  map[string]any
	value goja.Value
}

func (g *runtimeGlobals) bindParallelAPI() error {
	return g.vm.Set("parallel", g.hostParallel)
}

func (g *runtimeGlobals) hostParallel(call goja.FunctionCall) goja.Value {
	items, err := g.parallelItemsFromCall(call)
	if err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}
	if err := g.denyParallelFanout(len(items)); err != nil {
		panic(g.vm.NewTypeError(err.Error()))
	}

	concurrency := g.effectiveParallelConcurrency(len(items))
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

func (g *runtimeGlobals) denyParallelFanout(fanout int) error {
	if fanout <= g.policy.MaxAgents {
		return nil
	}
	return fmt.Errorf(
		"policy denied: requested fanout %d exceeds maxAgents %d",
		fanout,
		g.policy.MaxAgents,
	)
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

			// Lower-index items wait longer so completion order can differ from input order.
			time.Sleep(time.Duration(len(results)-item.index) * time.Millisecond)

			req, err := childExecutionRequestFromSpec(item.spec, g.workflowName(), g.argsSubject())
			if err != nil {
				results[item.index] = failedChildResultValue("", err)
				return
			}
			req.ReservedIdentity = identityByIndex[item.index]
			result, err := g.childExecutor.Execute(req)
			if err != nil {
				results[item.index] = failedChildResultValue(req.Label, err)
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
