package definitions

import (
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func buildDefinitionsProcess(t testing.TB) support.Process {
	t.Helper()
	return sharedDefinitionsProcess(t)
}
