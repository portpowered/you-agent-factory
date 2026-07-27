package wire_test

import (
	"encoding/json"
	"testing"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	validationservice "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation"
	validationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/wire"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

type stubLoadedSource struct {
	cfg *factorycontracts.FactoryConfig
}

func (s stubLoadedSource) FactoryConfig() *factorycontracts.FactoryConfig { return s.cfg }
func (s stubLoadedSource) FactoryDir() string                             { return "" }
func (s stubLoadedSource) RuntimeBaseDir() string                         { return "" }
func (s stubLoadedSource) SetRuntimeBaseDir(string)                       {}
func (s stubLoadedSource) PortableBundledFileReplacements() []factorycontracts.PortableBundledFileReplacement {
	return nil
}
func (s stubLoadedSource) MutateWorkers(func(*workerconfig.Config) error) error { return nil }
func (s stubLoadedSource) Workstation(string) (*factorycontracts.FactoryWorkstationConfig, bool) {
	return nil, false
}
func (s stubLoadedSource) Worker(string) (*workerconfig.Config, bool) { return nil, false }

func stubLoadCanonical(payload []byte, _ factorycontracts.WorkstationLoader) (factorycontracts.MutableLoadedFactorySource, error) {
	var cfg factorycontracts.FactoryConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		return nil, factoryroot.ErrInvalidNamedFactory
	}
	return stubLoadedSource{cfg: &cfg}, nil
}

func TestWire_NewServiceConstructsValidationSubservice(t *testing.T) {
	t.Parallel()
	validator := factoryvalidation.New(nil)
	svc, err := validationwire.NewService(validationservice.Dependencies{
		Operations:    validator,
		Effective:     validator,
		LoadCanonical: stubLoadCanonical,
	})
	if err != nil {
		t.Fatalf("validationwire.NewService: %v", err)
	}
	var _ validationservice.Service = svc
}

func TestWire_NewServiceRejectsMissingDependencies(t *testing.T) {
	t.Parallel()
	if _, err := validationwire.NewService(validationservice.Dependencies{}); err == nil {
		t.Fatal("expected dependency error")
	}
}
