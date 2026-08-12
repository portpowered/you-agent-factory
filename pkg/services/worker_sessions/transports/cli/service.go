// Package cli owns the Worker Sessions service CLI adapter.
package cli

// IDGenerator supplies caller-owned identities for CLI requests. Production
// composition selects the implementation in pkg/wire so the transport does
// not reach directly into an identity provider.
type IDGenerator func() string

// ExecutionFileReader supplies execution-document bytes for the invoke
// command. Production composition selects the filesystem implementation in
// pkg/wire; tests can replace it without touching the host filesystem.
type ExecutionFileReader func(string) ([]byte, error)

// Effects contains the external effects used while normalizing direct CLI
// requests. Continue uses only GenerateID; invoke uses both fields.
type Effects struct {
	GenerateID IDGenerator
	ReadFile   ExecutionFileReader
}

func selectEffects(effects []Effects) Effects {
	if len(effects) == 0 {
		return Effects{}
	}
	return effects[0]
}

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
