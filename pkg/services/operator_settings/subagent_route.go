package operatorsettings

import (
	"fmt"
	"strings"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// SubagentRoute is the dedicated file-only default for direct subagents.
// It never inherits generic worker defaults or environment overrides.
type SubagentRoute struct {
	Provider        string
	Model           string
	ReasoningEffort string
}

// SubagentRouteUpdate preserves omitted values in an existing route.
type SubagentRouteUpdate struct {
	Provider        *string
	Model           *string
	ReasoningEffort *string
}

func (route *SubagentRoute) Clone() *SubagentRoute {
	if route == nil {
		return nil
	}
	cloned := *route
	return &cloned
}

// Normalize accepts only a concrete provider and a non-empty model.
// Model identities remain opaque; Providers owns availability checks.
func (route *SubagentRoute) Normalize() (*SubagentRoute, error) {
	if route == nil {
		return nil, nil
	}
	provider, ok := definitions.CanonicalizeOperatorWorkerModelProviderInput(route.Provider)
	if strings.TrimSpace(route.Provider) == "" || !ok || definitions.IsSymbolicWorkerModelProviderDefault(provider) {
		return nil, fmt.Errorf("%w: defaults.subagent.provider must be a concrete provider", ErrDocumentMalformed)
	}
	model := strings.TrimSpace(route.Model)
	if model == "" {
		return nil, fmt.Errorf("%w: defaults.subagent.model is required", ErrDocumentMalformed)
	}
	effort, ok := definitions.CanonicalizeReasoningEffort(route.ReasoningEffort)
	if !ok {
		return nil, fmt.Errorf("%w: defaults.subagent.reasoningEffort is invalid", ErrDocumentMalformed)
	}
	return &SubagentRoute{Provider: provider, Model: model, ReasoningEffort: effort}, nil
}

// Merge validates the complete record before the document owner persists it.
func (route *SubagentRoute) Merge(update SubagentRouteUpdate) (*SubagentRoute, error) {
	next := SubagentRoute{}
	if route != nil {
		next = *route
	}
	fields := []struct {
		source *string
		target *string
	}{
		{update.Provider, &next.Provider}, {update.Model, &next.Model}, {update.ReasoningEffort, &next.ReasoningEffort},
	}
	for _, field := range fields {
		if field.source == nil {
			continue
		}
		if strings.TrimSpace(*field.source) == "" {
			return nil, fmt.Errorf("%w: subagent route values must be non-empty", ErrDocumentMalformed)
		}
		*field.target = *field.source
	}
	return next.Normalize()
}
