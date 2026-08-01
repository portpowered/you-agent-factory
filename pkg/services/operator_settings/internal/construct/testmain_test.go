package construct_test

import (
	"os"
	"testing"

	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
	internaltestproviders "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testproviders"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestMain(m *testing.M) {
	providersRoot := internaltestproviders.StandardCatalog()
	settingsconstruct.SetConstructProvidersRootForTests(func() (providers.Service, error) {
		return providersRoot, nil
	})
	os.Exit(m.Run())
}
