package apisurface

import (
	"errors"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestTopologyValidationErrorPreservesPublicContract(t *testing.T) {
	targets := []factoryapi.FactoryValidationTarget{{}}
	validationErr := NewTopologyValidationError("", targets)
	targets[0] = factoryapi.FactoryValidationTarget{}

	if got := validationErr.Error(); got != "factory topology validation failed" {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(validationErr, ErrInvalidNamedFactory) {
		t.Fatal("TopologyValidationError does not match ErrInvalidNamedFactory")
	}
	validationErr.Message = "invalid edge"
	if got := validationErr.Error(); got != "invalid edge" {
		t.Fatalf("custom Error() = %q", got)
	}
	var nilErr *TopologyValidationError
	if got := nilErr.Error(); got != "" {
		t.Fatalf("nil Error() = %q", got)
	}
}
