package definitions

import (
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func buildDefinitionsProcess(t testing.TB) support.Process {
	t.Helper()
	return support.BuildProcess(t, serviceedges.Edges{})
}
