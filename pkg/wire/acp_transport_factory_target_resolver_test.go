package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func namedFactoryCatalogForTest(t *testing.T) factorydefinitions.NamedFactoryCatalog {
	t.Helper()
	edges := serviceedges.Edges{}
	namedPathFileSystem := provideFactoryDefinitionNamedPathFileSystem(edges)
	namedPaths, err := provideFactoryDefinitionNamedPathResolver(namedPathFileSystem)
	if err != nil {
		t.Fatalf("provideFactoryDefinitionNamedPathResolver() error = %v", err)
	}
	catalogFileSystem := provideFactoryDefinitionNamedFactoryCatalogFileSystem(edges)
	catalog, err := provideNamedFactoryCatalog(namedPaths, catalogFileSystem)
	if err != nil {
		t.Fatalf("provideNamedFactoryCatalog() error = %v", err)
	}
	return catalog
}

func operatorDefaultsResolverForTest(t *testing.T) operatorsettings.DefaultsResolver {
	t.Helper()
	edges := serviceedges.Edges{}
	files := provideOperatorSettingsFileSystem(edges)
	providersRoot, err := provideProvidersService(edges)
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	providerRegistry, err := provideProviderRegistry(edges, providersRoot)
	if err != nil {
		t.Fatalf("provideProviderRegistry() error = %v", err)
	}
	service, err := provideOperatorSettingsService(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providerRegistry),
		provideOperatorConfigDecoder(),
		provideOperatorConfigEncoder(),
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("provideOperatorSettingsService() error = %v", err)
	}
	return provideOperatorDefaultsResolver(service)
}

// TestProvideACPServerFactoryTargetRuntimeResolver proves the closure
// provideACPServerFactoryTargetRuntimeResolver returns resolves a valid
// "factory:<name>" target and the Chat Session's working root into a
// Runtime Opening request carrying the resolved Factory's directory, fails
// safely for an already-canceled context and for a home-directory lookup
// failure, and reports factorydefinitions.ErrNamedFactoryNotFound for a
// target no installed named Factory resolves -- all without opening any
// runtime (construction and resolution alone perform no runtime I/O).
func TestProvideACPServerFactoryTargetRuntimeResolver(t *testing.T) {
	home := t.TempDir()
	seedInstalledPackagedFactories(t, home, "@you/review")

	resolveHomeDir := func() (string, error) { return home, nil }
	resolver := provideACPServerFactoryTargetRuntimeResolver(
		resolveHomeDir,
		namedFactoryCatalogForTest(t),
		operatorDefaultsResolverForTest(t),
		provideRuntimeArtifactRootResolver(),
	)

	t.Run("canceled context fails without resolving", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := resolver(ctx, "factory:@you/review", "/workspace/project"); err == nil {
			t.Fatal("resolver(canceled ctx) error = nil, want a cancellation error")
		}
	})

	t.Run("home directory lookup failure propagates", func(t *testing.T) {
		wantErr := errors.New("home dir unavailable")
		failingResolver := provideACPServerFactoryTargetRuntimeResolver(
			func() (string, error) { return "", wantErr },
			namedFactoryCatalogForTest(t),
			operatorDefaultsResolverForTest(t),
			provideRuntimeArtifactRootResolver(),
		)
		if _, err := failingResolver(context.Background(), "factory:@you/review", "/workspace/project"); !errors.Is(err, wantErr) {
			t.Fatalf("resolver(homeDir error) error = %v, want %v", err, wantErr)
		}
	})

	t.Run("unknown target reports ErrNamedFactoryNotFound", func(t *testing.T) {
		if _, err := resolver(context.Background(), "factory:@you/does-not-exist", "/workspace/project"); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
			t.Fatalf("resolver(unknown target) error = %v, want %v", err, factorydefinitions.ErrNamedFactoryNotFound)
		}
	})

	t.Run("known target resolves a Runtime Opening request", func(t *testing.T) {
		req, err := resolver(context.Background(), "factory:@you/review", "/workspace/project")
		if err != nil {
			t.Fatalf("resolver() error = %v, want a resolved Runtime Opening request", err)
		}
		if req.FactoryDefinition.Directory == "" {
			t.Fatal("resolved FactoryDefinition.Directory is blank, want the installed @you/review Factory directory")
		}
		if req.FactorySession.SystemConfigHome != home {
			t.Fatalf("resolved FactorySession.SystemConfigHome = %q, want %q", req.FactorySession.SystemConfigHome, home)
		}
	})
}
