package servicewire

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink"
)

// TestMain registers operator-settings composition hooks before servicewire tests run.
func TestMain(m *testing.M) {
	testlink.RegisterComposition()
	m.Run()
}
