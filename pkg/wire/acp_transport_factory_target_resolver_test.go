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
	providersRoot, err := provideProvidersService(selectedTestTimeEdges(edges))
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	service, err := newOperatorSettingsTestService(
		files,
		provideOperatorSettingsCreateTemporaryFile(edges),
		provideOperatorSettingsProviderCatalog(providersRoot),
		provideOperatorConfigDecoder(),
		provideOperatorConfigDiagnosticsDecoder(),
		provideOperatorConfigEncoder(),
		provideOperatorSettingsIDGenerator(edges),
		providersRoot,
		logging.NoopLogger{},
	)
	if err != nil {
		t.Fatalf("newOperatorSettingsTestService() error = %v", err)
	}
	return provideOperatorDefaultsResolver(service)
}

func TestProvideACPServerFactorySessionStartResolver(t *testing.T) {
	home := t.TempDir()
	seedInstalledPackagedFactories(t, home, "@you/review")

	resolver := provideACPServerFactorySessionStartResolver(
		func() (string, error) { return home, nil },
		namedFactoryCatalogForTest(t),
		operatorDefaultsResolverForTest(t),
		provideRuntimeArtifactRootResolver(),
	)

	req, err := resolver(context.Background(), "factory:@you/review", "/workspace/project", "req-1")
	if err != nil {
		t.Fatalf("resolver() error = %v", err)
	}
	if !req.ActivationOnly {
		t.Fatal("ActivationOnly = false, want true")
	}
	if req.Correlation.RequestID != "req-1" {
		t.Fatalf("Correlation.RequestID = %q, want req-1", req.Correlation.RequestID)
	}
	if req.Definition.FactoryID != "factory:@you/review" {
		t.Fatalf("Definition.FactoryID = %q", req.Definition.FactoryID)
	}
	if req.FolderPath == "" {
		t.Fatal("FolderPath is blank, want the installed @you/review Factory directory")
	}
	if req.RuntimeSelection == nil || req.RuntimeSelection.SystemConfigHome != home {
		t.Fatalf("RuntimeSelection = %+v, want SystemConfigHome %q", req.RuntimeSelection, home)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := resolver(ctx, "factory:@you/review", "/workspace/project", "req-2"); err == nil {
		t.Fatal("resolver(canceled ctx) error = nil, want a cancellation error")
	}
	if _, err := resolver(context.Background(), "factory:@you/does-not-exist", "/workspace/project", "req-3"); !errors.Is(err, factorydefinitions.ErrNamedFactoryNotFound) {
		t.Fatalf("resolver(unknown target) error = %v, want ErrNamedFactoryNotFound", err)
	}
	wantHomeErr := errors.New("home unavailable")
	failingHome := provideACPServerFactorySessionStartResolver(
		func() (string, error) { return "", wantHomeErr },
		namedFactoryCatalogForTest(t),
		operatorDefaultsResolverForTest(t),
		provideRuntimeArtifactRootResolver(),
	)
	if _, err := failingHome(context.Background(), "factory:@you/review", "/workspace/project", "req-4"); !errors.Is(err, wantHomeErr) {
		t.Fatalf("resolver(home error) = %v, want %v", err, wantHomeErr)
	}
}

// proves the ACP Start resolver supplies the operator-default environment layer when
// resolving a Factory target runtime.
//
// The CLI has always supplied this layer, so YOU_DEFAULT_WORKER_MODEL_PROVIDER
// selects the Worker provider for `you run`. The ACP resolver passed an empty
// operatorsettings.Defaults, silently dropping both variables. That is
// invisible for a Factory whose workers name their own provider and fatal for
// one whose workers do not: a JavaScript Factory's agent.run children carry no
// provider of their own, so with no operator default their dispatch is
// rejected before any provider runs. An ACP client cannot pass `--provider`,
// so the process environment is the only layer it has.
func TestProvideACPServerFactorySessionStartResolverAppliesOperatorDefaultsEnvironment(t *testing.T) {
	home := t.TempDir()
	seedInstalledPackagedFactories(t, home, "@you/review")

	resolver := provideACPServerFactorySessionStartResolver(
		func() (string, error) { return home, nil },
		namedFactoryCatalogForTest(t),
		operatorDefaultsResolverForTest(t),
		provideRuntimeArtifactRootResolver(),
	)

	t.Run("environment selects the default worker provider and model", func(t *testing.T) {
		t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "codex")
		t.Setenv(operatorsettings.EnvDefaultWorkerModel, "gpt-5")

		req, err := resolver(context.Background(), "factory:@you/review", "/workspace/project", "req-env-1")
		if err != nil {
			t.Fatalf("resolver() error = %v", err)
		}
		if req.RuntimeSelection == nil || req.RuntimeSelection.OperatorDefaults.WorkerModelProvider == "" {
			t.Fatal("resolved OperatorDefaults.WorkerModelProvider is blank, want the value the environment supplied")
		}
		if req.RuntimeSelection.OperatorDefaults.WorkerModel != "gpt-5" {
			t.Fatalf("resolved OperatorDefaults.WorkerModel = %q, want %q",
				req.RuntimeSelection.OperatorDefaults.WorkerModel, "gpt-5")
		}
	})

	t.Run("blank environment supplies no layer of its own", func(t *testing.T) {
		t.Setenv(operatorsettings.EnvDefaultWorkerModelProvider, "")
		t.Setenv(operatorsettings.EnvDefaultWorkerModel, "")

		req, err := resolver(context.Background(), "factory:@you/review", "/workspace/project", "req-env-2")
		if err != nil {
			t.Fatalf("resolver() error = %v", err)
		}
		// An unset variable must not override the persisted Operator Settings
		// document with a blank, so this asserts the environment contributes
		// nothing rather than asserting a particular resolved value.
		if req.RuntimeSelection == nil || req.RuntimeSelection.OperatorDefaults.WorkerModel == "gpt-5" {
			t.Fatal("resolved OperatorDefaults.WorkerModel = \"gpt-5\" with no environment set, want the environment layer to contribute nothing")
		}
	})
}
