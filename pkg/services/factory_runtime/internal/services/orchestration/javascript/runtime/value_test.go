package workflowruntime

import (
	"errors"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestTypedValueFromGoja_PromiseRejectionUsesUnderlyingGoError(t *testing.T) {
	vm := goja.New()
	promise, _, reject := vm.NewPromise()
	reject(vm.NewGoError(errors.New("Antigravity: Agy does not support a separate reasoning effort.")))

	_, err := typedValueFromGoja(vm, vm.ToValue(promise))
	if err == nil || !strings.Contains(err.Error(), "Antigravity") ||
		!strings.Contains(err.Error(), "Agy does not support a separate reasoning effort") {
		t.Fatalf("typedValueFromGoja() error = %v, want readable GoError detail", err)
	}
	if strings.Contains(err.Error(), "map[") || strings.Contains(err.Error(), "unknown") {
		t.Fatalf("typedValueFromGoja() error = %v, must not format the GoError as an exported map", err)
	}
}

func TestResolvePromiseValue_PromiseRejectionUsesUnderlyingGoError(t *testing.T) {
	vm := goja.New()
	promise, _, reject := vm.NewPromise()
	reject(vm.NewGoError(errors.New("Antigravity: Agy does not support a separate reasoning effort.")))

	value, err := (&runtimeGlobals{vm: vm}).resolvePromiseValue(vm.ToValue(promise))
	if value != nil {
		t.Fatalf("resolvePromiseValue() value = %#v, want nil", value)
	}
	if err == nil || !strings.Contains(err.Error(), "Antigravity") ||
		!strings.Contains(err.Error(), "Agy does not support a separate reasoning effort") {
		t.Fatalf("resolvePromiseValue() error = %v, want readable GoError detail", err)
	}
	if strings.Contains(err.Error(), "map[") || strings.Contains(err.Error(), "unknown") {
		t.Fatalf("resolvePromiseValue() error = %v, must not format the GoError as an exported map", err)
	}
}
