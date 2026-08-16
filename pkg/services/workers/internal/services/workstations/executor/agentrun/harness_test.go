package agentrun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type staticInferencer struct {
	response string
	err      error
	delay    time.Duration
}

func (s staticInferencer) Infer(ctx context.Context, _ messages.InferenceRequest) (messages.InferenceResult, error) {
	if s.delay > 0 {
		timer := time.NewTimer(s.delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return messages.InferenceResult{}, ctx.Err()
		}
	}
	if s.err != nil {
		return messages.InferenceResult{}, s.err
	}
	return messages.InferenceResult{
		Message: messages.NewTextMessage(messages.RoleAssistant, s.response),
	}, nil
}

func (s staticInferencer) InferStream(ctx context.Context, req messages.InferenceRequest) (<-chan messages.StreamMessage, error) {
	result, err := s.Infer(ctx, req)
	ch := make(chan messages.StreamMessage, 4)
	go func() {
		defer close(ch)
		if err != nil {
			ch <- messages.StreamMessage{Type: messages.StreamTypeError, Value: messages.NewErrorValue(err.Error())}
			return
		}
		text := result.Message.TextContent()
		ch <- messages.StreamMessage{Type: messages.StreamTypeTextDelta, Value: messages.NewTextDeltaValue(text)}
		ch <- messages.StreamMessage{Type: messages.StreamTypeMessageEnd, Value: messages.NewMessageEndValue(messages.TokenUsage{})}
	}()
	return ch, nil
}

type stubRunner struct {
	response    string
	err         error
	delay       time.Duration
	mu          sync.Mutex
	calls       int
	lastRequest workerexecution.RunnerExecutionRequest
}

func (runner *stubRunner) Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	runner.mu.Lock()
	runner.calls++
	runner.lastRequest = request
	runner.mu.Unlock()
	if runner.delay > 0 {
		timer := time.NewTimer(runner.delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return workerexecution.RunnerExecutionResult{}, ctx.Err()
		}
	}
	if runner.err != nil {
		return workerexecution.RunnerExecutionResult{}, runner.err
	}
	return workerexecution.RunnerExecutionResult{Content: runner.response}, nil
}

func TestLibraryHarnessAdapterSuccess(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
	result, err := adapter.Execute(context.Background(), HarnessInput{
		SystemPrompt: "system",
		UserMessage:  "hello",
		Inferencer:   staticInferencer{response: "done"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.FinalText != "done" {
		t.Fatalf("FinalText = %q, want done", result.FinalText)
	}
}

func TestLibraryHarnessAdapterCancellationStopsLoop(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
	_, err := adapter.Execute(ctx, HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{response: "ignored", delay: time.Second},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute error = %v, want context.Canceled", err)
	}
}

func TestLibraryHarnessAdapterFailure(t *testing.T) {
	t.Parallel()

	adapter := NewLibraryHarnessAdapter(localToolFileSystem{})
	_, err := adapter.Execute(context.Background(), HarnessInput{
		UserMessage: "hello",
		Inferencer:  staticInferencer{err: errors.New("model exploded")},
	})
	if err == nil {
		t.Fatal("Execute: expected error, got nil")
	}
}
