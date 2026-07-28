package servicewire

import (
	"testing"

	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

// TestMain registers operator-settings composition hooks before wire composition tests run.
func TestMain(m *testing.M) {
	settingswire.RegisterTestComposition()
	m.Run()
}
