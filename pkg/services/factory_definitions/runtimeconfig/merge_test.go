package runtimeconfig_test

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/runtimeconfig"
)

type definitions struct {
	workers      map[string]*factorydefinitions.FactoryWorkerConfig
	workstations map[string]*factorydefinitions.FactoryWorkstationConfig
}

func (d definitions) Worker(
	name string,
) (*factorydefinitions.FactoryWorkerConfig, bool) {
	worker, ok := d.workers[name]
	return worker, ok
}

func (d definitions) Workstation(
	name string,
) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	workstation, ok := d.workstations[name]
	return workstation, ok
}

func TestMergeBuildsDetachedEffectiveDefinition(t *testing.T) {
	t.Parallel()

	authored := &factorydefinitions.FactoryConfig{
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name:  "agent",
			Model: "authored-model",
		}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name:      "review",
			StopWords: []string{"authored"},
			Env:       map[string]string{"AUTHORED": "true"},
		}},
	}
	runtimeWorker := &factorydefinitions.FactoryWorkerConfig{
		Name:  "agent",
		Model: "runtime-model",
		Args:  []string{"--runtime"},
		Auth:  &factorydefinitions.HostedWorkerAuthConfig{},
	}
	runtimeWorkstation := &factorydefinitions.FactoryWorkstationConfig{
		Name:             "review",
		WorkerTypeName:   "agent",
		StopWords:        []string{"runtime"},
		RuntimeStopWords: []string{"terminal"},
		Env: map[string]string{
			"AUTHORED": "overridden",
			"RUNTIME":  "true",
		},
	}

	effective, err := runtimeconfig.Merge(
		authored,
		definitions{
			workers: map[string]*factorydefinitions.FactoryWorkerConfig{
				"agent": runtimeWorker,
			},
			workstations: map[string]*factorydefinitions.FactoryWorkstationConfig{
				"review": runtimeWorkstation,
			},
		},
	)
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	if got := effective.Workers[0].Model; got != "runtime-model" {
		t.Fatalf("worker model = %q, want runtime-model", got)
	}
	if got := effective.Workstations[0].Type; got != factorydefinitions.WorkstationTypeModel {
		t.Fatalf("workstation type = %q, want %q", got, factorydefinitions.WorkstationTypeModel)
	}
	if got := effective.Workstations[0].StopWords; len(got) != 3 ||
		got[0] != "authored" || got[1] != "runtime" || got[2] != "terminal" {
		t.Fatalf("stop words = %#v", got)
	}
	if got := effective.Workstations[0].Env["AUTHORED"]; got != "overridden" {
		t.Fatalf("AUTHORED env = %q, want overridden", got)
	}

	effective.Workers[0].Args[0] = "mutated"
	effective.Workstations[0].Env["RUNTIME"] = "mutated"
	if runtimeWorker.Args[0] != "--runtime" {
		t.Fatal("effective Worker aliases runtime definition Args")
	}
	if runtimeWorkstation.Env["RUNTIME"] != "true" {
		t.Fatal("effective Workstation aliases runtime definition Env")
	}
	if authored.Workers[0].Model != "authored-model" {
		t.Fatal("Merge mutated authored Factory Definition")
	}
}

func TestMergeRequiresRuntimeDefinitions(t *testing.T) {
	t.Parallel()

	_, err := runtimeconfig.Merge(&factorydefinitions.FactoryConfig{}, nil)
	if err == nil || err.Error() != "runtime config is required" {
		t.Fatalf("Merge error = %v", err)
	}
}

func TestMergeNilFactoryReturnsNoEffectiveDefinition(t *testing.T) {
	t.Parallel()

	effective, err := runtimeconfig.Merge(nil, definitions{})
	if err != nil {
		t.Fatalf("Merge nil Factory: %v", err)
	}
	if effective != nil {
		t.Fatalf("effective Factory = %#v, want nil", effective)
	}
}

func TestNormalizeCanonicalWorkstationRuntimeAppliesPublicDefaults(t *testing.T) {
	t.Parallel()

	workstation := &factorydefinitions.FactoryWorkstationConfig{
		Type:    factorydefinitions.WorkstationTypePoller,
		Body:    "poll the queue",
		Timeout: "2m",
	}

	runtimeconfig.NormalizeCanonicalWorkstationRuntime(workstation)

	if workstation.Kind != factorydefinitions.WorkstationKindPoller {
		t.Fatalf("kind = %q, want poller", workstation.Kind)
	}
	if workstation.PromptTemplate != "poll the queue" {
		t.Fatalf("prompt template = %q, want body fallback", workstation.PromptTemplate)
	}
	if workstation.Limits.MaxExecutionTime != "2m" || workstation.Timeout != "" {
		t.Fatalf(
			"execution limits = (%q, legacy %q), want canonical 2m only",
			workstation.Limits.MaxExecutionTime,
			workstation.Timeout,
		)
	}
}
