package systeminitialization_test

import (
	"testing"

	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
)

func TestMain(m *testing.M) {
	settingswire.RegisterTestComposition()
	m.Run()
}
