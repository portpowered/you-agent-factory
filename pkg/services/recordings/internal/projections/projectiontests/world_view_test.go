package projections_test

import (
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	. "github.com/portpowered/infinite-you/pkg/services/recordings/internal/projections"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func assertCanonicalFactoryGraphPreserved(
	t *testing.T,
	worldState interfaces.FactoryWorldState,
	view interfaces.FactoryWorldView,
	want factoryapi.Factory,
) {
	t.Helper()

	if worldState.Factory == nil {
		t.Fatal("world state factory = nil, want canonical factory graph")
	}
	if view.Factory == nil {
		t.Fatal("world view factory = nil, want canonical factory graph")
	}
	if got := decodeFactorySnapshot(t, worldState.Factory); !reflect.DeepEqual(got, want) {
		t.Fatalf("world state factory = %#v, want canonical payload", got)
	}
	if got := decodeFactorySnapshot(t, view.Factory); !reflect.DeepEqual(got, want) {
		t.Fatalf("world view factory = %#v, want canonical payload", got)
	}
}

func decodeFactorySnapshot(t *testing.T, snapshot *interfaces.FactorySnapshot) factoryapi.Factory {
	t.Helper()
	var factory factoryapi.Factory
	if err := snapshot.Decode(&factory); err != nil {
		t.Fatalf("decode factory snapshot: %v", err)
	}
	return factory
}

func assertCanonicalFactoryWorkstationDetailsPreserved(
	t *testing.T,
	factory factoryapi.Factory,
	promptBody string,
	maxRetries int,
) {
	t.Helper()

	workstation := (*factory.Workstations)[0]
	assertCanonicalFactoryRoutePreserved(t, workstation.OnContinue, "continue", "onContinue")
	assertCanonicalFactoryRoutePreserved(t, workstation.OnRejection, "rejected", "onRejection")
	assertCanonicalFactoryRoutePreserved(t, workstation.OnFailure, "failed", "onFailure")
	if workstation.Body == nil || *workstation.Body != promptBody {
		t.Fatalf("body = %#v, want prompt body preserved", workstation.Body)
	}
	if workstation.Limits == nil || workstation.Limits.MaxRetries == nil || *workstation.Limits.MaxRetries != maxRetries {
		t.Fatalf("limits = %#v, want max retries preserved", workstation.Limits)
	}
}

func assertCanonicalFactoryRoutePreserved(
	t *testing.T,
	routes *[]factoryapi.WorkstationIO,
	wantState string,
	label string,
) {
	t.Helper()

	if routes == nil || len(*routes) != 1 || (*routes)[0].State != wantState {
		t.Fatalf("%s = %#v, want %s route", label, routes, wantState)
	}
}
