package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/prompting"
)

func (*Service) BuildPromptTemplateContract(
	inputCount int,
	docPaths []string,
) workers.PromptTemplateContract {
	return workerprompting.BuildPromptTemplateContract(inputCount, docPaths)
}

func (*Service) ValidatePromptTemplate(
	template string,
	inputCount int,
	docPaths []string,
) workers.PromptTemplateValidationResult {
	return workerprompting.ValidatePromptTemplate(template, inputCount, docPaths)
}

// ResolveTemplateFields exposes the Workers-owned template resolver to Wire
// without exposing the prompting implementation package.
func ResolveTemplateFields(
	workingDirectory string,
	environment map[string]string,
	tokens []workers.Token,
	workflowContext *workers.Context,
	worktree string,
) (*workers.ResolvedTemplateFields, error) {
	return workerprompting.ResolveTemplateFields(
		workingDirectory,
		environment,
		tokens,
		workflowContext,
		worktree,
	)
}

// RenderPrompt keeps authored-prompt rendering behind the Workers
// implementation boundary while exposing only the detached request contract
// to Factory Runtime.
func (s *Service) RenderPrompt(
	template string,
	tokens []workers.Token,
	workflowContext *workers.Context,
) (string, error) {
	if s == nil {
		return "", fmt.Errorf("render Worker prompt: service is unavailable")
	}
	renderer := &workerprompting.DefaultPromptRenderer{FactoryDocs: s.factoryDocs}
	return renderer.Render(template, tokens, workflowContext)
}
