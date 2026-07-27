package registry

import (
	"context"
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type fakeIntegration struct {
	identity inference.Identity
	maximum  inference.CapabilitySet
}

func (f *fakeIntegration) Identity() inference.Identity { return f.identity }
func (f *fakeIntegration) MaximumCapabilities() inference.CapabilitySet {
	return f.maximum
}
func (f *fakeIntegration) Discover(context.Context) (inference.Discovery, error) {
	panic("registry construction must not discover providers")
}
func (f *fakeIntegration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	panic("registry construction must not negotiate capabilities")
}
func (f *fakeIntegration) Invoke(context.Context, inference.InvocationRequest, inference.ResponseWriter) error {
	panic("registry construction must not invoke providers")
}

func TestNewJoinsSupportedCatalogManifestsWithoutProviderSideEffects(t *testing.T) {
	t.Parallel()

	registrations := supportedCatalogRegistrations(t)
	registry, err := New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if len(registry.manifests) != 8 || len(registry.integrations) != 8 {
		t.Fatalf("joined counts = (%d manifests, %d integrations), want (8, 8)", len(registry.manifests), len(registry.integrations))
	}
}

func TestBuiltInRegistrationsBuildAllSelectableBundledProviders(t *testing.T) {
	t.Parallel()

	registrations, err := BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	registry, err := New(registrations...)
	if err != nil {
		t.Fatalf("New(BuiltInRegistrations()) error = %v", err)
	}

	want := []string{"agy", "claude", "codex", "cursor", "gemini", "kiro", "opencode", "pi"}
	if got := entryIdentities(registry.Entries()); !reflect.DeepEqual(got, want) {
		t.Fatalf("built-in manifest identities = %v, want %v", got, want)
	}
	cursor, err := registry.Lookup("agent")
	if err != nil {
		t.Fatalf("Lookup(cursor alias) error = %v", err)
	}
	if cursor.Identity() != "cursor" {
		t.Fatalf("Lookup(cursor alias) identity = %q, want cursor", cursor.Identity())
	}
	integration, err := registry.Integration("agent")
	if err != nil {
		t.Fatalf("Integration(cursor alias) error = %v", err)
	}
	if integration.Identity() != "cursor" {
		t.Fatalf("Integration(cursor alias) identity = %q, want cursor", integration.Identity())
	}
}

func TestNewAcceptsDetachedSchemaValidExternalManifest(t *testing.T) {
	t.Parallel()

	registrations := supportedCatalogRegistrations(t)
	manifest := externalManifest(t, "customer.provider", "customer")
	localizedNames := map[string]string{"en-US": "Customer Provider"}
	manifest.DisplayName.Values = &localizedNames
	registrations = append(registrations, ExternalRegistration(manifest, integrationFor(manifest)))
	manifest.ID = "mutated"
	manifest.Aliases[0] = "mutated"
	localizedNames["en-US"] = "Mutated"

	registry, err := New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, ok := registry.manifests["customer.provider"]; !ok {
		t.Fatal("external manifest was not joined under its detached identity")
	}
	if _, ok := registry.manifests["mutated"]; ok {
		t.Fatal("caller mutation changed the registered external manifest")
	}
	storedNames := registry.manifests["customer.provider"].DisplayName.Values
	if storedNames == nil || (*storedNames)["en-US"] != "Customer Provider" {
		t.Fatal("caller mutation changed nested external manifest metadata")
	}
}

func TestBuildAcceptsCatalogOnlyAndNotSupportedManifestsWithoutImplementations(t *testing.T) {
	t.Parallel()

	catalogOnly := externalManifest(t, "catalog.entry", "catalog-alias")
	catalogOnly.ImplementationAvailability = ImplementationCatalogOnly
	notSupported := externalManifest(t, "unsupported.entry", "unsupported-alias")
	notSupported.TechnicalSupportLevel = SupportNotSupported

	registry, err := build([]Manifest{catalogOnly, notSupported}, nil)
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	if len(registry.manifests) != 2 || len(registry.integrations) != 0 {
		t.Fatalf("joined counts = (%d manifests, %d integrations), want (2, 0)", len(registry.manifests), len(registry.integrations))
	}
}

func TestNewRejectsInvalidRegistrationInvariants(t *testing.T) {
	t.Parallel()

	var typedNil *fakeIntegration
	tests := []struct {
		name   string
		mutate func([]Registration) []Registration
		want   []string
	}{
		{
			name: "nil integration",
			mutate: func(registrations []Registration) []Registration {
				index := catalogRegistrationIndex(registrations, "claude")
				registrations[index].integration = typedNil
				return registrations
			},
			want: []string{`"claude": integration is nil`, `"claude": supported bundled manifest has no matching implementation`},
		},
		{
			name: "mismatched integration identity",
			mutate: func(registrations []Registration) []Registration {
				index := catalogRegistrationIndex(registrations, "claude")
				registrations[index].integration = &fakeIntegration{
					identity: "different",
					maximum:  registrations[index].integration.MaximumCapabilities(),
				}
				return registrations
			},
			want: []string{`"claude": integration identity "different" differs from its manifest binding`},
		},
		{
			name: "malformed binding",
			mutate: func(registrations []Registration) []Registration {
				index := catalogRegistrationIndex(registrations, "claude")
				registrations[index].identity = " Bad Identity "
				return registrations
			},
			want: []string{`"bad identity": manifest binding identity is invalid`, `"bad identity": implementation has no matching manifest`, `"claude": supported bundled manifest has no matching implementation`},
		},
		{
			name: "not supported implementation",
			mutate: func(registrations []Registration) []Registration {
				manifest := externalManifest(t, "unsupported.provider", "unsupported")
				manifest.TechnicalSupportLevel = SupportNotSupported
				return append(registrations, ExternalRegistration(manifest, integrationFor(manifest)))
			},
			want: []string{`"unsupported.provider": non-selectable manifest cannot have a runnable integration`},
		},
		{
			name: "external manifest claims bundled availability",
			mutate: func(registrations []Registration) []Registration {
				manifest := externalManifest(t, "customer.provider", "customer")
				manifest.ImplementationAvailability = ImplementationBundled
				return append(registrations, ExternalRegistration(manifest, integrationFor(manifest)))
			},
			want: []string{`"customer.provider": external manifest implementation availability must be externally-supplied`},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(test.mutate(supportedCatalogRegistrations(t))...)
			assertErrorContains(t, err, test.want...)
		})
	}
}

func TestNewRejectsCoverageAndCapabilityContradictions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func([]Registration) []Registration
		want   string
	}{
		{
			name: "missing bundled implementation",
			mutate: func(registrations []Registration) []Registration {
				return withoutCatalogRegistration(registrations, "claude")
			},
			want: `"claude": supported bundled manifest has no matching implementation`,
		},
		{
			name: "implementation without manifest",
			mutate: func(registrations []Registration) []Registration {
				return append(registrations, CatalogRegistration("missing", &fakeIntegration{
					identity: "missing",
					maximum:  inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
				}))
			},
			want: `"missing": implementation has no matching manifest`,
		},
		{
			name: "maximum exceeds manifest",
			mutate: func(registrations []Registration) []Registration {
				index := catalogRegistrationIndex(registrations, "claude")
				current := registrations[index].integration.MaximumCapabilities().Values()
				current = append(current, inference.CapabilityProviderReconnect)
				registrations[index].integration = &fakeIntegration{identity: "claude", maximum: inference.NewCapabilitySet(current...)}
				return registrations
			},
			want: `"claude": integration maximum exceeds manifest maximum: provider_reconnect`,
		},
		{
			name: "maximum contradicts manifest",
			mutate: func(registrations []Registration) []Registration {
				index := catalogRegistrationIndex(registrations, "claude")
				registrations[index].integration = &fakeIntegration{
					identity: "claude",
					maximum:  inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
				}
				return registrations
			},
			want: `"claude": integration maximum contradicts manifest maximum by omitting: message_deltas`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(test.mutate(supportedCatalogRegistrations(t))...)
			assertErrorContains(t, err, test.want)
		})
	}
}

func TestNewRejectsEveryIdentityCollisionIndependentOfInputOrder(t *testing.T) {
	t.Parallel()

	base := supportedCatalogRegistrations(t)
	first := externalManifest(t, "customer.one", "customer-alias")
	second := externalManifest(t, "customer.two", "customer-alias")
	third := externalManifest(t, "agent", "third-alias")
	fourth := externalManifest(t, "customer.one", "fourth-alias")
	additions := []Registration{
		ExternalRegistration(first, integrationFor(first)),
		ExternalRegistration(second, integrationFor(second)),
		ExternalRegistration(third, integrationFor(third)),
		ExternalRegistration(fourth, integrationFor(fourth)),
	}

	var want string
	for seed := int64(0); seed < 20; seed++ {
		permuted := append([]Registration(nil), additions...)
		rand.New(rand.NewSource(seed)).Shuffle(len(permuted), func(i, j int) {
			permuted[i], permuted[j] = permuted[j], permuted[i]
		})
		_, err := New(append(append([]Registration(nil), base...), permuted...)...)
		if err == nil {
			t.Fatal("New() error = nil")
		}
		if seed == 0 {
			want = err.Error()
		} else if err.Error() != want {
			t.Fatalf("seed %d error differs\n got: %s\nwant: %s", seed, err, want)
		}
	}
	assertErrorContains(t, errorString(want),
		`"agent": identity collision between alias of "cursor", canonical "agent"`,
		`"customer-alias": identity collision between alias of "customer.one", alias of "customer.two"`,
		`"customer.one": identity collision between canonical "customer.one", canonical "customer.one"`,
	)
}

func TestNewRejectsCompatibilityAliasCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity string
		alias    string
		want     string
	}{
		{
			name:     "openai canonical shadows compatibility alias",
			identity: "openai",
			alias:    "customer-openai",
			want:     `"openai": identity collision between canonical "openai", compatibility alias of "codex"`,
		},
		{
			name:     "anthropic canonical shadows compatibility alias",
			identity: "anthropic",
			alias:    "customer-anthropic",
			want:     `"anthropic": identity collision between canonical "anthropic", compatibility alias of "claude"`,
		},
		{
			name:     "external alias shadows openai compatibility alias",
			identity: "customer.openai",
			alias:    "openai",
			want:     `"openai": identity collision between alias of "customer.openai", compatibility alias of "codex"`,
		},
		{
			name:     "external alias shadows anthropic compatibility alias",
			identity: "customer.anthropic",
			alias:    "anthropic",
			want:     `"anthropic": identity collision between alias of "customer.anthropic", compatibility alias of "claude"`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			manifest := externalManifest(t, test.identity, test.alias)
			registrations := append(
				supportedCatalogRegistrations(t),
				ExternalRegistration(manifest, integrationFor(manifest)),
			)
			_, err := New(registrations...)
			assertErrorContains(t, err, test.want)
		})
	}
}

func TestNewRejectsMalformedExternalManifestWithStableDiagnostics(t *testing.T) {
	t.Parallel()

	manifest := externalManifest(t, "customer.valid", "customer-alias")
	manifest.ID = " INVALID "
	registrations := append(supportedCatalogRegistrations(t), ExternalRegistration(manifest, &fakeIntegration{
		identity: "invalid",
		maximum:  inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
	}))
	_, err := New(registrations...)
	assertErrorContains(t, err,
		`"invalid": external manifest violates the published schema`,
	)
}

func supportedCatalogRegistrations(t *testing.T) []Registration {
	t.Helper()
	var registrations []Registration
	for _, manifest := range publishedCatalog(t) {
		if manifest.TechnicalSupportLevel == SupportNotSupported ||
			manifest.ImplementationAvailability == ImplementationCatalogOnly {
			continue
		}
		registrations = append(registrations, CatalogRegistration(inference.Identity(manifest.ID), integrationFor(manifest)))
	}
	return registrations
}

func externalManifest(t *testing.T, identity, alias string) Manifest {
	t.Helper()
	manifest := cloneManifest(findManifest(t, publishedCatalog(t), "codex"))
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = ImplementationExternallySupplied
	return manifest
}

func publishedCatalog(t *testing.T) []Manifest {
	t.Helper()
	var catalog catalogDocument
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("parse published catalog: %v", err)
	}
	return catalog.Providers
}

func catalogRegistrationIndex(registrations []Registration, identity string) int {
	for index := range registrations {
		if registrationIdentity(registrations[index]) == identity {
			return index
		}
	}
	return -1
}

func withoutCatalogRegistration(registrations []Registration, identity string) []Registration {
	filtered := make([]Registration, 0, len(registrations))
	for _, registration := range registrations {
		if registrationIdentity(registration) != identity {
			filtered = append(filtered, registration)
		}
	}
	return filtered
}

func findManifest(t *testing.T, manifests []Manifest, identity string) Manifest {
	t.Helper()
	for _, manifest := range manifests {
		if manifest.ID == identity {
			return manifest
		}
	}
	t.Fatalf("manifest %q not found", identity)
	return Manifest{}
}

func integrationFor(manifest Manifest) inference.Integration {
	return &fakeIntegration{
		identity: inference.Identity(manifest.ID),
		maximum:  manifestCapabilities(manifest),
	}
}

func assertErrorContains(t *testing.T, err error, fragments ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("error = nil")
	}
	for _, fragment := range fragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not contain %q", err, fragment)
		}
	}
}

type errorString string

func (e errorString) Error() string { return string(e) }
