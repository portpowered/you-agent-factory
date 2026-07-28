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

type emptyDefinitions struct{}

func (emptyDefinitions) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (emptyDefinitions) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
