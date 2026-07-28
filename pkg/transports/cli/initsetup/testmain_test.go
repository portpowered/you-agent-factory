package initsetup_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/operator_settings/testlink"
)

func TestMain(m *testing.M) {
	testlink.RegisterProvidersRoot()
	m.Run()
}
