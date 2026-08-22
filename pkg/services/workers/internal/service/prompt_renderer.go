package service

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/prompting"
)

// RenderPrompt exposes the canonical authored-prompt renderer as an optional
// detached Workers capability. It has no session or runtime lookup path.
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
