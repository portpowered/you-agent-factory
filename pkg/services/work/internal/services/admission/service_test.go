package admission_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
	admissionwire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission/wire"
)

func TestWireConstructsSingularAdmissionService(t *testing.T) {
	t.Parallel()

	svc := admissionwire.NewService()
	if svc == nil {
		t.Fatal("wire.NewService() returned nil")
	}
	var _ admission.Service = svc
}
