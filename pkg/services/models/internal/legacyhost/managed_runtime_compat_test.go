package modelhost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	managedruntime "github.com/portpowered/infinite-you/pkg/services/models/internal/managedruntime"
)

func TestInspectReadiness_ReportsInstalledAssetsWithoutLiveSupervisedSlot(t *testing.T) {
	loaded := mustLoadedCatalogConfig(t, catalogFactoryConfig(true))
	host := mustNewCatalogHost(t, stubAssetGateway{
		byModel: map[string]CacheInspection{
			"OMNIVOICE_Q4_K_M": {
				Supported:          true,
				Installed:          true,
				InstalledFileCount: 2,
			},
		},
	}, testHostOptions{})

	snapshot, err := host.InspectReadiness(context.Background(), loaded, "OMNIVOICE_Q4_K_M")
	if err != nil {
		t.Fatalf("InspectReadiness: %v", err)
	}
	if snapshot.ReadinessState != managedruntime.ReadinessStateReady {
		t.Fatalf("readiness = %s, want READY", snapshot.ReadinessState)
	}
}

func TestInspectReadiness_ReportsSupervisedRuntimeCrash(t *testing.T) {
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
		snapshot, readinessErr := host.InspectReadiness(context.Background(), loaded, "OMNIVOICE_Q4_K_M")
		if readinessErr == nil && snapshot.ReadinessState == managedruntime.ReadinessStateFailed {
			if snapshot.FailureClass != FailureClassProcessCrash {
				t.Fatalf("failure class = %s, want %s", snapshot.FailureClass, FailureClassProcessCrash)
			}
			if err := host.ReleaseLease(context.Background(), lease.ID); err != nil {
				t.Fatalf("ReleaseLease: %v", err)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for failed readiness snapshot; last readiness = %s err=%v", snapshot.ReadinessState, readinessErr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
