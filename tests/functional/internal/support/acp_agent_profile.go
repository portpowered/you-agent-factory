package support

import (
	"context"
	"os"
	"sync"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/services/operator_settings/transports/globalconfig"
	settingswire "github.com/portpowered/infinite-you/pkg/services/operator_settings/wire"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
)

// SeedACPAgentProfile persists a real ACP Agent profile at the production
// Operator Settings config path for home, through the same
// operatorsettings.Service.UpdateACPAgentProfile production callers use, so
// a root.BuildProcess-composed process's real Operator Settings root
// resolves it unmodified. This is inert fixture setup, not a second
// service-construction path exercised by the scenario itself: the
// Providers/Operator Settings services built here only ever write the
// config document ahead of time and are discarded before the scenario's own
// root.BuildProcess call.
func SeedACPAgentProfile(t testing.TB, home, defaultTarget string, allowedTargets []string) {
	t.Helper()

	providersRoot, err := providerswire.NewService()
	if err != nil {
		t.Fatalf("providerswire.NewService() error = %v", err)
	}
	service, err := settingswire.NewServiceFromConfigDocument(
		settingswire.NewConfigDocumentService(
			platformfilesystem.Local{},
			func(dir, pattern string) (operatorsettings.TemporaryFile, error) {
				return os.CreateTemp(dir, pattern)
			},
			globalconfigmapping.Decode,
			globalconfigmapping.Encode,
			nil,
			&sync.Mutex{},
		),
		providersRoot,
		func() string { return "00000000-0000-4000-8000-000000000009" },
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("NewServiceFromConfigDocument() error = %v", err)
	}

	configPath := operatorsettings.DefaultConfigPath(home)
	if _, err := service.UpdateACPAgentProfile(context.Background(), configPath, operatorsettings.ACPAgentProfile{
		DefaultTarget:  defaultTarget,
		AllowedTargets: allowedTargets,
	}); err != nil {
		t.Fatalf("UpdateACPAgentProfile() error = %v", err)
	}
}
