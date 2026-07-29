package registry

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	inference "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/provider/inferencecontract"
)

type recordingIntegration struct {
	identity        inference.Identity
	maximum         inference.CapabilitySet
	negotiated      inference.CapabilitySet
	discovery       inference.Discovery
	capabilitiesErr error
	discoveryErr    error
	discoveryCalls  int
	capabilityCalls int
	invocationCalls int
}

func (r *recordingIntegration) Identity() inference.Identity { return r.identity }
func (r *recordingIntegration) MaximumCapabilities() inference.CapabilitySet {
	return r.maximum
}
func (r *recordingIntegration) Discover(context.Context) (inference.Discovery, error) {
	r.discoveryCalls++
	return r.discovery, r.discoveryErr
}
func (r *recordingIntegration) Capabilities(context.Context, inference.InvocationRequest) (inference.CapabilitySet, error) {
	r.capabilityCalls++
	return r.negotiated, r.capabilitiesErr
}
func (r *recordingIntegration) Invoke(context.Context, inference.InvocationRequest, inference.ResponseWriter) error {
	r.invocationCalls++
	return nil
}

func TestRegistryStaticQueriesAreNormalizedDeterministicDetachedAndInert(t *testing.T) {
	t.Parallel()

	registry, recordings := newRecordingRegistry(t)
	entry, err := registry.Lookup("  AGENT  ")
	if err != nil {
		t.Fatalf("Lookup(alias) error = %v", err)
	}
	if entry.Identity() != "cursor" {
		t.Fatalf("Lookup(alias).Identity() = %q, want cursor", entry.Identity())
	}
	entries := registry.Entries()
	identities := entryIdentities(entries)
	if !slices.IsSorted(identities) {
		t.Fatalf("Entries() identities = %v, want canonical order", identities)
	}
	if len(entries) != 8 {
		t.Fatalf("Entries() count = %d, want 8", len(entries))
	}

	cursorEntry := findEntry(t, entries, "cursor")
	manifest := cursorEntry.Manifest()
	if !slices.IsSorted(cursorEntry.Aliases()) ||
		!slices.IsSorted(cursorEntry.DiscoveryPrerequisites().ConfigurationKeys) ||
		!slices.IsSorted(cursorEntry.DiscoveryPrerequisites().EndpointKinds) ||
		!slices.IsSorted(cursorEntry.DiscoveryPrerequisites().ExecutableNames) {
		t.Fatal("entry slice projections are not in canonical order")
	}
	manifest.Aliases[0] = "mutated"
	aliases := entry.Aliases()
	aliases[0] = "mutated"
	prerequisites := entry.DiscoveryPrerequisites()
	prerequisites.ExecutableNames[0] = "mutated"
	maximum := entry.MaximumCapabilities().Values()
	maximum[0] = inference.CapabilityUsage

	again, err := registry.Lookup("agent")
	if err != nil {
		t.Fatalf("Lookup(alias again) error = %v", err)
	}
	if again.Aliases()[0] == "mutated" ||
		again.DiscoveryPrerequisites().ExecutableNames[0] == "mutated" ||
		!again.MaximumCapabilities().Has(inference.CapabilityPromptSubmission) {
		t.Fatal("caller mutation changed later registry results")
	}
	assertProviderCalls(t, recordings, 0, 0, 0)
}

func TestRegistryLookupRejectsInvalidUnknownAndNonSelectableIdentities(t *testing.T) {
	t.Parallel()

	registry, recordings := newRecordingRegistry(t)
	tests := []struct {
		identity string
		want     string
	}{
		{identity: "", want: `provider lookup "<empty>" is invalid`},
		{identity: "bad identity", want: `provider lookup "bad identity" is invalid`},
		{identity: "missing", want: `provider "missing" is unknown`},
	}
	for _, test := range tests {
		_, err := registry.Lookup(test.identity)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("Lookup(%q) error = %v, want containing %q", test.identity, err, test.want)
		}
	}
	assertProviderCalls(t, recordings, 0, 0, 0)
}

func TestRegistryDiagnosticsDistinguishStaticAvailabilityWithoutDiscovery(t *testing.T) {
	t.Parallel()

	catalogOnly := externalManifest(t, "catalog.entry", "catalog")
	catalogOnly.ImplementationAvailability = ImplementationCatalogOnly
	unavailable := externalManifest(t, "external.entry", "external")
	registry, err := build([]Manifest{catalogOnly, unavailable}, nil)
	if err != nil {
		t.Fatalf("build() error = %v", err)
	}
	got := registry.SupportedProviders()
	if len(got) != 2 ||
		got[0].Entry().Identity() != "catalog.entry" ||
		got[0].Availability() != AvailabilityCatalogOnly ||
		got[1].Entry().Identity() != "external.entry" ||
		got[1].Availability() != AvailabilitySupportedButUnavailable {
		t.Fatalf("SupportedProviders() = %#v, want catalog-only then supported-but-unavailable", got)
	}

	fullRegistry, recordings := newRecordingRegistry(t)
	availability := make(map[inference.Identity]Availability)
	for _, diagnostic := range fullRegistry.SupportedProviders() {
		availability[diagnostic.Entry().Identity()] = diagnostic.Availability()
	}
	if availability["agy"] != AvailabilitySelectable ||
		availability["claude"] != AvailabilitySelectable {
		t.Fatalf("availability = %v", availability)
	}
	assertProviderCalls(t, recordings, 0, 0, 0)
}

func TestRegistryExplicitOperationsDelegateOnlyToResolvedIntegration(t *testing.T) {
	t.Parallel()

	registry, recordings := newRecordingRegistry(t)
	request := inference.NewInvocationRequest(inference.InvocationInput{
		Required: inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
	})
	assertNegotiatedCapabilityOperation(t, registry, request)
	assertDiscoveryOperation(t, registry)
	assertInvocationAccess(t, registry, request)
	if _, err := registry.MaximumCapabilities("agent"); err != nil {
		t.Fatalf("MaximumCapabilities() error = %v", err)
	}
	assertOnlyCursorReceivedExplicitCalls(t, recordings)
}

func assertNegotiatedCapabilityOperation(
	t *testing.T,
	registry *Registry,
	request inference.InvocationRequest,
) {
	t.Helper()
	capabilities, err := registry.Capabilities(context.Background(), "agent", request)
	if err != nil || !capabilities.Has(inference.CapabilityPromptSubmission) {
		t.Fatalf("Capabilities() = %v, %v", capabilities.Values(), err)
	}
	if capabilities.Values()[0] != inference.CapabilityPromptSubmission {
		t.Fatalf("Capabilities() = %v, want canonical manifest order", capabilities.Values())
	}
}

func assertDiscoveryOperation(t *testing.T, registry *Registry) {
	t.Helper()
	discovery, err := registry.Discover(context.Background(), "cursor")
	if err != nil || discovery.Readiness() != inference.ReadinessReady {
		t.Fatalf("Discover() = %#v, %v", discovery, err)
	}
	if discovery.Prerequisites()[0].Kind() != inference.PrerequisiteConfiguration {
		t.Fatalf("Discover() prerequisites = %#v, want canonical order", discovery.Prerequisites())
	}
}

func assertInvocationAccess(t *testing.T, registry *Registry, request inference.InvocationRequest) {
	t.Helper()
	integration, err := registry.Integration(" AGENT ")
	if err != nil || integration.Identity() != "cursor" {
		t.Fatalf("Integration() identity = %q, %v", integration.Identity(), err)
	}
	if err := integration.Invoke(context.Background(), request, nil); err != nil {
		t.Fatalf("resolved Integration.Invoke() error = %v", err)
	}
}

func assertOnlyCursorReceivedExplicitCalls(
	t *testing.T,
	recordings map[string]*recordingIntegration,
) {
	t.Helper()
	cursor := recordings["cursor"]
	if cursor.capabilityCalls != 1 || cursor.discoveryCalls != 1 || cursor.invocationCalls != 1 {
		t.Fatalf("cursor calls = capabilities %d, discovery %d, invocation %d", cursor.capabilityCalls, cursor.discoveryCalls, cursor.invocationCalls)
	}
	for identity, recording := range recordings {
		if identity != "cursor" && (recording.capabilityCalls != 0 || recording.discoveryCalls != 0 || recording.invocationCalls != 0) {
			t.Fatalf("%s received unexpected calls", identity)
		}
	}
}

func TestRegistryRejectsInvalidNegotiatedCapabilitiesAndDiscovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(*recordingIntegration)
		call   func(*Registry) error
		want   string
	}{
		{
			name: "unknown capability",
			mutate: func(integration *recordingIntegration) {
				integration.negotiated = inference.NewCapabilitySet(inference.CapabilityPromptSubmission, "unknown")
			},
			call: capabilityCall,
			want: `returned invalid capabilities`,
		},
		{
			name: "above manifest maximum",
			mutate: func(integration *recordingIntegration) {
				integration.negotiated = inference.NewCapabilitySet(
					inference.CapabilityPromptSubmission,
					inference.CapabilityProviderReconnect,
					inference.CapabilitySessionResume,
				)
			},
			call: capabilityCall,
			want: `returned invalid capabilities`,
		},
		{
			name: "contradictory capability dependencies",
			mutate: func(integration *recordingIntegration) {
				integration.negotiated = inference.NewCapabilitySet(
					inference.CapabilityPromptSubmission,
					inference.CapabilityMessageDeltas,
				)
			},
			call: capabilityCall,
			want: `returned invalid capabilities`,
		},
		{
			name: "omits required capability",
			mutate: func(integration *recordingIntegration) {
				integration.negotiated = inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
			},
			call: func(registry *Registry) error {
				request := inference.NewInvocationRequest(inference.InvocationInput{
					Required: inference.NewCapabilitySet(inference.CapabilityMessageSnapshots),
				})
				_, err := registry.Capabilities(context.Background(), "cursor", request)
				return err
			},
			want: `omit required capability "message_snapshots"`,
		},
		{
			name: "contradictory discovery",
			mutate: func(integration *recordingIntegration) {
				integration.discovery = inference.NewDiscovery(inference.ReadinessUnavailable)
			},
			call: func(registry *Registry) error {
				_, err := registry.Discover(context.Background(), "cursor")
				return err
			},
			want: `returned invalid discovery`,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			registry, recordings := newRecordingRegistry(t)
			test.mutate(recordings["cursor"])
			err := test.call(registry)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("operation error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestRegistryBoundsProviderOperationErrorsAndPreservesCause(t *testing.T) {
	t.Parallel()

	registry, recordings := newRecordingRegistry(t)
	cause := errors.New(strings.Repeat("native provider detail ", 100))
	recordings["cursor"].discoveryErr = cause
	_, err := registry.Discover(context.Background(), "cursor")
	if !errors.Is(err, cause) {
		t.Fatalf("Discover() error = %v, want wrapped cause", err)
	}
	if got := err.Error(); got != `provider "cursor" discovery failed` {
		t.Fatalf("Discover() error = %q, want bounded provider context", got)
	}
}

func newRecordingRegistry(t *testing.T) (*Registry, map[string]*recordingIntegration) {
	t.Helper()
	recordings := make(map[string]*recordingIntegration)
	registrations := supportedCatalogRegistrations(t)
	for index, registration := range registrations {
		manifest := findManifest(t, publishedCatalog(t), string(registration.identity))
		integration := &recordingIntegration{
			identity:   registration.identity,
			maximum:    manifestCapabilities(manifest),
			negotiated: reverseCapabilities(manifestCapabilities(manifest)),
			discovery: inference.NewDiscovery(
				inference.ReadinessReady,
				inference.NewPrerequisite(
					inference.PrerequisiteDependency,
					"second dependency",
					inference.PrerequisiteSatisfied,
					"second dependency is available",
				),
				inference.NewPrerequisite(
					inference.PrerequisiteConfiguration,
					"first configuration",
					inference.PrerequisiteSatisfied,
					"first configuration is available",
				),
			),
		}
		registrations[index].integration = integration
		recordings[string(integration.identity)] = integration
	}
	registry, err := New(registrations...)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return registry, recordings
}

func reverseCapabilities(capabilities inference.CapabilitySet) inference.CapabilitySet {
	values := capabilities.Values()
	slices.Reverse(values)
	return inference.NewCapabilitySet(values...)
}

func capabilityCall(registry *Registry) error {
	request := inference.NewInvocationRequest(inference.InvocationInput{
		Required: inference.NewCapabilitySet(inference.CapabilityPromptSubmission),
	})
	_, err := registry.Capabilities(context.Background(), "cursor", request)
	return err
}

func entryIdentities(entries []Entry) []string {
	identities := make([]string, 0, len(entries))
	for _, entry := range entries {
		identities = append(identities, string(entry.Identity()))
	}
	return identities
}

func findEntry(t *testing.T, entries []Entry, identity inference.Identity) Entry {
	t.Helper()
	for _, entry := range entries {
		if entry.Identity() == identity {
			return entry
		}
	}
	t.Fatalf("entry %q not found", identity)
	return Entry{}
}

func assertProviderCalls(
	t *testing.T,
	recordings map[string]*recordingIntegration,
	discoveryCalls, capabilityCalls, invocationCalls int,
) {
	t.Helper()
	for identity, recording := range recordings {
		if recording.discoveryCalls != discoveryCalls ||
			recording.capabilityCalls != capabilityCalls ||
			recording.invocationCalls != invocationCalls {
			t.Fatalf(
				"%s calls = discovery %d, capabilities %d, invocation %d; want %d, %d, %d",
				identity,
				recording.discoveryCalls,
				recording.capabilityCalls,
				recording.invocationCalls,
				discoveryCalls,
				capabilityCalls,
				invocationCalls,
			)
		}
	}
}
