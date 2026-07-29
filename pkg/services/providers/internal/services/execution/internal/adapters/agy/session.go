package agy

import (
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func sessionRefFromRequest(resume *providers.SessionRef) *providers.SessionRef {
	if resume == nil {
		return nil
	}
	sessionID := strings.TrimSpace(resume.ID)
	if sessionID == "" {
		return nil
	}
	provider := resume.Provider
	if strings.TrimSpace(string(provider)) == "" {
		provider = providers.IDAntigravity
	}
	kind := strings.TrimSpace(resume.Kind)
	if kind == "" {
		kind = providers.SessionIDKind
	}
	ref := providers.SessionRef{
		Provider: provider,
		Kind:     kind,
		ID:       sessionID,
	}
	if err := ref.Validate(); err != nil {
		return nil
	}
	cloned := ref.Clone()
	return &cloned
}
