package service

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// CommandRunner is the Providers-owned subprocess effect used by provider
// adapters. The composition root may project a platform or request-scoped
// runner into this contract, but Providers never consumes a Workers command
// interface directly.
type CommandRunner interface {
	Run(context.Context, CommandRequest) (CommandResult, error)
}

// StreamingCommandRunner is the optional streaming extension of
// CommandRunner. Adapters fall back to one completed output chunk when only
// CommandRunner is available.
type StreamingCommandRunner interface {
	CommandRunner
	RunStreaming(context.Context, CommandRequest, OutputChunkObserver) (CommandResult, error)
}

// OutputChunkObserver receives output from one provider subprocess effect and
// returns an error when the consumer cannot accept the chunk.
type OutputChunkObserver func(stream string, chunk []byte) error

const (
	OutputStreamStdout = "stdout"
	OutputStreamStderr = "stderr"
)

// CommandRequest contains policy-free process inputs plus the Providers
// attempt correlation needed by composition-owned effect adapters.
type CommandRequest struct {
	Command                  string
	Args                     []string
	Stdin                    []byte
	Env                      []string
	WorkDir                  string
	FactorySessionID         string
	DispatchID               string
	AttemptID                string
	TransitionID             string
	WorkerType               string
	WorkstationName          string
	ProjectID                string
	InputTokens              []any
	InputBindings            map[string][]string
	Execution                work.ExecutionMetadata
	ExecutionLogger          logging.Logger
	ProcessLifecycleObserver platformprocess.ProcessLifecycleObserver
}

// CommandResult is the observable result of one provider subprocess effect.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// AdaptCommandRunner accepts either the Providers-owned command effect or a
// legacy composition edge whose request/result structs have the same named
// fields. The latter exists only at migration boundaries; provider adapters
// themselves retain the typed CommandRunner contract above.
func AdaptCommandRunner(candidate any) CommandRunner {
	if candidate == nil {
		return nil
	}
	if runner, ok := candidate.(CommandRunner); ok {
		return runner
	}
	value := reflect.ValueOf(candidate)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil
	}
	return reflectedCommandRunner{value: value}
}

type reflectedCommandRunner struct {
	value reflect.Value
}

func (runner reflectedCommandRunner) Run(ctx context.Context, request CommandRequest) (CommandResult, error) {
	method := runner.value.MethodByName("Run")
	if !method.IsValid() {
		return CommandResult{}, errors.New("provider command runner does not implement Run")
	}
	values, err := callReflectedCommand(method, ctx, request, nil)
	if err != nil {
		return CommandResult{}, err
	}
	return reflectedCommandResult(values)
}

func (runner reflectedCommandRunner) RunStreaming(
	ctx context.Context,
	request CommandRequest,
	observer OutputChunkObserver,
) (CommandResult, error) {
	method := runner.value.MethodByName("RunStreaming")
	if !method.IsValid() {
		result, err := runner.Run(ctx, request)
		if observerErr := publishReflectedCommandOutput(observer, result.Stdout, result.Stderr); err == nil {
			err = observerErr
		}
		return result, err
	}
	var observerErr error
	values, err := callReflectedCommand(method, ctx, request, OutputChunkObserver(func(stream string, chunk []byte) error {
		if observerErr != nil || observer == nil {
			return observerErr
		}
		observerErr = observer(stream, chunk)
		return observerErr
	}))
	if err != nil {
		return CommandResult{}, err
	}
	result, err := reflectedCommandResult(values)
	if err == nil {
		err = observerErr
	}
	return result, err
}

func publishReflectedCommandOutput(observer OutputChunkObserver, stdout, stderr []byte) error {
	if observer == nil {
		return nil
	}
	if len(stdout) > 0 {
		if err := observer(OutputStreamStdout, append([]byte(nil), stdout...)); err != nil {
			return err
		}
	}
	if len(stderr) > 0 {
		return observer(OutputStreamStderr, append([]byte(nil), stderr...))
	}
	return nil
}

func callReflectedCommand(
	method reflect.Value,
	ctx context.Context,
	request CommandRequest,
	observer any,
) ([]reflect.Value, error) {
	typeOf := method.Type()
	if typeOf.NumIn() != 2 && typeOf.NumIn() != 3 {
		return nil, fmt.Errorf("provider command runner has unsupported method arity %d", typeOf.NumIn())
	}
	requestValue, err := reflectedRequest(typeOf.In(1), request)
	if err != nil {
		return nil, err
	}
	arguments := []reflect.Value{reflect.ValueOf(ctx), requestValue}
	if typeOf.NumIn() == 3 {
		callback, err := reflectedObserver(typeOf.In(2), observer)
		if err != nil {
			return nil, err
		}
		arguments = append(arguments, callback)
	}
	return method.Call(arguments), nil
}

func reflectedRequest(target reflect.Type, request CommandRequest) (reflect.Value, error) {
	if target.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("provider command runner request must be a struct, got %s", target)
	}
	value := reflect.New(target).Elem()
	copyReflectedField(value, "Command", request.Command)
	copyReflectedField(value, "Args", request.Args)
	copyReflectedField(value, "Stdin", request.Stdin)
	copyReflectedField(value, "Env", request.Env)
	copyReflectedField(value, "WorkDir", request.WorkDir)
	dispatchID := request.DispatchID
	if dispatchID == "" {
		dispatchID = request.AttemptID
	}
	copyReflectedField(value, "DispatchID", dispatchID)
	copyReflectedField(value, "AttemptID", request.AttemptID)
	copyReflectedField(value, "TransitionID", request.TransitionID)
	copyReflectedField(value, "WorkerType", request.WorkerType)
	copyReflectedField(value, "WorkstationName", request.WorkstationName)
	copyReflectedField(value, "ProjectID", request.ProjectID)
	copyReflectedField(value, "InputTokens", request.InputTokens)
	copyReflectedField(value, "InputBindings", request.InputBindings)
	copyReflectedField(value, "Execution", request.Execution)
	copyReflectedField(value, "ExecutionLogger", request.ExecutionLogger)
	return value, nil
}

func reflectedObserver(target reflect.Type, observer any) (reflect.Value, error) {
	if target.Kind() != reflect.Func || target.NumIn() != 2 || target.NumOut() != 0 {
		return reflect.Value{}, fmt.Errorf("provider command runner observer has unsupported type %s", target)
	}
	callback := reflect.MakeFunc(target, func(values []reflect.Value) []reflect.Value {
		if observer == nil || len(values) != 2 {
			return nil
		}
		stream, _ := values[0].Interface().(string)
		chunk, _ := values[1].Interface().([]byte)
		_ = observer.(OutputChunkObserver)(stream, chunk)
		return nil
	})
	return callback, nil
}

func reflectedCommandResult(values []reflect.Value) (CommandResult, error) {
	if len(values) != 2 {
		return CommandResult{}, fmt.Errorf("provider command runner returned %d values, want 2", len(values))
	}
	resultValue := values[0]
	if resultValue.Kind() == reflect.Ptr {
		if resultValue.IsNil() {
			return CommandResult{}, errors.New("provider command runner returned a nil result")
		}
		resultValue = resultValue.Elem()
	}
	if resultValue.Kind() != reflect.Struct {
		return CommandResult{}, fmt.Errorf("provider command runner result must be a struct, got %s", resultValue.Type())
	}
	result := CommandResult{
		Stdout:   reflectedBytes(resultValue, "Stdout"),
		Stderr:   reflectedBytes(resultValue, "Stderr"),
		ExitCode: reflectedInt(resultValue, "ExitCode"),
	}
	if err := reflectedError(values[1]); err != nil {
		return result, err
	}
	return result, nil
}

func reflectedError(value reflect.Value) error {
	if !value.IsValid() || (value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr) && value.IsNil() {
		return nil
	}
	err, ok := value.Interface().(error)
	if !ok {
		return fmt.Errorf("provider command runner error result has type %s", value.Type())
	}
	return err
}

func copyReflectedField(target reflect.Value, name string, source any) {
	field := target.FieldByName(name)
	if !field.IsValid() || !field.CanSet() {
		return
	}
	value := reflect.ValueOf(source)
	if !value.IsValid() {
		return
	}
	if value.Type().AssignableTo(field.Type()) {
		field.Set(value)
	} else if value.Type().ConvertibleTo(field.Type()) {
		field.Set(value.Convert(field.Type()))
	}
}

func reflectedBytes(value reflect.Value, name string) []byte {
	field := value.FieldByName(name)
	if !field.IsValid() || field.Kind() != reflect.Slice || field.Type().Elem().Kind() != reflect.Uint8 {
		return nil
	}
	return append([]byte(nil), field.Bytes()...)
}

func reflectedInt(value reflect.Value, name string) int {
	field := value.FieldByName(name)
	if !field.IsValid() || !field.CanInt() {
		return 0
	}
	return int(field.Int())
}

// AdaptPTYAllocator accepts a Providers-owned PTY allocator or a legacy
// composition edge with the same named launch/config/result fields. It keeps
// compatibility at the composition boundary without making provider adapters
// depend on Workers contracts.
func AdaptPTYAllocator(candidate any) PTYAllocator {
	if candidate == nil {
		return nil
	}
	if allocator, ok := candidate.(PTYAllocator); ok {
		return allocator
	}
	value := reflect.ValueOf(candidate)
	if value.Kind() == reflect.Ptr && value.IsNil() {
		return nil
	}
	return reflectedPTYAllocator{value: value}
}

type reflectedPTYAllocator struct{ value reflect.Value }

func (allocator reflectedPTYAllocator) Allocate(
	ctx context.Context,
	launch PTYProcessLaunch,
	config PTYSessionConfig,
) (PTYSession, error) {
	method := allocator.value.MethodByName("Allocate")
	if !method.IsValid() || method.Type().NumIn() != 3 {
		return nil, errors.New("provider PTY allocator does not implement Allocate")
	}
	launchValue, err := reflectedRequest(method.Type().In(1), CommandRequest{
		Command: launch.Executable,
		Args:    launch.Argv,
		Env:     launch.Env,
		WorkDir: launch.WorkDir,
	})
	if err != nil {
		return nil, err
	}
	configValue, err := reflectedPTYConfig(method.Type().In(2), config)
	if err != nil {
		return nil, err
	}
	values := method.Call([]reflect.Value{reflect.ValueOf(ctx), launchValue, configValue})
	if len(values) != 2 {
		return nil, fmt.Errorf("provider PTY allocator returned %d values, want 2", len(values))
	}
	if err := reflectedError(values[1]); err != nil {
		return nil, err
	}
	if values[0].IsNil() {
		return nil, errors.New("provider PTY allocator returned a nil session")
	}
	return reflectedPTYSession{value: values[0]}, nil
}

func reflectedPTYConfig(target reflect.Type, config PTYSessionConfig) (reflect.Value, error) {
	if target.Kind() != reflect.Struct {
		return reflect.Value{}, fmt.Errorf("provider PTY config must be a struct, got %s", target)
	}
	value := reflect.New(target).Elem()
	copyReflectedField(value, "MaxCaptureBytes", config.MaxCaptureBytes)
	copyReflectedField(value, "IdleTimeout", config.IdleTimeout)
	copyReflectedField(value, "HardTimeout", config.HardTimeout)
	return value, nil
}

type reflectedPTYSession struct{ value reflect.Value }

func (session reflectedPTYSession) Run(ctx context.Context) (PTYSessionResult, error) {
	method := session.value.MethodByName("Run")
	if !method.IsValid() {
		return PTYSessionResult{}, errors.New("provider PTY session does not implement Run")
	}
	values := method.Call([]reflect.Value{reflect.ValueOf(ctx)})
	if len(values) != 2 {
		return PTYSessionResult{}, fmt.Errorf("provider PTY session returned %d values, want 2", len(values))
	}
	resultValue := values[0]
	if resultValue.Kind() == reflect.Ptr {
		resultValue = resultValue.Elem()
	}
	result := PTYSessionResult{
		ExitCode:    reflectedInt(resultValue, "ExitCode"),
		RawBytes:    reflectedBytes(resultValue, "RawBytes"),
		CleanedText: reflectedString(resultValue, "CleanedText"),
		TimedOut:    reflectedBool(resultValue, "TimedOut"),
		CapacityHit: reflectedBool(resultValue, "CapacityHit"),
	}
	return result, reflectedError(values[1])
}

func (session reflectedPTYSession) Close() error {
	method := session.value.MethodByName("Close")
	if !method.IsValid() {
		return errors.New("provider PTY session does not implement Close")
	}
	values := method.Call(nil)
	if len(values) != 1 {
		return fmt.Errorf("provider PTY session Close returned %d values, want 1", len(values))
	}
	return reflectedError(values[0])
}

func reflectedString(value reflect.Value, name string) string {
	field := value.FieldByName(name)
	if field.IsValid() && field.Kind() == reflect.String {
		return field.String()
	}
	return ""
}

func reflectedBool(value reflect.Value, name string) bool {
	field := value.FieldByName(name)
	if field.IsValid() && field.Kind() == reflect.Bool {
		return field.Bool()
	}
	return false
}

// PTYSessionConfig carries bounded capture and timeout policy for one
// Providers-owned PTY session.
type PTYSessionConfig struct {
	MaxCaptureBytes int
	IdleTimeout     time.Duration
	HardTimeout     time.Duration
}

const (
	DefaultPTYMaxCaptureBytes = 4 * 1024 * 1024
	MaxPTYMaxCaptureBytes     = 16 * 1024 * 1024
	DefaultPTYIdleTimeout     = 30 * time.Second
	DefaultPTYHardTimeout     = 10 * time.Minute
)

// DefaultPTYSessionConfig returns the bounded native-session defaults used by
// the Providers Agy adapter.
func DefaultPTYSessionConfig() PTYSessionConfig {
	return PTYSessionConfig{
		MaxCaptureBytes: DefaultPTYMaxCaptureBytes,
		IdleTimeout:     DefaultPTYIdleTimeout,
		HardTimeout:     DefaultPTYHardTimeout,
	}
}

// PTYProcessLaunch is the typed subprocess description for one Agy PTY run.
type PTYProcessLaunch struct {
	Executable string
	Argv       []string
	WorkDir    string
	Env        []string
}

// PTYSessionResult is the observable outcome after a PTY session is cleaned
// up.
type PTYSessionResult struct {
	ExitCode    int
	RawBytes    []byte
	CleanedText string
	TimedOut    bool
	CapacityHit bool
}

// PTYKind identifies the native terminal mechanism selected by the host.
type PTYKind int

const (
	PTYKindUnknown PTYKind = iota
	PTYKindPOSIX
	PTYKindConPTY
)

func (kind PTYKind) String() string {
	switch kind {
	case PTYKindPOSIX:
		return "posix"
	case PTYKindConPTY:
		return "conpty"
	default:
		return "unknown"
	}
}

// PTYAllocator opens one native PTY session for a provider subprocess.
type PTYAllocator interface {
	Allocate(context.Context, PTYProcessLaunch, PTYSessionConfig) (PTYSession, error)
}

// PTYSession is the private seam for bounded capture, timeout signaling, and
// cleanup of one provider PTY process.
type PTYSession interface {
	Run(context.Context) (PTYSessionResult, error)
	Close() error
}

var (
	ErrPTYUnsupportedPlatform = errors.New("agypty: platform PTY allocation is not supported")
	ErrPTYAllocationFailed    = errors.New("agypty: PTY allocation failed")
	ErrPTYSessionTimedOut     = errors.New("agypty: session timed out")
	ErrPTYNonzeroExit         = errors.New("agypty: process exited with nonzero status")
	ErrPTYClockRequired       = errors.New("agypty: clock is required")
	ErrPTYHostRequired        = errors.New("agypty: native PTY host is required")
)
