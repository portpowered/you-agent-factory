// Package builtins declares the parent-private packaged Providers catalog.
package builtins

import providers "github.com/portpowered/infinite-you/pkg/services/providers"

// Service exposes immutable packaged provider integrations to the Providers
// composition boundary. Returned values are detached from service state.
type Service interface {
	ACPIntegrations() []providers.ACPIntegration
}
