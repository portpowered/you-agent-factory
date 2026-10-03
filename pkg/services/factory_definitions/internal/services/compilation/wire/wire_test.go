package wire_test

import (
	"context"
	"encoding/json"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

type stubLoadedSource struct {
	cfg *factorydefinitions.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorydefinitions.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                               { return "/factories/alpha" }
func (s stubLoadedSource) RuntimeBaseDir() string                           { return "/factories/alpha" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                         {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorydefinitions.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*factorydefinitions.FactoryWorkerConfig) error) error {
	return nil
}
func (s stubLoadedSource) Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*factorydefinitions.FactoryWorkerConfig, bool) {
	return nil, false
}

func stubLoadCanonical(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
	var cfg factorydefinitions.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factoryroot.ErrInvalidNamedFactory
	}
	return stubLoadedSource{cfg: &cfg}, nil
}

func stubEncodeFactory(cfg *factorydefinitions.FactoryConfig) ([]byte, error) {
	return json.Marshal(cfg)
}

func TestNewService_HostEffectsComeOnlyFromInjectedPorts(t *testing.T) {
	t.Parallel()

	loadCalls := 0
	loadCanonical := factorydefinitions.CanonicalFactoryJSONLoader(func(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		loadCalls++
		return stubLoadCanonical(payload, nil)
	})
	loadFromFactoryDir := factorydefinitions.LoadedFactoryLoader(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("directory loader must not be used for canonical compile")
		return nil, nil
	})

	svc := compilationwire.NewService(
		loadCanonical,
		loadFromFactoryDir,
		stubEncodeFactory,
	)
	if loadCalls != 0 {
		t.Fatal("construction loaded a Factory")
	}

	_, err := svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`{"name":"alpha"}`),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource: %v", err)
	}
	if loadCalls != 1 {
		t.Fatalf("canonical loader calls = %d, want one", loadCalls)
	}
}
