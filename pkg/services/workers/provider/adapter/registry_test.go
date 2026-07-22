package adapter_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

func TestRegistry_LookupIsNormalizedAndDeterministic(t *testing.T) {
	t.Parallel()

	zulu := &recordingAdapter{identity: "zulu"}
	alpha := &recordingAdapter{identity: "Alpha"}
	registry, err := adapter.NewRegistry(zulu, alpha)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if got := registry.Identities(); !reflect.DeepEqual(got, []adapter.Identity{"alpha", "zulu"}) {
		t.Fatalf("Identities() = %#v", got)
	}
	got, err := registry.Lookup(" ALPHA ")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got != alpha {
		t.Fatalf("Lookup() = %T %p, want alpha %p", got, got, alpha)
	}

	identities := registry.Identities()
	identities[0] = "mutated"
	if got := registry.Identities()[0]; got != "alpha" {
		t.Fatalf("registry identities mutated through caller slice: %q", got)
	}
}

func TestRegistry_RejectsDuplicateNormalizedIdentity(t *testing.T) {
	t.Parallel()

	_, err := adapter.NewRegistry(
		&recordingAdapter{identity: "fake"},
		&recordingAdapter{identity: " FAKE "},
	)
	if !errors.Is(err, adapter.ErrDuplicateProvider) {
		t.Fatalf("NewRegistry() error = %v, want ErrDuplicateProvider", err)
	}
}

func TestRegistry_UnknownIdentityDoesNotSelectDefault(t *testing.T) {
	t.Parallel()

	registry, err := adapter.NewRegistry(&recordingAdapter{identity: "known"})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	got, err := registry.Lookup("missing")
	if got != nil {
		t.Fatalf("Lookup() adapter = %#v, want nil", got)
	}
	if !errors.Is(err, adapter.ErrUnknownProvider) {
		t.Fatalf("Lookup() error = %v, want ErrUnknownProvider", err)
	}
}

func TestRegistry_RejectsNilAndEmptyIdentityAdapters(t *testing.T) {
	t.Parallel()

	var typedNil *recordingAdapter
	for _, candidates := range [][]adapter.Adapter{
		{nil},
		{typedNil},
		{&recordingAdapter{}},
	} {
		_, err := adapter.NewRegistry(candidates...)
		if !errors.Is(err, adapter.ErrInvalidProvider) {
			t.Fatalf("NewRegistry(%#v) error = %v, want ErrInvalidProvider", candidates, err)
		}
	}
}
