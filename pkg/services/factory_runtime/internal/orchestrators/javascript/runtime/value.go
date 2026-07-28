package workflowruntime

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/dop251/goja"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/runtimecontract"
)

func typedValueFromGoja(vm *goja.Runtime, value goja.Value) (workflowresult.TypedValue, error) {
	if value == nil || goja.IsUndefined(value) {
		return workflowresult.TypedValue{}, nil
	}
	if goja.IsNull(value) {
		raw, err := json.Marshal(nil)
		if err != nil {
			return workflowresult.TypedValue{}, fmt.Errorf("marshal null result: %w", err)
		}
		return workflowresult.TypedValue{JSON: raw}, nil
	}
	exported := value.Export()
	if exported == nil {
		raw, err := json.Marshal(nil)
		if err != nil {
			return workflowresult.TypedValue{}, fmt.Errorf("marshal null result: %w", err)
		}
		return workflowresult.TypedValue{JSON: raw}, nil
	}
	switch reflect.TypeOf(exported).Kind() {
	case reflect.Func:
		return workflowresult.TypedValue{Function: true}, nil
	}
	if promise, ok := exported.(*goja.Promise); ok {
		switch promise.State() {
		case goja.PromiseStatePending:
			return workflowresult.TypedValue{Unresolved: true}, nil
		case goja.PromiseStateRejected:
			return workflowresult.TypedValue{}, fmt.Errorf("workflow promise rejected: %v", promise.Result().Export())
		default:
			return typedValueFromGoja(vm, promise.Result())
		}
	}
	raw, err := json.Marshal(exported)
	if err != nil {
		return workflowresult.TypedValue{}, fmt.Errorf("marshal workflow result: %w", err)
	}
	return workflowresult.TypedValue{JSON: raw}, nil
}
