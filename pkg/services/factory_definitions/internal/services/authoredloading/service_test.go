package authoredloading

import (
	"context"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestLoadValidatedAuthoredFactoryDefinitionReturnsDetachedEffectiveFacts(t *testing.T) {
	t.Parallel()

	loaded := &loadedSourceStub{
		factoryDir:     "fixtures/alpha",
		runtimeBaseDir: "fixtures/alpha",
		config: &factorydefinitions.FactoryConfig{
			Name:    "alpha",
			Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "planner", Body: "original"}},
		},
		bundled: []factorydefinitions.PortableBundledFileReplacement{{TargetPath: "docs/guide.md"}},
	}
	var currentCalls, selectedCalls int
	validator := &blockingValidatorStub{}
	service := New(
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			currentCalls++
			return loaded, nil
		},
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			selectedCalls++
			return loaded, nil
		},
		validator,
	)

	result, err := service.LoadValidatedAuthoredFactoryDefinition(
		t.Context(),
		factorydefinitions.LoadValidatedAuthoredFactoryDefinitionRequest{
			SourcePath:       "fixtures/alpha/factory.yaml",
			ExecutionBaseDir: "execution",
		},
	)
	if err != nil {
		t.Fatalf("LoadValidatedAuthoredFactoryDefinition: %v", err)
	}
	if currentCalls != 0 || selectedCalls != 1 || validator.calls != 1 {
		t.Fatalf(
			"loading calls current=%d selected=%d validation=%d, want 0,1,1",
			currentCalls,
			selectedCalls,
			validator.calls,
		)
	}
	if result.Source.Path != "fixtures/alpha/factory.yaml" ||
		result.Source.Format != factorydefinitions.AuthoredFactoryFormatYAML ||
		result.FactoryDir != "fixtures/alpha" || result.RuntimeBaseDir != "execution" {
		t.Fatalf("result identity facts = %#v", result)
	}
	if result.Definition == nil || result.Definition.Workers[0].Body != "original" ||
		len(result.BundledFileReplacements) != 1 {
		t.Fatalf("result effective facts = %#v", result)
	}

	result.Definition.Workers[0].Body = "caller mutation"
	result.BundledFileReplacements[0].TargetPath = "caller.md"
	if loaded.config.Workers[0].Body != "original" || loaded.bundled[0].TargetPath != "docs/guide.md" {
		t.Fatal("returned facts mutated the loader-owned effective source")
	}
}

type loadedSourceStub struct {
	factorydefinitions.MutableLoadedFactorySource
	factoryDir     string
	runtimeBaseDir string
	config         *factorydefinitions.FactoryConfig
	bundled        []factorydefinitions.PortableBundledFileReplacement
}

func (s *loadedSourceStub) FactoryDir() string { return s.factoryDir }

func (s *loadedSourceStub) RuntimeBaseDir() string { return s.runtimeBaseDir }

func (s *loadedSourceStub) FactoryConfig() *factorydefinitions.FactoryConfig { return s.config }

func (s *loadedSourceStub) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return s.bundled
}

type blockingValidatorStub struct {
	factorydefinitions.Validator
	calls int
}

func (s *blockingValidatorStub) ValidateBlockingLoad(
	context.Context,
	*factorydefinitions.FactoryConfig,
) factorydefinitions.ValidationResult {
	s.calls++
	return factorydefinitions.ValidationResult{}
}
