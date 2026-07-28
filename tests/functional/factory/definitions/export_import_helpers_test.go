package definitions

import (
	"os"
	"path/filepath"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func flattenFactoryConfigWithEdges(
	t *testing.T,
	edges serviceedges.Edges,
	factoryConfigPath string,
) ([]byte, error) {
	t.Helper()

	inputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "flatten", factoryConfigPath,
	})
	inputs.Input.Env = os.Environ()
	inputs.Input.WorkingDirectory = filepath.Dir(factoryConfigPath)
	if err := support.BuildProcess(t, edges).Execute(inputs.Input); err != nil {
		return nil, err
	}
	return []byte(inputs.Stdout()), nil
}

func decodeFactoryDefinitionForTest(payload []byte) (factoryapi.Factory, error) {
	return support.DecodeFactoryDefinition(payload)
}
