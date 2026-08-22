package wire

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
)

// adaptPTYAllocator keeps the Workers PTY request/result types below the
// Workers internal boundary. The canonical composition root supplies the
// Providers-owned allocator as an opaque value; this owner-side structural
// adapter avoids importing a peer service's wire package.
func adaptPTYAllocator(candidate any) workersinternal.PTYAllocator {
	if candidate == nil {
		return nil
	}
	if allocator, ok := candidate.(workersinternal.PTYAllocator); ok {
		return allocator
	}
	value := reflect.ValueOf(candidate)
	if isNilReflectValue(value) || !value.MethodByName("Allocate").IsValid() {
		return nil
	}
	return reflectedPTYAllocator{value: value}
}

type reflectedPTYAllocator struct {
	value reflect.Value
}

func (allocator reflectedPTYAllocator) Allocate(
	ctx context.Context,
	launch workersinternal.PTYProcessLaunch,
	config workersinternal.PTYSessionConfig,
) (workersinternal.PTYSession, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	method := allocator.value.MethodByName("Allocate")
	if !method.IsValid() {
		return nil, errors.New("workers PTY allocator does not implement Allocate")
	}
	if method.Type().NumIn() != 3 {
		return nil, fmt.Errorf("workers PTY allocator has unsupported method arity %d", method.Type().NumIn())
	}
	launchValue, err := reflectedPTYValue(method.Type().In(1), map[string]any{
		"Executable": launch.Executable,
		"Argv":       append([]string(nil), launch.Argv...),
		"WorkDir":    launch.WorkDir,
		"Env":        append([]string(nil), launch.Env...),
	})
	if err != nil {
		return nil, err
	}
	configValue, err := reflectedPTYValue(method.Type().In(2), map[string]any{
		"MaxCaptureBytes": config.MaxCaptureBytes,
		"IdleTimeout":     config.IdleTimeout,
		"HardTimeout":     config.HardTimeout,
	})
	if err != nil {
		return nil, err
	}
	values := method.Call([]reflect.Value{reflect.ValueOf(ctx), launchValue, configValue})
	if len(values) != 2 {
		return nil, fmt.Errorf("workers PTY allocator returned %d values, want 2", len(values))
	}
	if err := reflectedPTYError(values[1]); err != nil {
		return nil, err
	}
	if isNilReflectValue(values[0]) {
		return nil, errors.New("workers PTY allocator returned a nil session")
	}
	return reflectedPTYSession{value: values[0]}, nil
}

type reflectedPTYSession struct {
	value reflect.Value
}

func (session reflectedPTYSession) Run(ctx context.Context) (workersinternal.PTYSessionResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	method := session.value.MethodByName("Run")
	if !method.IsValid() {
		return workersinternal.PTYSessionResult{}, errors.New("workers PTY session does not implement Run")
	}
	values := method.Call([]reflect.Value{reflect.ValueOf(ctx)})
	if len(values) != 2 {
		return workersinternal.PTYSessionResult{}, fmt.Errorf("workers PTY session returned %d values, want 2", len(values))
	}
	resultValue := values[0]
	if resultValue.Kind() == reflect.Ptr {
		if resultValue.IsNil() {
			return workersinternal.PTYSessionResult{}, errors.New("workers PTY session returned a nil result")
		}
		resultValue = resultValue.Elem()
	}
	if resultValue.Kind() != reflect.Struct {
		return workersinternal.PTYSessionResult{}, fmt.Errorf("workers PTY session result must be a struct, got %s", resultValue.Type())
	}
	result := workersinternal.PTYSessionResult{
		ExitCode:    reflectedPTYInt(resultValue, "ExitCode"),
		RawBytes:    reflectedPTYBytes(resultValue, "RawBytes"),
		CleanedText: reflectedPTYString(resultValue, "CleanedText"),
		TimedOut:    reflectedPTYBool(resultValue, "TimedOut"),
		CapacityHit: reflectedPTYBool(resultValue, "CapacityHit"),
	}
	return result, reflectedPTYError(values[1])
}

func (session reflectedPTYSession) Close() error {
	method := session.value.MethodByName("Close")
	if !method.IsValid() {
		return errors.New("workers PTY session does not implement Close")
	}
	values := method.Call(nil)
	if len(values) != 1 {
		return fmt.Errorf("workers PTY session Close returned %d values, want 1", len(values))
	}
	return reflectedPTYError(values[0])
}

func reflectedPTYValue(target reflect.Type, fields map[string]any) (reflect.Value, error) {
	if target.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("workers PTY value must be a struct, got %s", target)
	}
	value := reflect.New(target).Elem()
	for name, source := range fields {
		field := value.FieldByName(name)
		if !field.IsValid() || !field.CanSet() {
			continue
		}
		sourceValue := reflect.ValueOf(source)
		if sourceValue.Type().AssignableTo(field.Type()) {
			field.Set(sourceValue)
		} else if sourceValue.Type().ConvertibleTo(field.Type()) {
			field.Set(sourceValue.Convert(field.Type()))
		}
	}
	return value, nil
}

func reflectedPTYError(value reflect.Value) error {
	if !value.IsValid() || isNilReflectValue(value) {
		return nil
	}
	err, ok := value.Interface().(error)
	if !ok {
		return fmt.Errorf("workers PTY error result has type %s", value.Type())
	}
	return err
}

func reflectedPTYInt(value reflect.Value, name string) int {
	field := value.FieldByName(name)
	if !field.IsValid() || !field.CanInt() {
		return 0
	}
	return int(field.Int())
}

func reflectedPTYBytes(value reflect.Value, name string) []byte {
	field := value.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.Slice || field.Type().Elem().Kind() != reflect.Uint8 {
		return nil
	}
	return append([]byte(nil), field.Bytes()...)
}

func reflectedPTYString(value reflect.Value, name string) string {
	field := value.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.String {
		return ""
	}
	return field.String()
}

func reflectedPTYBool(value reflect.Value, name string) bool {
	field := value.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.Bool {
		return false
	}
	return field.Bool()
}

func isNilReflectValue(value reflect.Value) bool {
	if !value.IsValid() {
		return true
	}
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}
