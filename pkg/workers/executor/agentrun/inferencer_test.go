package agentrun

import (
	"context"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

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
