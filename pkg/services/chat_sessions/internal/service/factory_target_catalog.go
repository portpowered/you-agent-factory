package service

import (
	"context"
	"errors"
	"path/filepath"
	"regexp"
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

// dependencyContextCause returns context.Canceled or context.DeadlineExceeded
// when err is classifiable as one, and nil otherwise. It never returns err
// itself: only these two safe, text-free sentinels are ever attached to a
// public FactoryTargetCatalogError's Cause field, so an arbitrary dependency
// error can never leak through it.
func dependencyContextCause(err error) error {
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	}
	return nil
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
			Err:   chatsessions.ErrFactoryTargetProfileUnavailable,
			Cause: dependencyContextCause(err),
		}
	}

	installed, err := s.resolveInstalledFactories(ctx, req.FactoryDiscovery)
	if err != nil {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, err
	}

	choicesByValue := allowedInstalledChoices(profile, installed)
	if len(choicesByValue) == 0 {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Err: chatsessions.ErrFactoryTargetCatalogEmpty,
		}
	}

	current := req.CurrentTarget
	callerSuppliedTarget := current != ""
	if !callerSuppliedTarget {
		current = profile.DefaultTarget
		// An unrestricted profile carries the built-in default target, which
		// the operator never chose and may not have installed (a home where
		// initialization never ran, or where Factory Builder was deleted).
		// Falling back to the first selectable choice keeps session/new
		// working there. A caller-supplied target, and an operator who
		// authored an allowlist, both named a target deliberately and still
		// get the strict not-installed error below.
		if profile.IsUnrestricted() {
			if _, ok := choicesByValue[current]; !ok {
				current = firstFactoryTargetValue(choicesByValue)
			}
		}
	}
	if !isWellFormedFactoryTargetReference(current) {
		// current has not passed lexical validation: it may be arbitrary
		// caller-supplied input (a path, credential-like value, or control
		// text), so it is never copied into the public error's Target field.
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Err: chatsessions.ErrFactoryTargetReferenceMalformed,
		}
	}

	bareName := strings.TrimPrefix(current, operatorsettings.ACPFactoryTargetNamespace)
	if _, installedOK := installed[bareName]; !installedOK {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetNotInstalled,
		}
	}
	// Membership in the projected catalog, not in the raw allowlist: this is
	// the same check for a restricted profile, and correct for an unrestricted
	// one. ErrFactoryTargetNotAllowed therefore stays reachable only when a
	// restriction was actually authored.
	if _, allowedOK := choicesByValue[current]; !allowedOK {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetNotAllowed,
		}
	}

	if err := s.validateWorkingRootCompatibility(ctx, req, bareName, current); err != nil {
		return chatsessions.ResolveFactoryTargetCatalogResult{}, err
	}

	currentChoice := choicesByValue[current]
	return chatsessions.ResolveFactoryTargetCatalogResult{
		Choices:       orderedFactoryTargetChoices(choicesByValue, currentChoice, profile.AllowedTargets),
		CurrentTarget: currentChoice.Value,
	}, nil
}

// resolveInstalledFactories reads the effective Factory catalog and returns
// only entries that are actually installed (materialized to a filesystem
// location), keyed by bare catalog name. A packaged definition that has not
// been materialized carries a nil Location and is never treated as
// installed, even though it is still effective/listable.
func (s *Service) resolveInstalledFactories(
	ctx context.Context,
	discovery chatsessions.FactoryDiscoveryRoots,
) (map[string]factorydefinitions.EffectiveFactoryCatalogEntry, error) {
	listing, err := s.factoryDefinitions.ListEffectiveFactories(ctx, factorydefinitions.ListEffectiveFactoriesRequest{
		ProjectRoot: discovery.ProjectRoot,
		GlobalRoot:  discovery.GlobalRoot,
	})
	if err != nil {
		return nil, &chatsessions.FactoryTargetCatalogError{
			Err:   chatsessions.ErrFactoryTargetCatalogUnavailable,
			Cause: dependencyContextCause(err),
		}
	}

	installed := make(map[string]factorydefinitions.EffectiveFactoryCatalogEntry, len(listing.Entries))
	for _, entry := range listing.Entries {
		if entry.Location == nil {
			continue
		}
		installed[entry.Name] = entry
	}
	return installed, nil
}

// allowedInstalledChoices projects the selectable FACTORY choices for a
// profile, deduplicating on target value and preserving each installed entry's
// display name.
//
// An unrestricted profile (no authored allowlist) offers every installed
// Factory whose canonical name can be expressed as a well-formed factory:
// target reference. That filter is load-bearing rather than defensive: the
// installed catalog also holds project-local and operator-authored Factories
// with arbitrary names, and session/set_config_option revalidates whatever the
// client picks through the same grammar. Emitting an inexpressible name would
// produce a picker entry the client can select but the server must then
// reject.
func allowedInstalledChoices(
	profile operatorsettings.ACPAgentProfile,
	installed map[string]factorydefinitions.EffectiveFactoryCatalogEntry,
) map[string]chatsessions.FactoryTargetCatalogChoice {
	if profile.IsUnrestricted() {
		choicesByValue := make(map[string]chatsessions.FactoryTargetCatalogChoice, len(installed))
		for name, entry := range installed {
			target := operatorsettings.ACPFactoryTargetNamespace + name
			if !isWellFormedFactoryTargetReference(target) {
				continue
			}
			choicesByValue[target] = chatsessions.FactoryTargetCatalogChoice{
				Value: target,
				Name:  factoryDisplayName(entry),
			}
		}
		return choicesByValue
	}

	allowedTargets := profile.AllowedTargets
	choicesByValue := make(map[string]chatsessions.FactoryTargetCatalogChoice, len(allowedTargets))
	for _, target := range allowedTargets {
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
	return choicesByValue
}

// validateWorkingRootCompatibility rejects a project-local canonical
// resolution whose project root disagrees with the caller's client working
// root. It is a no-op when the caller supplies no client working root.
//
// req.FactoryDiscovery.ProjectRoot (and therefore, when the resolution is
// project-local, resolved.Resolution.ProjectRoot, which callers such as
// factorydefinitions.ResolveNamedFactory always echo from it unchanged) is
// always the project-scoped Factory root derived from a working directory
// via factorydefinitions.ProjectFactoriesRoot -- i.e. "<workingDir>/factory",
// never the bare working directory itself. ClientWorkingRoot, in contrast, is
// documented and populated by every caller (see
// pkg/transports/acp/internal/stdio/session_new.go and
// session_set_config_option.go) as the client's own bare working directory.
// Comparing the two directly would always disagree for a project-local
// resolution regardless of whether the client is actually working inside
// that project, so clientRoot is projected through the same
// ProjectFactoriesRoot derivation before comparison.
func (s *Service) validateWorkingRootCompatibility(
	ctx context.Context,
	req chatsessions.ResolveFactoryTargetCatalogRequest,
	bareName string,
	current string,
) error {
	clientRoot := strings.TrimSpace(req.ClientWorkingRoot)
	if clientRoot == "" {
		return nil
	}

	resolved, err := s.factoryDefinitions.ResolveNamedFactory(ctx, factorydefinitions.ResolveNamedFactoryRequest{
		ProjectRoot: req.FactoryDiscovery.ProjectRoot,
		GlobalRoot:  req.FactoryDiscovery.GlobalRoot,
		Name:        bareName,
	})
	if err != nil {
		return &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetCatalogUnavailable,
			Cause:  dependencyContextCause(err),
		}
	}
	clientProjectRoot := filepath.Clean(factorydefinitions.ProjectFactoriesRoot(clientRoot))
	if resolved.Resolution.Source == factorydefinitions.NamedFactoryResolutionSourceProjectLocal &&
		clientProjectRoot != filepath.Clean(resolved.Resolution.ProjectRoot) {
		return &chatsessions.FactoryTargetCatalogError{
			Target: current,
			Err:    chatsessions.ErrFactoryTargetWorkingRootIncompatible,
		}
	}
	return nil
}

// firstFactoryTargetValue returns the lexically smallest target value in the
// projected catalog, independent of map iteration order. Callers use it to
// pick a deterministic current target when the profile's default is not
// selectable.
func firstFactoryTargetValue(
	choicesByValue map[string]chatsessions.FactoryTargetCatalogChoice,
) string {
	first := ""
	for value := range choicesByValue {
		if first == "" || value < first {
			first = value
		}
	}
	return first
}

// orderedFactoryTargetChoices returns the projected catalog in the order an
// ACP client renders it.
//
// An operator who authored allowedTargets chose that order, and
// docs/reference/serve-acp.md promises it back to them: a non-empty list is
// offered "in the authored order". It is preserved exactly, including which
// entry comes first -- re-sorting it, or hoisting the current target, would
// silently discard the only reason to author a list rather than take the
// unrestricted default.
//
// An unrestricted profile has no authored order to preserve, so this keeps
// the deterministic one it has always used: the current target first, then
// every other installed Factory in canonical reference order, independent of
// the supplied map's iteration order.
func orderedFactoryTargetChoices(
	choicesByValue map[string]chatsessions.FactoryTargetCatalogChoice,
	current chatsessions.FactoryTargetCatalogChoice,
	authoredOrder []string,
) []chatsessions.FactoryTargetCatalogChoice {
	if len(authoredOrder) > 0 {
		ordered := make([]chatsessions.FactoryTargetCatalogChoice, 0, len(choicesByValue))
		emitted := make(map[string]struct{}, len(choicesByValue))
		for _, value := range authoredOrder {
			// An authored target that is not installed was already dropped
			// from the projected catalog; it is skipped rather than offered.
			choice, ok := choicesByValue[value]
			if !ok {
				continue
			}
			// Operator Settings rejects a duplicated allowlist entry, so this
			// only guards a profile reaching here without that normalization.
			if _, seen := emitted[value]; seen {
				continue
			}
			emitted[value] = struct{}{}
			ordered = append(ordered, choice)
		}
		return ordered
	}

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
