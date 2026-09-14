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
		func() string { return "activation-one" },
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
		func() string { return "activation-two" },
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
		func() string { return "activation-three" },
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

func TestNewPublishesStableSecretSafeActivationTuple(t *testing.T) {
	t.Parallel()

	authored := &factorydefinitions.FactoryConfig{
		Name: "alpha",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker",
			Body: "private instructions",
		}},
	}
	first, err := loadedsource.New("factory", authored, emptyDefinitions{}, nil, func() string { return "activation-first" })
	if err != nil {
		t.Fatalf("New(first): %v", err)
	}
	second, err := loadedsource.New("factory", authored, emptyDefinitions{}, nil, func() string { return "activation-second" })
	if err != nil {
		t.Fatalf("New(second): %v", err)
	}
	firstTuple := first.FactoryActivationProvenance()
	secondTuple := second.FactoryActivationProvenance()
	if firstTuple == nil || secondTuple == nil {
		t.Fatalf("activation tuples = (%#v, %#v), want both present", firstTuple, secondTuple)
	}
	if firstTuple.ActivationID == "" || firstTuple.LoadedSourceDigest == "" {
		t.Fatalf("first activation tuple = %#v, want non-empty identity and digest", firstTuple)
	}
	if firstTuple.ActivationID == secondTuple.ActivationID {
		t.Fatal("separate source constructions reused activation identity")
	}
	if firstTuple.LoadedSourceDigest != secondTuple.LoadedSourceDigest {
		t.Fatalf("same effective source digest changed: first=%q second=%q", firstTuple.LoadedSourceDigest, secondTuple.LoadedSourceDigest)
	}
	if firstTuple.State != factorydefinitions.FactoryActivationStateNotActivated {
		t.Fatalf("initial activation state = %q, want NOT_ACTIVATED until the loader proves the source", firstTuple.State)
	}

	first.SetAuthoredSourceComparison(func() (factorydefinitions.FactoryActivationState, error) {
		return factorydefinitions.FactoryActivationStateActive, nil
	})
	state, err := first.CompareAuthoredSource()
	if err != nil || state != factorydefinitions.FactoryActivationStateActive {
		t.Fatalf("CompareAuthoredSource() = (%q, %v), want ACTIVE", state, err)
	}
	updatedTuple := first.FactoryActivationProvenance()
	if updatedTuple == nil || updatedTuple.ActivationID != firstTuple.ActivationID || updatedTuple.LoadedSourceDigest != firstTuple.LoadedSourceDigest {
		t.Fatalf("activation identity changed after comparison: before=%#v after=%#v", firstTuple, updatedTuple)
	}
}

func TestLoadedSourceDigestChangesWhenResolvedInstructionsChange(t *testing.T) {
	t.Parallel()

	base := &factorydefinitions.FactoryConfig{
		Name:    "alpha",
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker", Body: "first"}},
	}
	changed := &factorydefinitions.FactoryConfig{
		Name:    "alpha",
		Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker", Body: "second"}},
	}
	baseDigest, err := loadedsource.LoadedSourceDigest(base)
	if err != nil {
		t.Fatalf("LoadedSourceDigest(base): %v", err)
	}
	changedDigest, err := loadedsource.LoadedSourceDigest(changed)
	if err != nil {
		t.Fatalf("LoadedSourceDigest(changed): %v", err)
	}
	if baseDigest == changedDigest {
		t.Fatalf("instruction edit kept digest %q", baseDigest)
	}
}

type emptyDefinitions struct{}

func (emptyDefinitions) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func (emptyDefinitions) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
