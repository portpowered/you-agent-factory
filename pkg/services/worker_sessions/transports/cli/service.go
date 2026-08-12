// Package cli owns the Worker Sessions service CLI adapter.
package cli

// Service exposes Worker Sessions CLI operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	Show(ShowConfig) error
	Read(ReadConfig) error
	Continue(ContinueConfig) error
}

type service struct{}

// New constructs the Worker Sessions CLI adapter.
func New() Service { return service{} }

func (service) List(config ListConfig) error { return list(config) }

func (service) Show(config ShowConfig) error { return show(config) }

func (service) Read(config ReadConfig) error { return read(config) }

func (service) Continue(config ContinueConfig) error { return continueWorkerSession(config) }
