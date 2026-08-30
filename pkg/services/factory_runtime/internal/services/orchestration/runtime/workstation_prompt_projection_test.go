package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestRenderRuntimePromptUsesResolvedContextAndWorkInputTokens(t *testing.T) {
	t.Parallel()

	selection := &runtimeExecutionSelection{
		promptTemplate:   "authored prompt",
		workingDirectory: "authored-workdir",
		environment:      map[string]string{"TOKEN": "authored"},
		worktree:         "authored-worktree",
	}
	tokens := []workers.Token{
		{ID: "resource", Color: workers.Color{DataType: workers.DataTypeResource}},
		{ID: "work", Color: workers.Color{DataType: workers.DataTypeWork}},
	}
	callOrder := make([]string, 0, 2)
	fieldResolver := runtimeTemplateFieldResolverFunc(func(
		string,
		map[string]string,
		[]workers.Token,
		*workers.Context,
		string,
	) (*workers.ResolvedTemplateFields, error) {
		callOrder = append(callOrder, "resolve")
		return &workers.ResolvedTemplateFields{
			WorkingDirectory: "resolved-workdir",
			Env:              map[string]string{"TOKEN": "resolved"},
		}, nil
	})
	promptRenderer := runtimePromptRendererFunc(func(
		prompt string,
		tokens []workers.Token,
		context *workers.Context,
	) (string, error) {
		callOrder = append(callOrder, "render")
		if prompt != "authored prompt" {
			return "", errors.New("unexpected prompt")
		}
		if len(tokens) != 1 || tokens[0].ID != "work" {
			return "", errors.New("prompt received non-work tokens")
		}
		if context.WorkDirectory != "resolved-workdir" ||
			context.EnvVars["TOKEN"] != "resolved" ||
			context.EnvVars["BASE"] != "base" {
			return "", errors.New("prompt received unresolved execution context")
		}
		return "resolved prompt", nil
	})
	baseContext := &workers.Context{
		WorkDirectory: "base-workdir",
		EnvVars:       map[string]string{"BASE": "base"},
	}
	cfg := &runtimeConfig{
		promptRenderer:        promptRenderer,
		templateFieldResolver: fieldResolver,
	}
	if err := renderRuntimePrompt(cfg, selection, tokens, baseContext, nil, nil); err != nil {
		t.Fatalf("renderRuntimePrompt() error = %v", err)
	}
	if strings.Join(callOrder, ",") != "resolve,render" {
		t.Fatalf("render call order = %q, want resolve,render", strings.Join(callOrder, ","))
	}
	if selection.userMessage != "resolved prompt" {
		t.Fatalf("userMessage = %q, want resolved prompt", selection.userMessage)
	}
	if baseContext.WorkDirectory != "base-workdir" || baseContext.EnvVars["TOKEN"] != "" {
		t.Fatalf("base prompt context mutated = %#v", baseContext)
	}
}
