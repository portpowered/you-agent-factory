package service

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestNewDurableExecutionCanonicalizesOperatorDefaultsAndPresets(t *testing.T) {
	var got factoryruntime.JavaScriptWorkerSettings
	logger := zap.NewNop()
	var selectedLogger *zap.Logger
	executionFactory := func(
		_ string,
		_ factorysessions.PersistencePolicy,
		_ providers.Service,
		_ factoryruntime.Clock,
		_ map[string]struct{},
		settings factoryruntime.JavaScriptWorkerSettings,
		_ *workers.MockWorkersConfig,
		_ []operatorconfig.ACPIntegration,
		logger *zap.Logger,
	) (durableexecution.Service, error) {
		got = settings
		selectedLogger = logger
		return nil, nil
	}
	opened, err := NewDurableOpening(
		func(string) (operatorconfig.Config, error) {
			return operatorconfig.Config{
				Defaults: operatorconfig.Defaults{WorkerModelProvider: "customer"},
				WorkerPresets: []operatorconfig.WorkerPreset{{
					ID: "review", ModelProvider: "agent",
				}},
			}, nil
		},
		executionFactory,
		factorysessions.ProviderIdentityResolver(func(identity string) (string, error) {
			switch identity {
			case "CODEX":
				return "codex", nil
			case "customer":
				return "customer.provider", nil
			case "agent":
				return "cursor", nil
			default:
				return "", errors.New("unexpected provider")
			}
		}),
	).Open(
		factorydefinitions.RuntimeSelection{Directory: t.TempDir()},
		factorysessions.PersistencePolicyDisabled,
		t.TempDir(),
		"",
		operatorconfig.ResolvedDefaults{WorkerModelProvider: "CODEX", WorkerModel: "operator-model"},
		RuntimeRoot{FactoryRootDir: t.TempDir(), BaseLogger: logger},
		nil,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("NewDurableExecution: %v", err)
	}
	if selectedLogger != logger {
		t.Fatal("durable constructor did not receive the selected opening logger")
	}
	if got.DefaultModelProvider != "codex" || got.DefaultModel != "operator-model" {
		t.Fatalf("resolved defaults = %#v, want codex/operator-model", got)
	}
	if preset := got.Presets["review"]; preset.ModelProvider != "cursor" {
		t.Fatalf("review preset = %#v, want canonical cursor identity", preset)
	}
	if opened.WorkerSettings == nil || opened.WorkerSettings.Presets["review"].ModelProvider != "cursor" {
		t.Fatalf("opened WorkerSettings = %#v, want canonical review preset for the process start", opened.WorkerSettings)
	}
	got.Presets["review"] = factoryruntime.JavaScriptWorkerPreset{Model: "changed"}
	if opened.WorkerSettings.Presets["review"].Model != "" {
		t.Fatal("opened WorkerSettings aliases the runtime execution settings")
	}
}

func TestNewDurableExecutionPreservesPartialOwnerOnAcquisitionFailure(t *testing.T) {
	t.Parallel()
	owner := &portableReplayRuntimeOwner{}
	failure := errors.New("resource acquisition failed")
	opened, err := NewDurableOpening(
		func(string) (operatorconfig.Config, error) { return operatorconfig.Config{}, nil },
		func(string, factorysessions.PersistencePolicy, providers.Service, factoryruntime.Clock,
			map[string]struct{}, factoryruntime.JavaScriptWorkerSettings, *workers.MockWorkersConfig,
			[]operatorconfig.ACPIntegration, *zap.Logger) (durableexecution.Service, error) {
			return owner, failure
		},
		func(identity string) (string, error) { return identity, nil },
	).Open(
		factorydefinitions.RuntimeSelection{Directory: "/selected"},
		factorysessions.PersistencePolicyDisabled,
		"/operator",
		"",
		operatorconfig.ResolvedDefaults{},
		RuntimeRoot{FactoryRootDir: "/selected", BaseLogger: zap.NewNop()},
		nil,
		nil,
		nil,
	)
	if !errors.Is(err, failure) {
		t.Fatalf("opening error = %v, want acquisition failure", err)
	}
	if opened.Service != owner {
		t.Fatal("failed opening lost the acquired owner needed for cleanup")
	}
	if opened.WorkerSettings != nil || opened.ACPIntegrations != nil || opened.OperatorModels != nil {
		t.Fatal("failed opening published successful execution settings")
	}
}

func TestDurableOpeningKeepsRequestSettingsIndependent(t *testing.T) {
	t.Parallel()
	configured := operatorconfig.Config{WorkerPresets: []operatorconfig.WorkerPreset{{ID: "review", ModelProvider: "codex", Model: "first"}}}
	var roots []string
	opening := NewDurableOpening(
		func(string) (operatorconfig.Config, error) { return configured, nil },
		func(root string, _ factorysessions.PersistencePolicy, _ providers.Service, _ factoryruntime.Clock,
			_ map[string]struct{}, _ factoryruntime.JavaScriptWorkerSettings, _ *workers.MockWorkersConfig,
			_ []operatorconfig.ACPIntegration, _ *zap.Logger) (durableexecution.Service, error) {
			roots = append(roots, root)
			return nil, nil
		},
		func(identity string) (string, error) { return identity, nil },
	)
	first, err := opening.Open(factorydefinitions.RuntimeSelection{Directory: "/first"},
		factorysessions.PersistencePolicyDisabled, "/operator", "", operatorconfig.ResolvedDefaults{WorkerModel: "first-default"},
		RuntimeRoot{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	configured.WorkerPresets[0].Model = "second"
	second, err := opening.Open(factorydefinitions.RuntimeSelection{Directory: "/second"},
		factorysessions.PersistencePolicyDisabled, "/operator", "", operatorconfig.ResolvedDefaults{WorkerModel: "second-default"},
		RuntimeRoot{}, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 2 || roots[0] != "/first" || roots[1] != "/second" {
		t.Fatalf("acquired roots = %v", roots)
	}
	if first.WorkerSettings.Presets["review"].Model != "first" || first.WorkerSettings.DefaultModel != "first-default" {
		t.Fatalf("first opening settings changed: %#v", first.WorkerSettings)
	}
	if second.WorkerSettings.Presets["review"].Model != "second" || second.WorkerSettings.DefaultModel != "second-default" {
		t.Fatalf("second opening settings = %#v", second.WorkerSettings)
	}
	second.WorkerSettings.Presets["review"] = factoryruntime.JavaScriptWorkerPreset{Model: "mutated"}
	if first.WorkerSettings.Presets["review"].Model != "first" {
		t.Fatal("second result mutation changed the first opening")
	}
}
