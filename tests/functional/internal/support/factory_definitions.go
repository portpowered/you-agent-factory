package support

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// FlattenFactoryConfig observes Factory Definitions through the same public
// command used by customers.
func FlattenFactoryConfig(t testing.TB, path string) ([]byte, error) {
	t.Helper()
	inputs := FakeInputs(t.Context(), []string{"you", "factory", "config", "flatten", path})
	inputs.Input.Env = os.Environ()
	inputs.Input.WorkingDirectory = filepath.Dir(path)
	if err := BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		return nil, err
	}
	return []byte(inputs.Stdout()), nil
}

// LoadedFactory observes the generated public Factory value emitted by the
// same flatten command available to customers. Functional support deliberately
// does not reconstruct an internal Factory Definitions service value.
func LoadedFactory(t testing.TB, path string) (factoryapi.Factory, error) {
	t.Helper()
	payload, err := FlattenFactoryConfig(t, path)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return DecodeFactoryDefinition(payload)
}

// LoadedCurrentFactory observes the concrete current Factory directory
// returned by the customer operation that selected it.
func LoadedCurrentFactory(t testing.TB, currentFactoryDir string) (factoryapi.Factory, error) {
	t.Helper()
	return LoadedFactory(t, currentFactoryDir)
}

// DecodeFactoryDefinition decodes the public JSON representation without
// invoking transport mapping policy from functional code.
func DecodeFactoryDefinition(payload []byte) (factoryapi.Factory, error) {
	var factory factoryapi.Factory
	if err := json.Unmarshal(payload, &factory); err != nil {
		return factoryapi.Factory{}, err
	}
	return factory, nil
}

func FindFactoryWorker(factory factoryapi.Factory, name string) (factoryapi.Worker, bool) {
	if factory.Workers == nil {
		return factoryapi.Worker{}, false
	}
	for _, worker := range *factory.Workers {
		if worker.Name == name {
			return worker, true
		}
	}
	return factoryapi.Worker{}, false
}

func FindFactoryWorkstation(factory factoryapi.Factory, name string) (factoryapi.Workstation, bool) {
	if factory.Workstations == nil {
		return factoryapi.Workstation{}, false
	}
	for _, workstation := range *factory.Workstations {
		if workstation.Name == name {
			return workstation, true
		}
	}
	return factoryapi.Workstation{}, false
}
