package service

import (
	"context"
	"testing"
	"time"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	modelhost "github.com/portpowered/infinite-you/pkg/services/models/internal/host"
	localmodels "github.com/portpowered/infinite-you/pkg/services/models/internal/local"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestServiceOwnsLeaseAndLocalModelInvocation(t *testing.T) {
	worker := modelRuntimeWorker{
		Name: "voice-local", Type: models.RuntimeWorkerTypeModel,
		Model: "voice", ModelLocality: models.RuntimeModelLocalityLocal,
		Resources: []modelRuntimeResource{{Name: "voice-cache"}},
	}
	cfg := &testFactoryConfig{
		Name: "test", Workers: []modelRuntimeWorker{worker},
		Resources: []modelRuntimeResource{{
			Name: "voice-cache", Type: models.RuntimeResourceTypeModel, Model: "voice",
			Backend: "TEST", LoadPolicy: "ON_DEMAND",
		}},
	}
	loaded := projectTestModelsRuntimeConfig(t.TempDir(), cfg)
	host := &leaseTestHost{}
	runtime := &leaseTestRuntime{}
	execution, err := newLocalExecutor(
		func() *modelRuntimeConfig { return loaded },
		host, leaseTestAssets{}, runtime, nil, nil, models.LocalRuntimeHooks{},
		time.Now,
	)
	if err != nil {
		t.Fatalf("new local execution: %v", err)
	}

	result, err := execution.InvokeLocal(context.Background(), models.LocalInvocationRequest{
		Holder: "dispatch-1",
		Worker: models.LocalWorker{
			Name: worker.Name, Type: worker.Type, Model: worker.Model,
			ModelLocality: worker.ModelLocality,
			Resources:     []models.LocalResource{{Name: "voice-cache"}},
		},
		Resources: []models.LocalResource{{
			Name: "voice-cache", Type: models.RuntimeResourceTypeModel, Model: "voice",
			Backend: "TEST", LoadPolicy: "ON_DEMAND",
		}},
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-1"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !result.Handled || result.Content != "local" || host.acquires != 1 || host.releases != 1 || runtime.loads != 1 {
		t.Fatalf("result=%q acquire/release=%d/%d loads=%d", result.Content, host.acquires, host.releases, runtime.loads)
	}
}

type leaseTestHost struct{ acquires, releases int }

func (*leaseTestHost) ResolveIdentity(context.Context, *modelRuntimeConfig, string) (modelhost.Identity, error) {
	return modelhost.Identity{}, nil
}
func (*leaseTestHost) InspectReadiness(context.Context, *modelRuntimeConfig, string) (modelhost.ReadinessSnapshot, error) {
	return modelhost.ReadinessSnapshot{}, nil
}
func (*leaseTestHost) Pull(context.Context, *modelRuntimeConfig, string) (modelhost.PullSnapshot, error) {
	return modelhost.PullSnapshot{}, nil
}
func (h *leaseTestHost) AcquireLease(context.Context, *modelRuntimeConfig, string, modelhost.LeaseOptions) (modelhost.Lease, error) {
	h.acquires++
	return modelhost.Lease{ID: "lease-1", Endpoint: "http://local"}, nil
}
func (h *leaseTestHost) ReleaseLease(context.Context, string) error {
	h.releases++
	return nil
}
func (*leaseTestHost) Unload(context.Context, *modelRuntimeConfig, string) error {
	return nil
}

type leaseTestAssets struct{}

func (leaseTestAssets) PullModel(context.Context, *modelRuntimeConfig, string) (models.PullResult, error) {
	return models.PullResult{}, nil
}
func (leaseTestAssets) EnsureModelAvailable(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) error {
	return nil
}
func (leaseTestAssets) ResolveModelCache(context.Context, *modelRuntimeConfig, *modelRuntimeWorker) (localmodels.CacheLayout, error) {
	return localmodels.CacheLayout{ModelName: "voice", CachePath: "cache"}, nil
}
func (leaseTestAssets) InspectRuntimeCache(context.Context, *modelRuntimeConfig, string) (localmodels.RuntimeCacheInspection, error) {
	return localmodels.RuntimeCacheInspection{Supported: true, Installed: true}, nil
}

type leaseTestRuntime struct{ loads int }

func (*leaseTestRuntime) Supports(modelRuntimeResource, *modelRuntimeWorker) bool {
	return true
}
func (r *leaseTestRuntime) Load(context.Context, localmodels.LoadRequest) (localmodels.Handle, error) {
	r.loads++
	return leaseTestHandle{}, nil
}

type leaseTestHandle struct{}

func (leaseTestHandle) Invoke(context.Context, localmodels.InvocationRequest) (localmodels.InvocationResponse, error) {
	return localmodels.InvocationResponse{Content: "local"}, nil
}
