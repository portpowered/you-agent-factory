package operatorsettingsservicewire

import (
	"context"
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

func TestNewCompositionRootRequiresDocumentOwner(t *testing.T) {
	t.Parallel()

	_, err := newCompositionRoot(nil, &stubResolutionService{})
	if err == nil || !strings.Contains(err.Error(), "document owner is required") {
		t.Fatalf("newCompositionRoot(nil, resolution) error = %v, want document owner required", err)
	}
}

func TestNewCompositionRootRequiresResolutionService(t *testing.T) {
	t.Parallel()

	_, err := newCompositionRoot(&stubDocumentOwner{}, nil)
	if err == nil || !strings.Contains(err.Error(), "resolution service is required") {
		t.Fatalf("newCompositionRoot(document, nil) error = %v, want resolution service required", err)
	}
}

type stubDocumentOwner struct{}

func (stubDocumentOwner) LoadDocument(operatorsettings.LoadDocumentRequest) (operatorsettings.LoadDocumentResult, error) {
	return operatorsettings.LoadDocumentResult{}, nil
}

func (stubDocumentOwner) MergeDocumentProviderModel(
	operatorsettings.Document,
	operatorsettings.DocumentProviderModelUpdate,
) (operatorsettings.Document, error) {
	return operatorsettings.Document{}, nil
}

func (stubDocumentOwner) ApplyDocumentUpdate(
	operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	return operatorsettings.ApplyDocumentUpdateResult{}, nil
}

func (stubDocumentOwner) PersistDocument(
	_ context.Context,
	_ operatorsettings.PersistDocumentRequest,
) error {
	return nil
}

type stubResolutionService struct{}

func (stubResolutionService) ResolveEffective(
	operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return operatorsettings.ResolveEffectiveResult{}, nil
}

var (
	_ operatorsettings.DocumentOwner = (*stubDocumentOwner)(nil)
	_ resolution.Service             = (*stubResolutionService)(nil)
)
