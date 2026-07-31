package providers

import "context"

// ACPConfiguration updates the effective operator-configured ACP integrations
// on the singular Providers root without reconstructing the application graph.
type ACPConfiguration interface {
	ConfigureACPIntegrations(context.Context, []ACPIntegration) error
}
