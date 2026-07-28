package servicewire

import (
	"testing"

	internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"
)

// TestMain registers operator-settings composition hooks before servicewire tests run.
func TestMain(m *testing.M) {
	internaltestlink.RegisterComposition()
	m.Run()
}
