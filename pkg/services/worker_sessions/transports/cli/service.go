// Package cli owns the Worker Sessions service CLI adapter.
package cli

// Service exposes Worker Sessions CLI operations to Cobra composition.
type Service interface {
	List(ListConfig) error
}

type service struct{}

// New constructs the Worker Sessions CLI adapter.
func New() Service { return service{} }

func (service) List(config ListConfig) error { return list(config) }
