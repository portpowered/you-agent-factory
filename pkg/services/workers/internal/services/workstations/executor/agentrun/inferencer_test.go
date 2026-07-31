package agentrun

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

type sequenceRunner struct {
	mu       sync.Mutex
	errors   []error
	response string
	calls    int
}

func (runner *sequenceRunner) Execute(_ context.Context, _ workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls++
	if len(runner.errors) > 0 {
		err := runner.errors[0]
		runner.errors = runner.errors[1:]
		if err != nil {
			return workerexecution.RunnerExecutionResult{}, err
		}
	}
	return workerexecution.RunnerExecutionResult{Content: runner.response}, nil
}

func TestRunnerInferencer_UsesConversationPrompt(t *testing.T) {
	t.Parallel()

	runner := &stubRunner{response: "assistant reply"}
	inferencer := newRunnerInferencer(runner, workerexecution.ProviderInferenceRequest{
		SystemPrompt: "base system",
		UserMessage:  "fallback",
	})
	result, err := inferencer.Infer(context.Background(), messages.InferenceRequest{
		Messages: []messages.Message{
			messages.NewTextMessage(messages.RoleSystem, "runtime system"),
			messages.NewTextMessage(messages.RoleUser, "first question"),
			messages.NewTextMessage(messages.RoleAssistant, "first answer"),
			messages.NewTextMessage(messages.RoleUser, "follow up"),
		},
	})
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}
	if result.Message.TextContent() != "assistant reply" {
		t.Fatalf("content = %q, want assistant reply", result.Message.TextContent())
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
}

func TestRunnerInferencer_InferStreamEmitsDeltas(t *testing.T) {
	t.Parallel()

	inferencer := newRunnerInferencer(&stubRunner{response: "streamed"}, workerexecution.ProviderInferenceRequest{
		UserMessage: "hello",
	})
	ch, err := inferencer.InferStream(context.Background(), messages.InferenceRequest{
		Messages: []messages.Message{messages.NewTextMessage(messages.RoleUser, "hello")},
	})
	if err != nil {
		t.Fatalf("InferStream: %v", err)
	}
	var sawText bool
	for msg := range ch {
		if msg.Type == messages.StreamTypeTextDelta {
			sawText = true
		}
	}
	if !sawText {
		t.Fatal("expected streamed text delta")
	}
}

func TestRunnerInferencer_RetriesTransientProviderFailure(t *testing.T) {
	t.Parallel()

	runner := &sequenceRunner{
		errors: []error{workerexecution.NewProviderError(
			workerexecution.WorkFailureTypeInternalServerError,
			"temporary provider output failure",
			errors.New("temporary provider output failure"),
		)},
		response: "recovered",
	}
	inferencer := newRunnerInferencer(runner, workerexecution.ProviderInferenceRequest{}).(*runnerInferencer)
	inferencer.retrySleep = func(context.Context, time.Duration) error { return nil }

	result, err := inferencer.Infer(context.Background(), messages.InferenceRequest{})
	if err != nil || result.Message.TextContent() != "recovered" {
		t.Fatalf("Infer() = %#v, %v, want recovered response", result, err)
	}
	if runner.calls != 2 {
		t.Fatalf("runner calls = %d, want initial call plus one retry", runner.calls)
	}
}

func TestRunnerInferencer_BoundsPersistentProviderFailure(t *testing.T) {
	t.Parallel()

	failure := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypeInternalServerError,
		"temporary provider output failure",
		errors.New("temporary provider output failure"),
	)
	runner := &sequenceRunner{errors: []error{failure, failure, failure, failure}}
	inferencer := newRunnerInferencer(runner, workerexecution.ProviderInferenceRequest{}).(*runnerInferencer)
	inferencer.retrySleep = func(context.Context, time.Duration) error { return nil }

	if _, err := inferencer.Infer(context.Background(), messages.InferenceRequest{}); !errors.Is(err, failure) {
		t.Fatalf("Infer() error = %v, want persistent provider failure", err)
	}
	if runner.calls != 3 {
		t.Fatalf("runner calls = %d, want three bounded attempts", runner.calls)
	}
}

func TestRunnerInferencer_DoesNotRetryTerminalProviderFailure(t *testing.T) {
	t.Parallel()

	failure := workerexecution.NewProviderError(
		workerexecution.WorkFailureTypePermanentBadRequest,
		"invalid request",
		errors.New("invalid request"),
	)
	runner := &sequenceRunner{errors: []error{failure}, response: "must not run"}
	inferencer := newRunnerInferencer(runner, workerexecution.ProviderInferenceRequest{}).(*runnerInferencer)
	inferencer.retrySleep = func(context.Context, time.Duration) error { return nil }

	if _, err := inferencer.Infer(context.Background(), messages.InferenceRequest{}); !errors.Is(err, failure) {
		t.Fatalf("Infer() error = %v, want terminal provider failure", err)
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want no retry", runner.calls)
	}
}
