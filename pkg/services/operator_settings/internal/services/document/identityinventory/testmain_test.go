package identityinventory_test

import (
	"testing"

	internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"

	_ "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/identityinputinventory"
)

func TestMain(m *testing.M) {
	internaltestlink.RegisterComposition()
	m.Run()
}
