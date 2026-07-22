package agentrun

import (
	"context"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

type runnerInferencer struct {
	runner  runnerContract
	baseReq workerexecution.ProviderInferenceRequest
}

type runnerContract interface {
	Execute(ctx context.Context, request workerexecution.RunnerExecutionRequest) (workerexecution.RunnerExecutionResult, error)
}

func newRunnerInferencer(runner runnerContract, baseReq workerexecution.ProviderInferenceRequest) messages.Inferencer {
	return &runnerInferencer{runner: runner, baseReq: baseReq}
}

func (i *runnerInferencer) Infer(ctx context.Context, req messages.InferenceRequest) (messages.InferenceResult, error) {
	runnerReq := i.baseReq
	systemPrompt, userMessage := conversationPrompt(i.baseReq.SystemPrompt, req.Messages)
	runnerReq.SystemPrompt = systemPrompt
	runnerReq.UserMessage = userMessage

	resp, err := i.runner.Execute(ctx, runnerReq)
	if err != nil {
		return messages.InferenceResult{}, err
	}
	return messages.InferenceResult{
		Message: messages.NewTextMessage(messages.RoleAssistant, resp.Content),
	}, nil
}

func (i *runnerInferencer) InferStream(ctx context.Context, req messages.InferenceRequest) (<-chan messages.StreamMessage, error) {
	result, err := i.Infer(ctx, req)
	ch := make(chan messages.StreamMessage, 4)
	go func() {
		defer close(ch)
		if err != nil {
			ch <- messages.StreamMessage{
				Type:  messages.StreamTypeError,
				Value: messages.NewErrorValue(err.Error()),
			}
			return
		}
		text := result.Message.TextContent()
		ch <- messages.StreamMessage{Type: messages.StreamTypeTextStart, Value: messages.NewTextStartValue(), Role: messages.RoleAssistant}
		if text != "" {
			ch <- messages.StreamMessage{Type: messages.StreamTypeTextDelta, Value: messages.NewTextDeltaValue(text), Role: messages.RoleAssistant}
		}
		ch <- messages.StreamMessage{Type: messages.StreamTypeTextEnd, Value: messages.NewTextEndValue(), Role: messages.RoleAssistant}
		ch <- messages.StreamMessage{Type: messages.StreamTypeMessageEnd, Value: messages.NewMessageEndValue(messages.TokenUsage{}), Role: messages.RoleAssistant}
	}()
	return ch, nil
}

func conversationPrompt(defaultSystem string, history []messages.Message) (systemPrompt string, userMessage string) {
	systemPrompt = strings.TrimSpace(defaultSystem)
	var conversation []string
	for _, msg := range history {
		text := strings.TrimSpace(msg.TextContent())
		if text == "" {
			continue
		}
		switch msg.Role {
		case messages.RoleSystem:
			if systemPrompt == "" {
				systemPrompt = text
			}
		case messages.RoleUser:
			conversation = append(conversation, "User: "+text)
		case messages.RoleAssistant:
			conversation = append(conversation, "Assistant: "+text)
		case messages.RoleTool:
			conversation = append(conversation, "Tool: "+text)
		}
	}
	if len(conversation) == 0 {
		for i := len(history) - 1; i >= 0; i-- {
			if history[i].Role == messages.RoleUser {
				return systemPrompt, history[i].TextContent()
			}
		}
		return systemPrompt, ""
	}
	return systemPrompt, strings.Join(conversation, "\n\n")
}
