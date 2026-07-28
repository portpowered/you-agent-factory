package operatorsettingsservicewire

import (
	"errors"
	"strings"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	globalconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/globalconfig"
)

func TestNewServiceFromHomePortsPropagatesResolutionConstructionErrors(t *testing.T) {

	previous := constructResolutionService
	t.Cleanup(func() { constructResolutionService = previous })
	constructResolutionService = func() (resolution.Service, error) {
		return nil, errors.New("resolution construction failed")
	}

	_, err := NewServiceFromHomePorts(platformfilesystem.Local{}, globalconfigmapping.Decode)
	if err == nil || !strings.Contains(err.Error(), "resolution construction failed") {
		t.Fatalf("NewServiceFromHomePorts() error = %v, want resolution construction failure", err)
	}
}

func TestNewServiceFromConfigDocumentPropagatesResolutionConstructionErrors(t *testing.T) {

	previous := constructResolutionService
	t.Cleanup(func() { constructResolutionService = previous })
	constructResolutionService = func() (resolution.Service, error) {
		return nil, errors.New("resolution construction failed")
	}

	_, err := NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{
		Files:   platformfilesystem.Local{},
		Decoder: globalconfigmapping.Decode,
	})
	if err == nil || !strings.Contains(err.Error(), "resolution construction failed") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want resolution construction failure", err)
	}
}

func TestDefaultResolutionServiceConstructsAcceptedResolutionRoot(t *testing.T) {
	t.Parallel()

	service, err := defaultResolutionService()
	if err != nil {
		t.Fatalf("defaultResolutionService() error = %v", err)
	}
	if service == nil {
		t.Fatal("defaultResolutionService() = nil, want resolution service")
	}
}

func TestDefaultResolutionServicePropagatesProvidersRootErrors(t *testing.T) {

	previousProviders := constructProvidersRoot
	t.Cleanup(func() { constructProvidersRoot = previousProviders })
	constructProvidersRoot = func() (providers.Service, error) {
		return nil, errors.New("providers root failed")
	}

	_, err := defaultResolutionService()
	if err == nil || !strings.Contains(err.Error(), "construct providers root") {
		t.Fatalf("defaultResolutionService() error = %v, want providers root failure", err)
	}
}

func TestDefaultResolutionServicePropagatesResolutionWireErrors(t *testing.T) {

	previousResolution := constructResolutionWire
	t.Cleanup(func() { constructResolutionWire = previousResolution })
	constructResolutionWire = func(providers.Service) (resolution.Service, error) {
		return nil, errors.New("resolution wire failed")
	}

	_, err := defaultResolutionService()
	if err == nil || !strings.Contains(err.Error(), "construct resolution service") {
		t.Fatalf("defaultResolutionService() error = %v, want resolution wire failure", err)
	}
}
