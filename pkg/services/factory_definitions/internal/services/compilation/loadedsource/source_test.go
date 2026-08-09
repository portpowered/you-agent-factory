package loadedsource_test

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loadedsource"
)

func TestNewBuildsDetachedEffectiveLookups(t *testing.T) {
	t.Parallel()

	authored := &factorydefinitions.FactoryConfig{
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}},
		Workstations: []factorydefinitions.FactoryWorkstationConfig{{
			Name: "workstation",
		}},
	}
	source, err := loadedsource.New(
		"factory",
		authored,
		emptyDefinitions{},
		[]factorydefinitions.PortableBundledFileReplacement{{TargetPath: "AGENTS.md"}},
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	worker, ok := source.Worker("worker")
	if !ok {
		t.Fatal("Worker(worker) missing")
	}
	worker.Body = "lookup mutation"
	if source.FactoryConfig().Workers[0].Body == worker.Body {
		t.Fatal("lookup worker aliases effective Factory worker")
	}
	if got := source.PortableBundledFileReplacements(); len(got) != 1 || got[0].TargetPath != "AGENTS.md" {
		t.Fatalf("PortableBundledFileReplacements = %#v", got)
	}
}

func TestMutateWorkersPreservesFactoryAndLookupErrorContext(t *testing.T) {
	t.Parallel()

	source, err := loadedsource.New(
		"factory",
		&factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}},
		},
		emptyDefinitions{},
		nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	calls := 0
	err = source.MutateWorkers(func(worker *factorydefinitions.FactoryWorkerConfig) error {
		calls++
		if calls == 2 {
			return errors.New("lookup failed")
		}
		worker.Body = "mutated"
		return nil
	})
	if err == nil || err.Error() != `worker "worker": lookup failed` {
		t.Fatalf("MutateWorkers error = %v", err)
	}
}

func TestNewKeepsPromptSourceIdentityOutsideFactoryConfiguration(t *testing.T) {
	t.Parallel()

	workerPath := "factory/workers/worker/AGENTS.md"
	workstationPath := "factory/workstations/review/prompt.md"
	source, err := loadedsource.New(
		"factory",
		&factorydefinitions.FactoryConfig{
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				Name:             "worker",
				PromptSourcePath: workerPath,
			}},
			Workstations: []factorydefinitions.FactoryWorkstationConfig{{
				Name:                   "review",
				PromptSourcePath:       workstationPath,
				PromptSourceIsTemplate: true,
			}},
		},
		emptyDefinitions{},
		nil,
	)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	workerSource, ok := source.WorkerPromptSource("worker")
	if !ok || workerSource.Path != workerPath || workerSource.IsTemplate {
		t.Fatalf("worker prompt source = (%#v, %t)", workerSource, ok)
	}
	workstationSource, ok := source.WorkstationPromptSource("review")
	if !ok || workstationSource.Path != workstationPath || !workstationSource.IsTemplate {
		t.Fatalf("workstation prompt source = (%#v, %t)", workstationSource, ok)
	}
	if source.FactoryConfig().Workers[0].PromptSourcePath != "" ||
		source.FactoryConfig().Workstations[0].PromptSourcePath != "" {
		t.Fatal("prompt source identity leaked into Factory configuration")
	}
}

type emptyDefinitions struct{}

func (emptyDefinitions) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (emptyDefinitions) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
