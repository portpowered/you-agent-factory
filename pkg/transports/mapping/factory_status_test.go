package apisurface

import (
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestFactoryStatusToAPIMapsDetachedOwnerResultWithoutPolicy(t *testing.T) {
	got := FactoryStatusToAPI(factoryruntime.FactoryStatus{
		FactoryState: "RUNNING", RuntimeStatus: "ACTIVE", LifecycleControlStatus: "PAUSED", TotalTokens: 3,
		Categories: factoryruntime.FactoryStatusCategories{Initial: 1, Processing: 2},
		Resources:  []factoryruntime.FactoryResourceUsage{{Name: "gpu", Available: 1, Total: 2}},
	})

	if got.FactoryState != "RUNNING" || got.RuntimeStatus != "ACTIVE" || got.TotalTokens != 3 ||
		got.Categories.Initial != 1 || got.Categories.Processing != 2 {
		t.Fatalf("mapped status = %#v, want owner fields preserved", got)
	}
	if got.LifecycleControlStatus == nil || *got.LifecycleControlStatus != "PAUSED" ||
		got.Resources == nil || len(*got.Resources) != 1 || (*got.Resources)[0].Name != "gpu" {
		t.Fatalf("mapped optional status fields = %#v, want lifecycle and gpu resource", got)
	}
}
