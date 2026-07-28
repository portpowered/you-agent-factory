package settingsdocument_test

import (
	"testing"

	internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"
)

func TestMain(m *testing.M) {
	internaltestlink.RegisterComposition()
	m.Run()
}
