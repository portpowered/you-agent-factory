package testutil

import (
	"context"
	"errors"
	"reflect"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

type recordingPlatformRunner struct {
	calls   int
	request platformprocess.CommandRequest
	result  platformprocess.CommandResult
	err     error
	onRun   func()
}

func (r *recordingPlatformRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.calls++
	r.request = request
	if r.onRun != nil {
		r.onRun()
	}
	if r.result.Stdout == nil && r.result.Stderr == nil && r.result.ExitCode == 0 {
		r.result.Stdout = []byte("ok")
	}
	return r.result, r.err
}

type recordingStreamingPlatformRunner struct {
	recordingPlatformRunner
	stream    func(platformprocess.OutputChunkObserver)
	streamErr error
}

func (r *recordingStreamingPlatformRunner) RunStreaming(
	_ context.Context,
	request platformprocess.CommandRequest,
	observer platformprocess.OutputChunkObserver,
) (platformprocess.CommandResult, error) {
	r.calls++
	r.request = request
	if r.stream != nil {
		r.stream(observer)
	}
	return r.result, r.streamErr
}

func TestAdaptPlatformCommandRunnerRunMapsRequestAndResult(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{
		result: platformprocess.CommandResult{
			Stdout:   []byte("stdout"),
			Stderr:   []byte("stderr"),
			ExitCode: 7,
		},
	}
	adapted := AdaptPlatformCommandRunner(runner)
	if adapted == nil {
		t.Fatal("AdaptPlatformCommandRunner() = nil, want adapter")
	}
	request := providers.CommandRequest{
		Command:          "provider",
		Args:             []string{"--print"},
		Stdin:            []byte("input"),
		Env:              []string{"MODE=test"},
		WorkDir:          "C:\\factory",
		FactorySessionID: "factory-session-1",
	}
	result, err := adapted.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !reflect.DeepEqual(runner.request, platformprocess.CommandRequest{
		Command:          "provider",
		Args:             []string{"--print"},
		Stdin:            []byte("input"),
		Env:              []string{"MODE=test"},
		WorkDir:          "C:\\factory",
		ExecutionScopeID: "factory-session-1",
	}) {
		t.Fatalf("platform request = %#v, want mapped request", runner.request)
	}
	if !reflect.DeepEqual(result, providers.CommandResult{
		Stdout:   []byte("stdout"),
		Stderr:   []byte("stderr"),
		ExitCode: 7,
	}) {
		t.Fatalf("adapted result = %#v, want mapped result", result)
	}
}

func TestAdaptPlatformCommandRunnerStreamingFallsBackAndPublishesOutput(t *testing.T) {
	t.Parallel()

	runner := &recordingPlatformRunner{result: platformprocess.CommandResult{
		Stdout: []byte("stdout"),
		Stderr: []byte("stderr"),
	}}
	adapted := AdaptPlatformCommandRunner(runner)
	streaming, ok := adapted.(providers.StreamingCommandRunner)
	if !ok {
		t.Fatal("AdaptPlatformCommandRunner() does not expose streaming effect")
	}
	var streams []string
	var chunks []string
	result, err := streaming.RunStreaming(context.Background(), providers.CommandRequest{}, func(stream string, chunk []byte) error {
		streams = append(streams, stream)
		chunks = append(chunks, string(chunk))
		return nil
	})
	if err != nil {
		t.Fatalf("RunStreaming() error = %v", err)
	}
	if !reflect.DeepEqual(streams, []string{providers.OutputStreamStdout, providers.OutputStreamStderr}) || !reflect.DeepEqual(chunks, []string{"stdout", "stderr"}) {
		t.Fatalf("published output = (%v, %v), want stdout/stderr chunks", streams, chunks)
	}
	if !reflect.DeepEqual(result, providers.CommandResult{Stdout: []byte("stdout"), Stderr: []byte("stderr")}) {
		t.Fatalf("fallback result = %#v, want completed result", result)
	}
}

func TestAdaptPlatformCommandRunnerStreamingPreservesObserverError(t *testing.T) {
	t.Parallel()

	runner := &recordingStreamingPlatformRunner{
		recordingPlatformRunner: recordingPlatformRunner{result: platformprocess.CommandResult{ExitCode: 3}},
		stream: func(observer platformprocess.OutputChunkObserver) {
			observer("stdout", []byte("first"))
			observer("stderr", []byte("second"))
		},
	}
	adapted := AdaptPlatformCommandRunner(runner)
	streaming, ok := adapted.(providers.StreamingCommandRunner)
	if !ok {
		t.Fatal("AdaptPlatformCommandRunner() does not expose streaming effect")
	}
	wantErr := errors.New("observer stopped")
	var calls int
	result, err := streaming.RunStreaming(context.Background(), providers.CommandRequest{}, func(_ string, _ []byte) error {
		calls++
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("RunStreaming() error = %v, want observer error", err)
	}
	if calls != 1 {
		t.Fatalf("observer calls = %d, want one after observer failure", calls)
	}
	if result.ExitCode != 3 {
		t.Fatalf("streaming result = %#v, want exit code preserved", result)
	}
}

func TestAdaptPlatformCommandRunnerNilAndEmptyOutput(t *testing.T) {
	t.Parallel()

	if AdaptPlatformCommandRunner(nil) != nil {
		t.Fatal("AdaptPlatformCommandRunner(nil) returned an adapter")
	}
	if err := publishCompleteOutput(nil, nil, nil); err != nil {
		t.Fatalf("publishCompleteOutput(nil) error = %v", err)
	}
}

// AdaptPlatformCommandRunner projects the policy-free platform process edge
// into the Providers-owned execution effect contract for component fixtures.
func AdaptPlatformCommandRunner(runner platformprocess.CommandRunner) providers.CommandRunner {
	if runner == nil {
		return nil
	}
	return platformCommandRunner{runner: runner}
}

type platformCommandRunner struct {
	runner platformprocess.CommandRunner
}

func (runner platformCommandRunner) Run(
	ctx context.Context,
	request providers.CommandRequest,
) (providers.CommandResult, error) {
	result, err := runner.runner.Run(ctx, platformprocess.CommandRequest{
		Command:                  request.Command,
		Args:                     request.Args,
		Stdin:                    request.Stdin,
		Env:                      request.Env,
		WorkDir:                  request.WorkDir,
		ExecutionScopeID:         request.FactorySessionID,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	})
	return providers.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func (runner platformCommandRunner) RunStreaming(
	ctx context.Context,
	request providers.CommandRequest,
	observer providers.OutputChunkObserver,
) (providers.CommandResult, error) {
	platformRequest := platformprocess.CommandRequest{
		Command:                  request.Command,
		Args:                     request.Args,
		Stdin:                    request.Stdin,
		Env:                      request.Env,
		WorkDir:                  request.WorkDir,
		ExecutionScopeID:         request.FactorySessionID,
		ExecutionLogger:          request.ExecutionLogger,
		ProcessLifecycleObserver: request.ProcessLifecycleObserver,
	}
	streaming, ok := runner.runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	})
	if !ok {
		result, err := runner.Run(ctx, request)
		if observerErr := publishCompleteOutput(observer, result.Stdout, result.Stderr); err == nil {
			err = observerErr
		}
		return result, err
	}
	var observerErr error
	result, err := streaming.RunStreaming(ctx, platformRequest, func(stream string, chunk []byte) {
		if observerErr != nil || observer == nil {
			return
		}
		observerErr = observer(stream, chunk)
	})
	if err == nil {
		err = observerErr
	}
	return providers.CommandResult{
		Stdout:   result.Stdout,
		Stderr:   result.Stderr,
		ExitCode: result.ExitCode,
	}, err
}

func publishCompleteOutput(observer providers.OutputChunkObserver, stdout, stderr []byte) error {
	if observer == nil {
		return nil
	}
	if len(stdout) > 0 {
		if err := observer(providers.OutputStreamStdout, append([]byte(nil), stdout...)); err != nil {
			return err
		}
	}
	if len(stderr) > 0 {
		return observer(providers.OutputStreamStderr, append([]byte(nil), stderr...))
	}
	return nil
}
