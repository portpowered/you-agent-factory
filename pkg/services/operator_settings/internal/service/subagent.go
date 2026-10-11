package service

import (
	"context"
	"fmt"
	"strings"

	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (s *Service) ResolveSubagentRoute(ctx context.Context, path string, overrides operatorsettings.SubagentRouteUpdate) (operatorsettings.SubagentRoute, error) {
	s.logger.Info("operator_settings.resolve_subagent_route.started")
	route, err := s.resolveSubagentRoute(ctx, path, overrides)
	if err != nil {
		// Do not log model identities, paths, or authored configuration values.
		s.logger.Warn("operator_settings.resolve_subagent_route.failed", "reason", "route_resolution_failed")
		return operatorsettings.SubagentRoute{}, err
	}
	s.logger.Info("operator_settings.resolve_subagent_route.finished")
	return route, nil
}

func (s *Service) resolveSubagentRoute(ctx context.Context, path string, overrides operatorsettings.SubagentRouteUpdate) (operatorsettings.SubagentRoute, error) {
	if ctx == nil {
		return operatorsettings.SubagentRoute{}, subagentResolutionError("context", "request context is required")
	}
	if err := ctx.Err(); err != nil {
		return operatorsettings.SubagentRoute{}, err
	}
	loaded, err := s.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.SubagentRoute{}, err
	}
	if err := ctx.Err(); err != nil {
		return operatorsettings.SubagentRoute{}, err
	}
	return resolveSubagentOverrides(loaded.Document.Defaults.Subagent, overrides)
}

func resolveSubagentOverrides(authored *operatorsettings.SubagentRoute, overrides operatorsettings.SubagentRouteUpdate) (operatorsettings.SubagentRoute, error) {
	baseline, err := authored.Normalize()
	if err != nil {
		return operatorsettings.SubagentRoute{}, subagentResolutionError("defaults.subagent", "invalid dedicated default; repair defaults.subagent")
	}
	if overrides.Provider != nil {
		provider, valid := definitions.CanonicalizeOperatorWorkerModelProviderInput(*overrides.Provider)
		if !valid || strings.TrimSpace(*overrides.Provider) == "" || definitions.IsSymbolicWorkerModelProviderDefault(provider) {
			return operatorsettings.SubagentRoute{}, subagentResolutionError("provider", "select a concrete provider")
		}
		if (baseline == nil || provider != baseline.Provider) && overrides.Model == nil {
			return operatorsettings.SubagentRoute{}, subagentResolutionError("model", "an explicit model is required when selecting a different provider")
		}
	}
	if baseline == nil && (overrides.Provider == nil || overrides.Model == nil) {
		return operatorsettings.SubagentRoute{}, subagentResolutionError("defaults.subagent", "configure defaults.subagent.provider/model or supply both provider and model")
	}
	route, err := baseline.Merge(overrides)
	if err != nil {
		return operatorsettings.SubagentRoute{}, fmt.Errorf("%w: invalid subagent route; supply non-empty provider/model and a supported reasoningEffort", operatorsettings.ErrResolutionInvalidInput)
	}
	return *route, nil
}

func subagentResolutionError(field, message string) error {
	return operatorsettings.ResolutionFailure{Kind: operatorsettings.ResolutionFailureKindInvalidInput, Field: field, Message: message}
}
