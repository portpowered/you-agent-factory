package service

import (
	"context"
	"errors"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

func TestService_AcquireLease_ReturnsRuntimeNotReadyWhenHostMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		runtimeConfigLookup: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		clock:               time.Now,
	}

	_, err := svc.AcquireLease(context.Background(), models.AcquireLeaseRequest{ModelName: "local-model"})
	if !errors.Is(err, models.ErrHostRuntimeNotReady) {
		t.Fatalf("AcquireLease nil host = %v, want ErrHostRuntimeNotReady", err)
	}
}

func TestService_ReleaseLease_ReturnsLeaseNotFoundWhenHostMissing(t *testing.T) {
	t.Parallel()

	svc := &Service{
		runtimeConfigLookup: func() *models.RuntimeConfig { return &models.RuntimeConfig{} },
		loggerValue:         zap.NewNop(),
		clock:               time.Now,
	}

	err := svc.ReleaseLease(context.Background(), models.ReleaseLeaseRequest{LeaseID: "lease-1"})
	if !errors.Is(err, models.ErrHostLeaseNotFound) {
		t.Fatalf("ReleaseLease nil host = %v, want ErrHostLeaseNotFound", err)
	}
}
