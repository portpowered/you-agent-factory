package apisurface

import (
	"encoding/json"
	"testing"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
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

func TestFactorySessionStatusOptionalRecoveryClientRoundTrip(t *testing.T) {
	t.Parallel()
	for _, recovery := range []*factorysessions.StartupRecovery{nil, {
		Code: "DURABLE_STATE_QUARANTINED", Cause: "INVALID_JSON",
		File: "board.json", QuarantinedFile: "board.json.unreadable.UTC.unique",
	}} {
		response := FactorySessionStatusToAPI(FactorySessionStatus{
			FactoryStatus:   factoryruntime.FactoryStatus{FactoryState: "RUNNING", RuntimeStatus: "ACTIVE"},
			StartupRecovery: recovery,
		})
		data, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		var client factoryclient.StatusResponse
		if err := json.Unmarshal(data, &client); err != nil {
			t.Fatal(err)
		}
		if client.FactoryState != "RUNNING" || client.RuntimeStatus != "ACTIVE" {
			t.Fatal("round trip lost runtime health")
		}
		if recovery == nil {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if _, exists := fields["startupRecovery"]; exists || client.StartupRecovery != nil {
				t.Fatal("normal status must omit diagnostic")
			}
		} else if got := client.StartupRecovery; got == nil || string(got.Cause) != recovery.Cause || string(got.Code) != recovery.Code || got.File != recovery.File || got.QuarantinedFile != recovery.QuarantinedFile {
			t.Fatalf("round trip recovery = %#v", got)
		}
	}
}
