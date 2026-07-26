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
	if _, err := service.Route(
		context.Background(),
		workers.WorkstationRouteRequest{WorkstationName: "review"},
	); !errors.Is(err, workers.ErrWorkstationPoolUnavailable) {
		t.Fatalf("constructed route error = %v, want ErrWorkstationPoolUnavailable", err)
	}
}
