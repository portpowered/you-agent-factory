package wire

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workersinternal "github.com/portpowered/infinite-you/pkg/services/workers/internal"
)

func TestWorkersWireProviderAndLoggingBridgesPreserveRequests(t *testing.T) {
	t.Parallel()

	var observed platformprocess.CommandRequest
	next := canonicalCommandRunnerFunc(func(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
		observed = request
		return platformprocess.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr")}, nil
	})
	logged := NewLoggingCommandRunner(next, logging.NoopLogger{}, func() time.Time { return time.Unix(10, 0) })
	if logged == nil {
		t.Fatal("NewLoggingCommandRunner() = nil, want wrapped runner")
	}
	result, err := logged.Run(context.Background(), platformprocess.CommandRequest{
		Command: "worker", Args: []string{"--flag"}, WorkDir: "factory",
	})
	if err != nil || string(result.Stdout) != "stdout" || string(result.Stderr) != "stderr" {
		t.Fatalf("logging bridge Run() = %#v, %v", result, err)
	}
	if observed.Command != "worker" || !reflect.DeepEqual(observed.Args, []string{"--flag"}) || observed.WorkDir != "factory" {
		t.Fatalf("logging bridge request = %#v", observed)
	}
	if NewLoggingCommandRunner(nil, logging.NoopLogger{}, func() time.Time { return time.Unix(10, 0) }) != nil {
		t.Fatal("NewLoggingCommandRunner(nil) should return nil")
	}
	if NewLoggingCommandRunner(next, logging.NoopLogger{}, nil) == nil {
		t.Fatal("NewLoggingCommandRunner(nil clock) should preserve the next runner")
	}

	request := providerCommandRequest{
		Command:                  "provider-worker",
		Args:                     []string{"--model", "tts"},
		Stdin:                    []byte("input"),
		Env:                      []string{"MODE=test"},
		WorkDir:                  "factory",
		AttemptID:                "attempt-1",
		TransitionID:             "execute",
		WorkerType:               "inference",
		WorkstationName:          "tts-executor",
		ProjectID:                "project",
		InputTokens:              []any{"token"},
		InputBindings:            map[string][]string{"text": {"input-1"}},
		Execution:                work.ExecutionMetadata{},
		ExecutionLogger:          logging.NoopLogger{},
		ProcessLifecycleObserver: nil,
	}
	converted := workerCommandRequest(request)
	if converted.DispatchID != request.AttemptID || converted.Command != request.Command ||
		!reflect.DeepEqual(converted.Args, request.Args) || string(converted.Stdin) != string(request.Stdin) ||
		converted.WorkDir != request.WorkDir || converted.TransitionID != request.TransitionID ||
		converted.WorkerType != request.WorkerType || converted.WorkstationName != request.WorkstationName ||
		converted.ProjectID != request.ProjectID {
		t.Fatalf("workerCommandRequest() = %#v, want detached provider request mapping", converted)
	}

	opaque := NewProviderCommandRunner(next)
	providerRunner, ok := opaque.(providerCommandRunner)
	if !ok {
		t.Fatalf("NewProviderCommandRunner() type = %T, want providerCommandRunner", opaque)
	}
	result, err = providerRunner.Run(context.Background(), request)
	if err != nil || string(result.Stdout) != "stdout" {
		t.Fatalf("provider command Run() = %#v, %v", result, err)
	}
	var chunks []recordedOutputChunk
	result, err = providerRunner.RunStreaming(context.Background(), request, recordOutputChunks(&chunks))
	if err != nil || string(result.Stderr) != "stderr" {
		t.Fatalf("buffered provider RunStreaming() = %#v, %v", result, err)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{
		{stream: platformprocess.OutputStreamStdout, chunk: "stdout"},
		{stream: platformprocess.OutputStreamStderr, chunk: "stderr"},
	})

	streamingOpaque := NewProviderCommandRunner(streamingCanonicalCommandRunner{chunk: "streamed"})
	streamingRunner := streamingOpaque.(providerCommandRunner)
	chunks = nil
	result, err = streamingRunner.RunStreaming(context.Background(), request, recordOutputChunks(&chunks))
	if err != nil || string(result.Stdout) != "streamed provider-worker" {
		t.Fatalf("streaming provider RunStreaming() = %#v, %v", result, err)
	}
	assertStreamedChunks(t, chunks, []recordedOutputChunk{{stream: platformprocess.OutputStreamStdout, chunk: "streamed"}})

	if _, err := (providerCommandRunner{}).Run(context.Background(), request); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("nil provider Run() error = %v", err)
	}
	if _, err := (providerCommandRunner{}).RunStreaming(context.Background(), request, nil); err == nil || !strings.Contains(err.Error(), "provider command runner is required") {
		t.Fatalf("nil provider RunStreaming() error = %v", err)
	}
}

func TestWorkersWireProviderConstructionReturnsSelectedService(t *testing.T) {
	t.Parallel()

	providersService := &statelessTestProviders{}
	got, err := NewProviderFromCommandRunner(providersService, nil, nil, nil, nil, nil, nil, "")
	if err != nil || got != providersService {
		t.Fatalf("NewProviderFromCommandRunner() = %v, %v; want selected service", got, err)
	}
	if _, err := NewProviderFromCommandRunner(nil, nil, nil, nil, nil, nil, nil, ""); err == nil || !strings.Contains(err.Error(), "service is required") {
		t.Fatalf("NewProviderFromCommandRunner(nil) error = %v", err)
	}
}

func TestWorkersWirePTYBridgeHandlesTypedAndMalformedForeignShapes(t *testing.T) {
	t.Parallel()

	if adaptPTYAllocator(nil) != nil || adaptPTYAllocator(struct{}{}) != nil {
		t.Fatal("adaptPTYAllocator() should reject nil and shapes without Allocate")
	}
	typed := adaptPTYAllocator(&workersinternal.MockPTYAllocator{Result: workersinternal.PTYSessionResult{ExitCode: 3}})
	if typed == nil {
		t.Fatal("adaptPTYAllocator(typed allocator) = nil")
	}

	for _, testCase := range []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "bad arity", value: badArityPTYAllocator{}, wantErr: "unsupported method arity"},
		{name: "bad input shape", value: badInputPTYAllocator{}, wantErr: "must be a struct"},
		{name: "one return", value: oneReturnPTYAllocator{}, wantErr: "returned 1 values"},
		{name: "nil session", value: nilSessionPTYAllocator{}, wantErr: "returned a nil session"},
		{name: "allocator error", value: errorPTYAllocator{}, wantErr: "allocator failed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			allocator := adaptPTYAllocator(testCase.value)
			if allocator == nil {
				t.Fatal("adaptPTYAllocator() = nil")
			}
			_, err := allocator.Allocate(context.Background(), workersinternal.PTYProcessLaunch{}, workersinternal.PTYSessionConfig{})
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("Allocate() error = %v, want %q", err, testCase.wantErr)
			}
		})
	}

	if _, err := (reflectedPTYAllocator{value: reflect.ValueOf(struct{}{})}).Allocate(context.Background(), workersinternal.PTYProcessLaunch{}, workersinternal.PTYSessionConfig{}); err == nil || !strings.Contains(err.Error(), "does not implement Allocate") {
		t.Fatalf("invalid reflected allocator error = %v", err)
	}

	for _, testCase := range []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "missing Run", value: struct{}{}, wantErr: "does not implement Run"},
		{name: "one result", value: ptyRunOneResult{}, wantErr: "returned 1 values"},
		{name: "nil result", value: ptyRunNilResult{}, wantErr: "returned a nil result"},
		{name: "non struct result", value: ptyRunNonStructResult{}, wantErr: "must be a struct"},
		{name: "run error", value: ptyRunError{}, wantErr: "run failed"},
	} {
		t.Run("run "+testCase.name, func(t *testing.T) {
			_, err := (reflectedPTYSession{value: reflect.ValueOf(testCase.value)}).Run(context.Background())
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("Run() error = %v, want %q", err, testCase.wantErr)
			}
		})
	}

	for _, testCase := range []struct {
		name    string
		value   any
		wantErr string
	}{
		{name: "missing Close", value: struct{}{}, wantErr: "does not implement Close"},
		{name: "two results", value: ptyCloseTwoResults{}, wantErr: "returned 2 values"},
		{name: "non error", value: ptyCloseNonError{}, wantErr: "error result has type"},
		{name: "close error", value: ptyCloseError{}, wantErr: "close failed"},
	} {
		t.Run("close "+testCase.name, func(t *testing.T) {
			err := (reflectedPTYSession{value: reflect.ValueOf(testCase.value)}).Close()
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Fatalf("Close() error = %v, want %q", err, testCase.wantErr)
			}
		})
	}
}

func TestWorkersWirePTYReflectionHelpersHandleInvalidFields(t *testing.T) {
	t.Parallel()

	if _, err := reflectedPTYValue(reflect.TypeOf(0), nil); err == nil || !strings.Contains(err.Error(), "must be a struct") {
		t.Fatalf("reflectedPTYValue(non-struct) error = %v", err)
	}
	value, err := reflectedPTYValue(reflect.TypeOf(struct{ Count int }{}), map[string]any{"Count": "wrong", "Missing": 2})
	if err != nil || value.FieldByName("Count").Int() != 0 {
		t.Fatalf("reflectedPTYValue(unassignable fields) = %#v, %v", value, err)
	}
	if reflectedPTYError(reflect.Value{}) != nil || reflectedPTYError(reflect.ValueOf((*error)(nil))) != nil {
		t.Fatal("reflectedPTYError(nil) should be nil")
	}
	if err := reflectedPTYError(reflect.ValueOf(7)); err == nil || !strings.Contains(err.Error(), "error result has type int") {
		t.Fatalf("reflectedPTYError(non-error) = %v", err)
	}

	bad := reflect.ValueOf(struct {
		Count string
		Data  int
		Text  []byte
		Flag  string
	}{Count: "count", Data: 1, Text: []byte("text"), Flag: "flag"})
	if reflectedPTYInt(bad, "Count") != 0 || reflectedPTYInt(bad, "Missing") != 0 ||
		reflectedPTYBytes(bad, "Data") != nil || reflectedPTYBytes(bad, "Missing") != nil ||
		reflectedPTYString(bad, "Data") != "" || reflectedPTYString(bad, "Missing") != "" ||
		reflectedPTYBool(bad, "Flag") || reflectedPTYBool(bad, "Missing") {
		t.Fatal("reflection helpers should return zero values for invalid fields")
	}
	if !isNilReflectValue(reflect.Value{}) || !isNilReflectValue(reflect.ValueOf((*int)(nil))) || isNilReflectValue(reflect.ValueOf(1)) {
		t.Fatal("isNilReflectValue() did not classify values correctly")
	}
}

type badArityPTYAllocator struct{}

func (badArityPTYAllocator) Allocate(context.Context, foreignPTYProcessLaunch) (*foreignPTYSession, error) {
	return nil, nil
}

type badInputPTYAllocator struct{}

func (badInputPTYAllocator) Allocate(context.Context, int, int) (*foreignPTYSession, error) {
	return nil, nil
}

type oneReturnPTYAllocator struct{}

func (oneReturnPTYAllocator) Allocate(context.Context, foreignPTYProcessLaunch, foreignPTYSessionConfig) *foreignPTYSession {
	return &foreignPTYSession{}
}

type nilSessionPTYAllocator struct{}

func (nilSessionPTYAllocator) Allocate(context.Context, foreignPTYProcessLaunch, foreignPTYSessionConfig) (*foreignPTYSession, error) {
	return nil, nil
}

type errorPTYAllocator struct{}

func (errorPTYAllocator) Allocate(context.Context, foreignPTYProcessLaunch, foreignPTYSessionConfig) (*foreignPTYSession, error) {
	return nil, errors.New("allocator failed")
}

type ptyRunOneResult struct{}

func (ptyRunOneResult) Run(context.Context) foreignPTYSessionResult { return foreignPTYSessionResult{} }

type ptyRunNilResult struct{}

func (ptyRunNilResult) Run(context.Context) (*foreignPTYSessionResult, error) { return nil, nil }

type ptyRunNonStructResult struct{}

func (ptyRunNonStructResult) Run(context.Context) (int, error) { return 0, nil }

type ptyRunError struct{}

func (ptyRunError) Run(context.Context) (foreignPTYSessionResult, error) {
	return foreignPTYSessionResult{}, errors.New("run failed")
}

type ptyCloseTwoResults struct{}

func (ptyCloseTwoResults) Close() (error, error) { return nil, nil }

type ptyCloseNonError struct{}

func (ptyCloseNonError) Close() int { return 1 }

type ptyCloseError struct{}

func (ptyCloseError) Close() error { return errors.New("close failed") }
