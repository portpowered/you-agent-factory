package construct_test

import (
	"strings"
	"testing"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	settingsconstruct "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/construct"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

func TestNewServiceFromConfigDocumentRejectsNilDocumentOwner(t *testing.T) {
	restore := settingsconstruct.SetConstructResolutionServiceForTests(func() (resolution.Service, error) {
		return &stubResolutionService{}, nil
	})
	t.Cleanup(restore)

	_, err := settingsconstruct.NewServiceFromConfigDocument(operatorsettings.ConfigDocumentService{})
	if err == nil || !strings.Contains(err.Error(), "document ports are required") {
		t.Fatalf("NewServiceFromConfigDocument() error = %v, want document ports required", err)
	}
}

type stubResolutionService struct{}

func (stubResolutionService) ResolveEffective(
	operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	return operatorsettings.ResolveEffectiveResult{}, nil
}

var _ resolution.Service = (*stubResolutionService)(nil)
