package wire_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
)

type stubLoadedSource struct {
	cfg *factorycontracts.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorycontracts.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                             { return "/factories/alpha" }
func (s stubLoadedSource) RuntimeBaseDir() string                         { return "/factories/alpha" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                       {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorycontracts.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*factorycontracts.FactoryWorkerConfig) error) error {
	return nil
}
func (s stubLoadedSource) Workstation(string) (*factorycontracts.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*factorycontracts.FactoryWorkerConfig, bool) {
	return nil, false
}

func stubLoadCanonical(payload []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
	var cfg factorycontracts.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factoryroot.ErrInvalidNamedFactory
	}
	return stubLoadedSource{cfg: &cfg}, nil
}

func stubEncodeFactory(cfg *factorycontracts.FactoryConfig) ([]byte, error) {
	return json.Marshal(cfg)
}

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	loadCanonical := factorycontracts.CanonicalFactoryJSONLoader(stubLoadCanonical)
	loadFromFactoryDir := factorycontracts.LoadedFactoryLoader(func(string, factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
		return nil, factoryroot.ErrInvalidNamedFactory
	})
	encodeFactory := factorycontracts.FactoryConfigJSONEncoder(stubEncodeFactory)

	if svc, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      nil,
		LoadFromFactoryDir: loadFromFactoryDir,
		EncodeFactory:      encodeFactory,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "canonical Factory loader is required") {
		t.Fatalf("NewService(nil LoadCanonical) = %#v, %v; want canonical loader required error", svc, err)
	}
	if svc, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loadCanonical,
		LoadFromFactoryDir: nil,
		EncodeFactory:      encodeFactory,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "directory loader is required") {
		t.Fatalf("NewService(nil LoadFromFactoryDir) = %#v, %v; want directory loader required error", svc, err)
	}
	if svc, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loadCanonical,
		LoadFromFactoryDir: loadFromFactoryDir,
		EncodeFactory:      nil,
	}); err == nil || svc != nil || !strings.Contains(err.Error(), "encoder is required") {
		t.Fatalf("NewService(nil EncodeFactory) = %#v, %v; want encoder required error", svc, err)
	}

	svc, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loadCanonical,
		LoadFromFactoryDir: loadFromFactoryDir,
		EncodeFactory:      encodeFactory,
	})
	if err != nil {
		t.Fatalf("NewService with exact injected ports: %v", err)
	}
	if svc == nil {
		t.Fatal("NewService returned nil service")
	}
	var _ compilationservice.Service = svc
}

func TestNewService_HostEffectsComeOnlyFromInjectedPorts(t *testing.T) {
	t.Parallel()

	loadCalls := 0
	loadCanonical := factorycontracts.CanonicalFactoryJSONLoader(func(payload []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
		loadCalls++
		return stubLoadCanonical(payload, nil)
	})
	loadFromFactoryDir := factorycontracts.LoadedFactoryLoader(func(string, factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
		t.Fatal("directory loader must not be used for canonical compile")
		return nil, nil
	})

	svc, err := compilationwire.NewService(compilationservice.Dependencies{
		LoadCanonical:      loadCanonical,
		LoadFromFactoryDir: loadFromFactoryDir,
		EncodeFactory:      stubEncodeFactory,
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	_, err = svc.CompileEffectiveFactorySource(
		context.Background(),
		factoryroot.CompileEffectiveFactorySourceRequest{
			Canonical:  []byte(`{"name":"alpha"}`),
			FactoryDir: "/factories/alpha",
		},
	)
	if err != nil {
		t.Fatalf("CompileEffectiveFactorySource: %v", err)
	}
	if loadCalls == 0 {
		t.Fatal("compile did not use the injected canonical loader port")
	}
}
