package wire_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	compilationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/contracts"
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

func TestNewService_RequiresExactInjectedPorts(t *testing.T) {
	t.Parallel()

	loadCanonical := compilationcontracts.CanonicalFactoryLoader(stubLoadCanonical)
	loadFromFactoryDir := compilationcontracts.LoadedFactoryLoader(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, factoryroot.ErrInvalidNamedFactory
	})
	encodeFactory := compilationcontracts.FactoryConfigEncoder(stubEncodeFactory)

	if svc, err := compilationwire.NewService(nil, loadFromFactoryDir, encodeFactory); err == nil || svc != nil || !strings.Contains(err.Error(), "canonical Factory loader is required") {
		t.Fatalf("NewService(nil LoadCanonical) = %#v, %v; want canonical loader required error", svc, err)
	}
	if svc, err := compilationwire.NewService(loadCanonical, nil, encodeFactory); err == nil || svc != nil || !strings.Contains(err.Error(), "directory loader is required") {
		t.Fatalf("NewService(nil LoadFromFactoryDir) = %#v, %v; want directory loader required error", svc, err)
	}
	if svc, err := compilationwire.NewService(loadCanonical, loadFromFactoryDir, nil); err == nil || svc != nil || !strings.Contains(err.Error(), "encoder is required") {
		t.Fatalf("NewService(nil EncodeFactory) = %#v, %v; want encoder required error", svc, err)
	}

	svc, err := compilationwire.NewService(loadCanonical, loadFromFactoryDir, encodeFactory)
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
	loadCanonical := compilationcontracts.CanonicalFactoryLoader(func(payload []byte, _ factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		loadCalls++
		return stubLoadCanonical(payload, nil)
	})
	loadFromFactoryDir := compilationcontracts.LoadedFactoryLoader(func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
		t.Fatal("directory loader must not be used for canonical compile")
		return nil, nil
	})

	svc, err := compilationwire.NewService(loadCanonical, loadFromFactoryDir, stubEncodeFactory)
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
