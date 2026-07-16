package agentrun

import (
	"context"
	"fmt"
	"strings"

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/agentloop"
	"github.com/portpowered/go-agent-harness/go-agent-loop/pkg/messages"
)

// HarnessInput is the factory-owned input for one agent-run harness execution.
type HarnessInput struct {
	SystemPrompt string
	UserMessage  string
	Inferencer   messages.Inferencer
	ToolPolicy   string
	WorkingDir   string
	ToolRecorder *ToolDiagnosticRecorder
}

// HarnessResult is the factory-owned result from one harness execution turn.
type HarnessResult struct {
	FinalText string
	Messages  []messages.Message
	Err       error
}

// HarnessAdapter executes one agent loop through go-agent-harness.
type HarnessAdapter interface {
	Execute(ctx context.Context, input HarnessInput) (HarnessResult, error)
}

// LibraryHarnessAdapter runs agent loops through the go-agent-harness library.
type LibraryHarnessAdapter struct{}

func NewLibraryHarnessAdapter() *LibraryHarnessAdapter {
	return &LibraryHarnessAdapter{}
}

func (a *LibraryHarnessAdapter) Execute(ctx context.Context, input HarnessInput) (HarnessResult, error) {
	if input.Inferencer == nil {
		return HarnessResult{}, fmt.Errorf("agent run harness requires an inferencer")
	}

	policy := workerconfig.NormalizeAgentToolPolicy(input.ToolPolicy)
	opts := []agentloop.Option{
		agentloop.WithInferencer(input.Inferencer),
	}
	if workerconfig.AgentToolsAllowExecution(policy) {
		recorder := input.ToolRecorder
		if recorder == nil {
			recorder = NewToolDiagnosticRecorder()
		}
		opts = append(opts,
			agentloop.WithToolExecutor(NewPolicyToolExecutor(policy, input.WorkingDir, recorder)),
			agentloop.WithTools(toolDefinitionsForPolicy(policy)),
		)
	} else {
		opts = append(opts, agentloop.WithToolExecutionDisabled())
	}
	if systemPrompt := strings.TrimSpace(input.SystemPrompt); systemPrompt != "" {
		opts = append(opts, agentloop.WithSystemPrompt(systemPrompt))
	}

	loop, err := agentloop.New(opts...)
	if err != nil {
		return HarnessResult{}, fmt.Errorf("create agent loop: %w", err)
	}

	result, err := loop.Execute(ctx, agentloop.NewExecuteInput(input.UserMessage))
	final := result.FinalText()
	harnessResult := HarnessResult{
		FinalText: final.Text,
		Messages:  result.Messages,
		Err:       firstNonNilError(final.Err, err),
	}
	if harnessResult.Err != nil {
		return harnessResult, harnessResult.Err
	}
	if final.Status != agentloop.FinalTextSuccess && final.Status != agentloop.FinalTextEmptySuccess {
		if harnessResult.Err == nil {
			harnessResult.Err = fmt.Errorf("agent loop finished with status %s", final.Status)
		}
		return harnessResult, harnessResult.Err
	}
	return harnessResult, nil
}

func firstNonNilError(values ...error) error {
	for _, err := range values {
		if err != nil {
			return err
		}
	}
	return nil
}
