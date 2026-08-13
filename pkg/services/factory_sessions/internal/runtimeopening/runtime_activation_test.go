package runtimeopening

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestActivationRequestCarriesExplicitRuntimeInputs(t *testing.T) {
	t.Parallel()

	skipPermissions := true
	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{
			Directory:        "/factory",
			SourcePath:       "/source",
			ExecutionBaseDir: "/runtime",
		},
		FactoryRuntime: factoryruntime.RuntimeOpeningRequest{Verbose: true},
		FactorySession: factorysessions.SessionRuntimeOpeningRequest{
			BackendScopeID: "scope",
			Host: factorysessions.RuntimeHostRequest{
				Host: "127.0.0.1",
				Port: 8080,
			},
		},
		Workers: workers.RuntimeOpeningRequest{
			RunnerID:                          "runner",
			InvocationSkipPermissionsOverride: &skipPermissions,
		},
	}
	factory := &Factory{generateRuntimeInstanceID: func() string { return "runtime-1" }}
	activation, err := factory.activationRequest(request)
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	if activation.RuntimeID != "runtime-1" || activation.FactorySessionID != factorysessions.DefaultSessionID {
		t.Fatalf("activation identity = %#v, want runtime-1/%q", activation, factorysessions.DefaultSessionID)
	}
	if activation.Inputs.Definition.SourcePath != "/source" || activation.Inputs.Session.BackendScopeID != "scope" {
		t.Fatalf("activation inputs lost source or session values: %#v", activation.Inputs)
	}
	if activation.Inputs.Workers.InvocationSkipPermissionsOverride == nil || !*activation.Inputs.Workers.InvocationSkipPermissionsOverride {
		t.Fatal("activation inputs lost worker permission override")
	}
}

func TestActivationRequestDetachesMockWorkerInputs(t *testing.T) {
	t.Parallel()

	request := &factorysessions.RuntimeOpeningRequest{
		FactoryDefinition: factorydefinitions.RuntimeOpeningRequest{Directory: "/factory"},
		Workers: workers.RuntimeOpeningRequest{
			MockWorkers: &workers.MockWorkersConfig{
				MockWorkers: []workers.MockWorkerConfig{{
					RunType: workers.MockWorkerRunTypeScript,
					ScriptConfig: &workers.MockWorkerScriptConfig{
						Command: "run",
						Env:     map[string]string{"TOKEN": "one"},
					},
				}},
			},
		},
	}
	factory := &Factory{generateRuntimeInstanceID: func() string { return "runtime-1" }}
	activation, err := factory.activationRequest(request)
	if err != nil {
		t.Fatalf("activationRequest() error = %v", err)
	}
	request.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Env["TOKEN"] = "caller-mutated"
	request.Workers.MockWorkers.MockWorkers[0].ScriptConfig.Args = []string{"caller-mutated"}
	got := activation.Inputs.Workers.MockWorkers.MockWorkers[0]
	if got.ScriptConfig.Env["TOKEN"] != "one" || len(got.ScriptConfig.Args) != 0 {
		t.Fatalf("activation inputs retained caller mutation: %#v", got)
	}
}
