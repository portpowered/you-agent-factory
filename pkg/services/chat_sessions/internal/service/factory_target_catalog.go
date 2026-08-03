package service

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// factoryTargetReferencePattern mirrors Operator Settings' local factory:
// target-reference grammar (pkg/services/operator_settings/acp_agent_profile.go):
// one or more lowercase kebab-case path segments separated by '/', with an
// optional leading '@' scope marker on the first segment. It intentionally
// has no room for a version or digest pin, so a version- or digest-pinned
// reference is rejected the same way as any other malformed shape. Operator
// Settings only validates profile-sourced references (its own AllowedTargets
// and DefaultTarget); this package validates a caller-supplied CurrentTarget
// the same way, since that value never passes through Operator Settings'
// normalization.
var factoryTargetReferencePattern = regexp.MustCompile(
	`^@?[a-z0-9]+(?:-[a-z0-9]+)*(?:/[a-z0-9]+(?:-[a-z0-9]+)*)+$`,
)

func isWellFormedFactoryTargetReference(value string) bool {
	if !strings.HasPrefix(value, operatorsettings.ACPFactoryTargetNamespace) {
		return false
	}
	suffix := strings.TrimPrefix(value, operatorsettings.ACPFactoryTargetNamespace)
	return factoryTargetReferencePattern.MatchString(suffix)
}

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
	s.logger.Info("chat_sessions.resolve_factory_target_catalog.started")
	result, err := s.resolveFactoryTargetCatalog(ctx, req)
	if err != nil {
		s.logger.Warn(
			"chat_sessions.resolve_factory_target_catalog.failed",
			"reason", classifyFactoryTargetCatalogFailure(err),
		)
		return chatsessions.ResolveFactoryTargetCatalogResult{}, err
	}
	s.logger.Info(
		"chat_sessions.resolve_factory_target_catalog.finished",
		"choice_count", len(result.Choices),
	)
	return result, nil
}

// classifyFactoryTargetCatalogFailure returns a stable, safe reason label for
// operation logs. It never echoes a raw target, path, or collaborator error
// message, only the classification sentinel the failure unwraps to.
func classifyFactoryTargetCatalogFailure(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "context_deadline_exceeded"
	case errors.Is(err, chatsessions.ErrFactoryTargetProfileUnavailable):
		return "profile_unavailable"
	case errors.Is(err, chatsessions.ErrFactoryTargetCatalogUnavailable):
		return "catalog_unavailable"
	case errors.Is(err, chatsessions.ErrFactoryTargetReferenceMalformed):
		return "reference_malformed"
	case errors.Is(err, chatsessions.ErrFactoryTargetCatalogEmpty):
		return "catalog_empty"
	case errors.Is(err, chatsessions.ErrFactoryTargetNotInstalled):
		return "target_not_installed"
	case errors.Is(err, chatsessions.ErrFactoryTargetNotAllowed):
		return "target_not_allowed"
	case errors.Is(err, chatsessions.ErrFactoryTargetWorkingRootIncompatible):
		return "working_root_incompatible"
	}
	return "operation_failed"
}

func (s *Service) resolveFactoryTargetCatalog(
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
	if !isWellFormedFactoryTargetReference(current) {
		// current has not passed lexical validation: it may be arbitrary
		// caller-supplied input (a path, credential-like value, or control
		// text), so it is never copied into the public error's Target field.
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Err: chatsessions.ErrFactoryTargetReferenceMalformed,
		}
	}
	if len(choicesByValue) == 0 {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Err: chatsessions.ErrFactoryTargetCatalogEmpty,
		}
	}

	bareName := strings.TrimPrefix(current, operatorsettings.ACPFactoryTargetNamespace)
	if _, installedOK := installed[bareName]; !installedOK {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetNotInstalled,
		}
	}
	if !slices.Contains(profile.AllowedTargets, current) {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetNotAllowed,
		}
	}

	if clientRoot := strings.TrimSpace(req.ClientWorkingRoot); clientRoot != "" {
		resolved, err := s.factoryDefinitions.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{
			ProjectRoot: req.FactoryDiscovery.ProjectRoot,
			GlobalRoot:  req.FactoryDiscovery.GlobalRoot,
			Name:        bareName,
		})
		if err != nil {
			return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
				Target: current,
				Err:    chatsessions.ErrFactoryTargetCatalogUnavailable,
			}
		}
		if resolved.Resolution.Source == factorydefinitions.NamedFactoryResolutionSourceProjectLocal &&
			filepath.Clean(clientRoot) != filepath.Clean(resolved.Resolution.ProjectRoot) {
			return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
				Target: current,
				Err:    chatsessions.ErrFactoryTargetWorkingRootIncompatible,
			}
		}
	}

	currentChoice := choicesByValue[current]
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
