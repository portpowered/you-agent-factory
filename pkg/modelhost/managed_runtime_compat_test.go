package modelhost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
)

func TestEnsureInvocationReady_AllowsInstalledAssetsWithoutLiveSupervisedSlot(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := NewCatalogHost(stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, Options{})

	managed, err := EnsureInvocationReady(context.Background(), host, loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("EnsureInvocationReady: %v", err)
	}
	if managed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateREADY {
		t.Fatalf("readiness = %s, want READY", managed.ReadinessState)
	}
}

func TestEnsureInvocationReady_BlocksWhenSupervisedRuntimeCrashed(t *testing.T) {
	healthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(healthServer.Close)

	exitCh := make(chan error, 1)
	loaded := mustLoadedCatalogConfig(t, supervisedCatalogFactoryConfig())
	host := newSupervisedTestHost(t, &fakeProcessLauncher{
		newProcess: func(spec ProcessStartSpec) *fakeManagedProcess {
			return newFakeManagedProcess(healthServer.URL, exitCh)
		},
	})

	lease, err := host.AcquireLease(context.Background(), loaded, "OMNIVOICE_Q4_K_M", LeaseOptions{})
	if err != nil {
		t.Fatalf("AcquireLease: %v", err)
	}
	exitCh <- errors.New("unexpected exit")

	deadline := time.Now().Add(2 * time.Second)
	for {
		managed, readinessErr := EnsureInvocationReady(context.Background(), host, loaded, "OMNIVOICE_Q4_K_M")
		if readinessErr != nil {
			var invocationErr *apisurface.ManagedRuntimeInvocationError
			if !errors.As(readinessErr, &invocationErr) {
				t.Fatalf("readiness error = %v, want *apisurface.ManagedRuntimeInvocationError", readinessErr)
			}
			if !errors.Is(readinessErr, apisurface.ErrManagedRuntimeFailed) {
				t.Fatalf("readiness error = %v, want ErrManagedRuntimeFailed", readinessErr)
			}
			if managed.ReadinessState != factoryapi.ManagedRuntimeReadinessStateFAILED {
				t.Fatalf("readiness = %s, want FAILED", managed.ReadinessState)
			}
			if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
				t.Fatalf("ReleaseLease: %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for invocation readiness failure; last readiness = %s", managed.ReadinessState)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
