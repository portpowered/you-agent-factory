package internal

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog"
)

func TestCatalogListIsDeterministicAndReportsAvailability(t *testing.T) {
	t.Parallel()

	service, err := New([]catalog.Descriptor{
		{Provider: providers.Provider{ID: "zeta-acp", ExecutionKind: providers.ExecutionKindACP, Command: "missing"}},
		{Provider: providers.Provider{ID: "alpha-acp", ExecutionKind: providers.ExecutionKindACP, Command: "ready", Arguments: []string{"acp"}}},
	}, executableLocator{"ready": true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	listed, err := service.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got, want := []providers.ID{listed[0].Provider.ID, listed[1].Provider.ID}, []providers.ID{"alpha-acp", "zeta-acp"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("listed identities = %v, want %v", got, want)
	}
	if got := listed[0].Provider.Availability.State; got != providers.AvailabilityAvailable {
		t.Fatalf("alpha availability = %q, want AVAILABLE", got)
	}
	if got := listed[1].Provider.Availability; got.State != providers.AvailabilityUnavailable || got.Detail == "" {
		t.Fatalf("zeta availability = %#v, want unavailable with safe detail", got)
	}

	listed[0].Provider.Arguments[0] = "mutated"
	again, err := service.Get(context.Background(), "alpha-acp")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got := again.Provider.Arguments[0]; got != "acp" {
		t.Fatalf("catalog arguments leaked caller mutation: %q", got)
	}
}

func TestCatalogAcceptsEmptyAndRejectsDuplicateIdentity(t *testing.T) {
	t.Parallel()

	empty, err := New(nil, executableLocator{})
	if err != nil {
		t.Fatalf("New(empty) error = %v", err)
	}
	listed, err := empty.List(context.Background())
	if err != nil || len(listed) != 0 {
		t.Fatalf("List(empty) = (%v, %v), want empty success", listed, err)
	}

	_, err = New([]catalog.Descriptor{
		{Provider: providers.Provider{ID: "cursor-acp", ExecutionKind: providers.ExecutionKindACP}},
		{Provider: providers.Provider{ID: "cursor-acp", ExecutionKind: providers.ExecutionKindACP}},
	}, executableLocator{})
	if err == nil {
		t.Fatal("New(duplicate) error = nil")
	}
}

func TestCatalogUnknownIdentityIsTyped(t *testing.T) {
	t.Parallel()

	service, err := New(nil, executableLocator{})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, err = service.Get(context.Background(), "missing-acp")
	if !errors.Is(err, providers.ErrUnknownProvider) {
		t.Fatalf("Get(unknown) error = %v, want ErrUnknownProvider", err)
	}
}

func TestCatalogAliasResolvesCanonicalDescriptorWithoutDuplicateListing(t *testing.T) {
	t.Parallel()

	service, err := New([]catalog.Descriptor{{
		Provider: providers.Provider{ID: "droid-acp", ExecutionKind: providers.ExecutionKindACP, Command: "droid"},
		Aliases:  []providers.ID{"factory-droid", "factorydroid"},
	}}, executableLocator{"droid": true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	listed, err := service.List(context.Background())
	if err != nil || len(listed) != 1 {
		t.Fatalf("List() = (%#v, %v), want one canonical entry", listed, err)
	}
	resolved, err := service.Get(context.Background(), "factory-droid")
	if err != nil {
		t.Fatalf("Get(alias) error = %v", err)
	}
	if resolved.Provider.ID != "droid-acp" {
		t.Fatalf("alias resolved identity = %q, want droid-acp", resolved.Provider.ID)
	}
}

type executableLocator map[string]bool

func (locator executableLocator) LookPath(file string) (string, error) {
	if locator[file] {
		return file, nil
	}
	return "", errors.New("not found")
}
