// Package wire constructs the Work admission nested subservice.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission"
	admissionservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/admission/internal/service"
)

// NewService constructs the private Work admission subservice.
func NewService() admission.Service {
	return admissionservice.New()
}
