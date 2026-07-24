package conductor_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/conductor"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

func TestConductorRejectsCapabilityEscalationBeforeProviderIO(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	subject := conductor.New(providers)
	required := inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilityNativeStreaming,
	)
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-escalation",
		Required:     required,
	})

	for _, test := range []struct {
		name string
		call func() error
	}{
		{
			name: "discover",
			call: func() error {
				_, err := subject.Discover(context.Background(), "conductor.fixture", required)
				return err
			},
		},
		{
			name: "capabilities",
			call: func() error {
				_, err := subject.Capabilities(context.Background(), "conductor.fixture", request)
				return err
			},
		},
		{
			name: "invoke",
			call: func() error {
				return subject.Invoke(context.Background(), "conductor.fixture", request, &recordingWriter{})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			assertEscalationRejection(t, test.call(), inference.CapabilityNativeStreaming)
			assertNoProviderIO(t, recording)
		})
	}
}

func TestConductorAcceptedCapabilitySubsetProceedsToProviderIO(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	subject := conductor.New(providers)
	required := inference.NewCapabilitySet(inference.CapabilityPromptSubmission)
	request := inference.NewInvocationRequest(inference.InvocationInput{
		InvocationID: "inv-accepted",
		Required:     required,
	})

	discovery, err := subject.Discover(context.Background(), "conductor.fixture", required)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if discovery.Readiness() != inference.ReadinessReady {
		t.Fatalf("Discover() readiness = %q, want ready", discovery.Readiness())
	}
	if recording.discoveryCalls != 1 {
		t.Fatalf("discoveryCalls = %d, want 1", recording.discoveryCalls)
	}

	capabilities, err := subject.Capabilities(context.Background(), "conductor.fixture", request)
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if !capabilities.Has(inference.CapabilityPromptSubmission) {
		t.Fatalf("Capabilities() = %v, want prompt_submission", capabilities.Values())
	}
	if recording.capabilityCalls != 1 {
		t.Fatalf("capabilityCalls = %d, want 1", recording.capabilityCalls)
	}

	destination := &recordingWriter{}
	if err := subject.Invoke(context.Background(), "conductor.fixture", request, destination); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if recording.invocationCalls != 1 {
		t.Fatalf("invocationCalls = %d, want 1", recording.invocationCalls)
	}
	if destination.closed != 1 || destination.failure == nil {
		t.Fatalf("destination close = %d failure=%v, want one failure close", destination.closed, destination.failure)
	}
}

func TestConductorEscalationDiagnosticsAreStableAcrossRepeatedRuns(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	subject := conductor.New(providers)
	required := inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilityToolLifecycle,
		inference.CapabilityNativeStreaming,
	)

	var first *conductor.Rejection
	for attempt := 0; attempt < 5; attempt++ {
		_, err := subject.Discover(context.Background(), "conductor.fixture", required)
		var rejection *conductor.Rejection
		if !errors.As(err, &rejection) {
			t.Fatalf("attempt %d: error = %v, want *conductor.Rejection", attempt, err)
		}
		if first == nil {
			first = rejection
			continue
		}
		if rejection.Invariant() != first.Invariant() ||
			rejection.Capability() != first.Capability() ||
			rejection.Error() != first.Error() ||
			!diagnosticsEqual(rejection.Diagnostics(), first.Diagnostics()) {
			t.Fatalf("attempt %d diagnostics drifted: got %#v want %#v", attempt, rejection, first)
		}
	}
	assertNoProviderIO(t, recording)
	if first.Invariant() != conductor.InvariantCapabilityEscalation {
		t.Fatalf("Invariant() = %q, want %q", first.Invariant(), conductor.InvariantCapabilityEscalation)
	}
	if first.Capability() != string(inference.CapabilityNativeStreaming) {
		t.Fatalf("Capability() = %q, want first escalated capability in canonical order", first.Capability())
	}
}

func TestConductorRejectsContradictoryCapabilityDependenciesWithoutProviderIO(t *testing.T) {
	t.Parallel()

	providers, recording := newLimitedCapabilityRegistry(t)
	subject := conductor.New(providers)
	required := inference.NewCapabilitySet(
		inference.CapabilityPromptSubmission,
		inference.CapabilityMessageDeltas,
	)

	_, err := subject.Discover(context.Background(), "conductor.fixture", required)
	var rejection *conductor.Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("Discover() error = %v, want *conductor.Rejection", err)
	}
	if rejection.Invariant() != conductor.InvariantCapabilityDependency {
		t.Fatalf("Invariant() = %q, want %q", rejection.Invariant(), conductor.InvariantCapabilityDependency)
	}
	if rejection.Capability() != string(inference.CapabilityMessageDeltas) {
		t.Fatalf("Capability() = %q, want message_deltas", rejection.Capability())
	}
	if got := rejection.Diagnostics()["requires"]; got != string(inference.CapabilityNativeStreaming) {
		t.Fatalf("diagnostics[requires] = %q, want native_streaming", got)
	}
	assertNoProviderIO(t, recording)
}

type recordingIntegration struct {
	identity        inference.Identity
	maximum         inference.CapabilitySet
	discoveryCalls  int
	capabilityCalls int
	invocationCalls int
}

func (r *recordingIntegration) Identity() inference.Identity { return r.identity }
func (r *recordingIntegration) MaximumCapabilities() inference.CapabilitySet {
	return inference.NewCapabilitySet(r.maximum.Values()...)
}
func (r *recordingIntegration) Discover(context.Context) (inference.Discovery, error) {
	r.discoveryCalls++
	return inference.NewDiscovery(inference.ReadinessReady), nil
}
func (r *recordingIntegration) Capabilities(
	_ context.Context,
	request inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	r.capabilityCalls++
	return request.RequiredCapabilities(), nil
}
func (r *recordingIntegration) Invoke(
	ctx context.Context,
	_ inference.InvocationRequest,
	writer inference.ResponseWriter,
) error {
	r.invocationCalls++
	return writer.Close(ctx, inference.FailedCompletion(inference.NewFailure(inference.FailureInput{
		Kind:    inference.FailureDependency,
		Message: "fixture completed without provider-native work",
	})))
}

type recordingWriter struct {
	events  int
	closed  int
	failure *inference.Failure
}

func (w *recordingWriter) WriteEvent(context.Context, inference.EventDraft) error {
	w.events++
	return nil
}

func (w *recordingWriter) Close(_ context.Context, completion inference.Completion) error {
	w.closed++
	w.failure = completion.Failure()
	return nil
}

func newLimitedCapabilityRegistry(t *testing.T) (*registry.Registry, *recordingIntegration) {
	t.Helper()
	builtIns, err := registry.BuiltInRegistrations()
	if err != nil {
		t.Fatalf("BuiltInRegistrations() error = %v", err)
	}
	manifest := limitedExternalManifest(t, "conductor.fixture", "conductor-fixture")
	recording := &recordingIntegration{
		identity: inference.Identity(manifest.ID),
		maximum: inference.NewCapabilitySet(
			inference.CapabilityPromptSubmission,
		),
	}
	providers, err := registry.New(append(
		builtIns,
		registry.ExternalRegistration(manifest, recording),
	)...)
	if err != nil {
		t.Fatalf("registry.New() error = %v", err)
	}
	return providers, recording
}

func limitedExternalManifest(t *testing.T, identity, alias string) registry.Manifest {
	t.Helper()
	var catalog struct {
		Providers []registry.Manifest `json:"providers"`
	}
	if err := json.Unmarshal(modelproviders.CatalogJSON(), &catalog); err != nil {
		t.Fatalf("decode embedded provider catalog: %v", err)
	}
	manifest := catalog.Providers[0]
	manifest.ID = identity
	manifest.Aliases = []string{alias}
	manifest.ImplementationAvailability = registry.ImplementationExternallySupplied
	manifest.TechnicalSupportLevel = registry.SupportProduction
	manifest.Deprecation = nil
	manifest.MaximumExecutionCapabilities = registry.ExecutionCapabilities{
		PromptSubmission: true,
	}
	manifest.MaximumResponseFidelityCapabilities = registry.ResponseFidelityCapabilities{}
	return manifest
}

func assertEscalationRejection(t *testing.T, err error, capability inference.Capability) {
	t.Helper()
	var rejection *conductor.Rejection
	if !errors.As(err, &rejection) {
		t.Fatalf("error = %v, want *conductor.Rejection", err)
	}
	if rejection.Invariant() != conductor.InvariantCapabilityEscalation {
		t.Fatalf("Invariant() = %q, want %q", rejection.Invariant(), conductor.InvariantCapabilityEscalation)
	}
	if rejection.Capability() != string(capability) {
		t.Fatalf("Capability() = %q, want %q", rejection.Capability(), capability)
	}
	diagnostics := rejection.Diagnostics()
	if diagnostics["invariant"] != conductor.InvariantCapabilityEscalation {
		t.Fatalf("diagnostics[invariant] = %q", diagnostics["invariant"])
	}
	if diagnostics["capability"] != string(capability) {
		t.Fatalf("diagnostics[capability] = %q", diagnostics["capability"])
	}
}

func assertNoProviderIO(t *testing.T, recording *recordingIntegration) {
	t.Helper()
	if recording.discoveryCalls != 0 || recording.capabilityCalls != 0 || recording.invocationCalls != 0 {
		t.Fatalf(
			"provider I/O occurred: discovery=%d capabilities=%d invoke=%d",
			recording.discoveryCalls,
			recording.capabilityCalls,
			recording.invocationCalls,
		)
	}
}

func diagnosticsEqual(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}
