package wire

import (
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func effectiveProviderCommandClock(edges serviceedges.Edges) platformclock.Source {
	return edges.Clock
}

// Operational waits use the scheduler already selected at the process boundary.
func effectiveProviderScheduler(edges serviceedges.Edges) platformclock.TimerSource {
	return edges.ProcessScheduler
}

func projectACPIntegrations(integrations []operatorsettings.ACPIntegration) []providers.ACPIntegration {
	result := make([]providers.ACPIntegration, 0, len(integrations))
	for _, integration := range integrations {
		result = append(result, providers.ACPIntegration{
			ID: integration.ID, Name: providers.ID(integration.Name),
			Transport: integration.Transport, Command: integration.Command,
		})
	}
	return result
}
