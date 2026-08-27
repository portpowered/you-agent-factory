package definitions

import (
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var sharedDefinitionsProcess func(testing.TB) support.ApplicationProcess

func buildDefinitionsProcess(t testing.TB) support.Process {
	t.Helper()
	if sharedDefinitionsProcess == nil {
		t.Fatal("shared Factory Definitions process has not been initialized")
	}
	return sharedDefinitionsProcess(t)
}
