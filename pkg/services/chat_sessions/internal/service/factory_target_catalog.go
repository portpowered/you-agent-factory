package service

import (
	"context"
	"sort"
	"strings"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// ResolveFactoryTargetCatalog resolves the effective ACP Agent profile
// through Operator Settings, reads the installed Factory catalog through
// Factory Definitions, intersects the profile's allowlist with that catalog,
// and returns the deduplicated, stably ordered FACTORY choices plus one
// current/default target drawn from the intersection. Every call reads
// current collaborator state; nothing is persisted, cached, or reused across
// calls.
func (s *Service) ResolveFactoryTargetCatalog(
	ctx context.Context,
	req chatsessions.ResolveFactoryTargetCatalogRequest,
) (chatsessions.ResolveFactoryTargetCatalogResult, error) {
	profile, err := s.operatorSettings.ResolveACPAgentProfile(req.OperatorSettingsPath)
	if err != nil {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Err: chatsessions.ErrFactoryTargetProfileUnavailable,
		}
	}

	listing, err := s.factoryDefinitions.ListEffectiveFactories(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: req.FactoryDiscovery.ProjectRoot,
		GlobalRoot:  req.FactoryDiscovery.GlobalRoot,
	})
	if err != nil {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Err: chatsessions.ErrFactoryTargetCatalogUnavailable,
		}
	}

	installed := make(map[string]factorydefinitions.EffectiveFactoryCatalogEntry, len(listing.Entries))
	for _, entry := range listing.Entries {
		installed[entry.Name] = entry
	}

	choicesByValue := make(map[string]chatsessions.FactoryTargetCatalogChoice, len(profile.AllowedTargets))
	for _, target := range profile.AllowedTargets {
		if _, exists := choicesByValue[target]; exists {
			continue
		}
		name := strings.TrimPrefix(target, operatorsettings.ACPFactoryTargetNamespace)
		entry, ok := installed[name]
		if !ok {
			continue
		}
		choicesByValue[target] = chatsessions.FactoryTargetCatalogChoice{
			Value: target,
			Name:  factoryDisplayName(entry),
		}
	}

	current := req.CurrentTarget
	if current == "" {
		current = profile.DefaultTarget
	}
	currentChoice, ok := choicesByValue[current]
	if !ok {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetCurrentUnavailable,
		}
	}

	return chatsessions.ResolveFactoryTargetCatalogResult{
		Choices:       orderedFactoryTargetChoices(choicesByValue, currentChoice),
		CurrentTarget: currentChoice.Value,
	}, nil
}

// orderedFactoryTargetChoices returns current first, followed by every other
// choice in ascending canonical target-ref order, independent of the
// supplied map's iteration order.
func orderedFactoryTargetChoices(
	choicesByValue map[string]chatsessions.FactoryTargetCatalogChoice,
	current chatsessions.FactoryTargetCatalogChoice,
) []chatsessions.FactoryTargetCatalogChoice {
	remaining := make([]chatsessions.FactoryTargetCatalogChoice, 0, len(choicesByValue))
	for value, choice := range choicesByValue {
		if value == current.Value {
			continue
		}
		remaining = append(remaining, choice)
	}
	sort.Slice(remaining, func(i, j int) bool { return remaining[i].Value < remaining[j].Value })

	ordered := make([]chatsessions.FactoryTargetCatalogChoice, 0, len(choicesByValue))
	ordered = append(ordered, current)
	return append(ordered, remaining...)
}

// factoryDisplayName returns a non-empty stable display name derived from
// Factory Definitions-owned canonical identity facts: the Factory
// definition's authored Name when present, otherwise the catalog entry's
// canonical name.
func factoryDisplayName(entry factorydefinitions.EffectiveFactoryCatalogEntry) string {
	if entry.Definition != nil {
		if trimmed := strings.TrimSpace(entry.Definition.Name); trimmed != "" {
			return trimmed
		}
	}
	return entry.Name
}
