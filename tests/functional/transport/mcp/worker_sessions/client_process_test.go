package workersessions_test

import (
	"testing"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type selectedHostClientProcess = support.SelectedHostClientProcess

func newSelectedHostClientProcess(t *testing.T) selectedHostClientProcess {
	t.Helper()
	return support.NewSelectedHostClientProcess(t)
}
