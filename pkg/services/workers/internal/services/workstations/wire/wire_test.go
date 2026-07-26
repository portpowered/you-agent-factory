package wire

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewServiceConstructsInertCapability(t *testing.T) {
	t.Parallel()

	service := NewService()
	if service == nil {
		t.Fatal("NewService() = nil")
	}
	if err := service.Route(context.Background(), "review"); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed route error = %v, want ErrWorkstationPoolUnavailable", err)
	}
}
