package runtimebuild_test

import (
	"context"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	runtimebuild "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/instance_host/build"
	factoryhost "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/host"
	"go.uber.org/zap"
)

type interpolationLoadedFactory struct {
	config         *factorydefinitions.FactoryConfig
	runtimeBaseDir string
}

func (*interpolationLoadedFactory) FactoryDir() string { return "/fusion" }
func (source *interpolationLoadedFactory) FactoryConfig() *factorydefinitions.FactoryConfig {
	return source.config
}
func (source *interpolationLoadedFactory) Worker(name string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	for index := range source.config.Workers {
		if source.config.Workers[index].Name == name {
			return &source.config.Workers[index], true
		}
	}
	return nil, false
}
func (*interpolationLoadedFactory) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (source *interpolationLoadedFactory) RuntimeBaseDir() string { return source.runtimeBaseDir }
func (source *interpolationLoadedFactory) SetRuntimeBaseDir(dir string) {
	source.runtimeBaseDir = dir
}
func (*interpolationLoadedFactory) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (source *interpolationLoadedFactory) MutateWorkers(
	mutate func(*factorydefinitions.FactoryWorkerConfig) error,
) error {
	for index := range source.config.Workers {
		if err := mutate(&source.config.Workers[index]); err != nil {
			return err
		}
	}
	return nil
}

func TestBuiltInFusionFactory_RuntimeBuildAllowsInvocationInterpolatedModelProvider(t *testing.T) {
	loaded := &interpolationLoadedFactory{
		config: &factorydefinitions.FactoryConfig{
			Name: "fusion",
			Workers: []factorydefinitions.FactoryWorkerConfig{{
				Name:          "fusion-drafter",
				ModelProvider: "${firstProvider}",
			}},
		},
	}
	builder, err := runtimebuild.New(
		"CODEX",
		"gpt-5",
		true,
		"",
		"",
		nil,
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return loaded, nil
		},
		nil,
		nil,
		nil,
		nil,
		nil,
		platformclock.Real{},
		testRuntimeID,
		zap.NewNop(),
		func(context.Context, runtimebuild.SessionBuildSpec) (*factoryhost.Bundle, error) {
			return &factoryhost.Bundle{}, nil
		},
		nil,
	)
	if err != nil {
		t.Fatalf("runtimebuild.New: %v", err)
	}
	spec, err := builder.BuildSpec(
		context.Background(),
		"/fusion",
		"/fusion",
		"~default",
		"/fusion",
		nil,
		"",
		nil,
		nil,
		nil,
		nil,
		false,
	)
	if err != nil {
		t.Fatalf("BuildSpec: %v", err)
	}
	worker, ok := spec.LoadedFactoryCfg.Worker("fusion-drafter")
	if !ok {
		t.Fatal("expected fusion-drafter worker")
	}
	if worker.ModelProvider != "${firstProvider}" {
		t.Fatalf("modelProvider = %q, want exact invocation placeholder preserved", worker.ModelProvider)
	}
}
