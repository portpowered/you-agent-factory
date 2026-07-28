package settingsresolution_test

import (
	"testing"

	internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"

	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/identityinputinventory"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution/defaults"
)

func TestMain(m *testing.M) {
	internaltestlink.RegisterComposition()
	m.Run()
}
