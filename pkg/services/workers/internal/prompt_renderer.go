package internal

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

// RenderPrompt keeps the canonical authored-prompt renderer behind the
// Workers implementation boundary while exposing only its detached request
// contract to Factory Runtime.
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
