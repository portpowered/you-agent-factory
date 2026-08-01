package wire

import (
	"context"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

func TestRegistrationContractValuesDetachAndReportCapabilities(t *testing.T) {
	t.Parallel()

	set := NewCapabilitySet(CapabilityPromptSubmission, "custom")
	values := set.Values()
	values[0] = "mutated"
	if !set.Has(CapabilityPromptSubmission) || set.Has("missing") {
		t.Fatalf("CapabilitySet = %#v, want prompt capability and no missing capability", set.Values())
	}
	if set.Values()[0] != CapabilityPromptSubmission {
		t.Fatalf("CapabilitySet.Values() shares internal storage: %v", set.Values())
	}

	response := Response{Content: "detached"}
	completion := SuccessfulCompletion(response)
	if completion.Response == nil || completion.Response.Content != response.Content {
		t.Fatalf("SuccessfulCompletion() = %#v, want detached response", completion)
	}
	response.Content = "mutated"
	if completion.Response.Content == response.Content {
		t.Fatal("SuccessfulCompletion() shares response value state")
	}
}

func TestProgressingExternalIntegrationExercisesProtocolLifecycle(t *testing.T) {
	t.Parallel()

	integration := ProgressingExternalIntegration("sealed", "result")
	if integration.Identity() != "sealed" {
		t.Fatalf("Identity() = %q, want sealed", integration.Identity())
	}
	if !integration.MaximumCapabilities().Has(CapabilityPromptSubmission) {
		t.Fatalf("MaximumCapabilities() = %v, want prompt submission", integration.MaximumCapabilities().Values())
	}
	if _, err := integration.Discover(context.Background()); err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if _, err := integration.Capabilities(context.Background(), InvocationRequest{}); err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}

	writer := &recordingResponseWriter{}
	if err := integration.Invoke(context.Background(), InvocationRequest{}, writer); err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if writer.events != 1 || writer.completion == nil || writer.completion.Response == nil || writer.completion.Response.Content != "result" {
		t.Fatalf("response writer = %#v, want one event and result completion", writer)
	}
	stats := integration.Stats()
	want := ProgressingIntegrationStats{DiscoverCalls: 1, CapabilityCalls: 1, InvokeCalls: 1, ProgressWrites: 1, TerminalCloses: 1}
	if !reflect.DeepEqual(stats, want) {
		t.Fatalf("Stats() = %#v, want %#v", stats, want)
	}
}

func TestRegistrationContractManifestCloneDetachesSlicesAndMaps(t *testing.T) {
	t.Parallel()

	values := map[string]string{"en": "Description"}
	manifest := Manifest{
		Aliases:       []string{"alias"},
		Documentation: []DocumentationLink{{Kind: "docs", URL: "https://example.invalid"}},
		Description:   LocalizedValue{Values: &values},
	}
	cloned := cloneManifest(manifest)
	manifest.Aliases[0] = "mutated"
	manifest.Documentation[0].URL = "mutated"
	values["en"] = "mutated"
	if cloned.Aliases[0] == "mutated" || cloned.Documentation[0].URL == "mutated" || (*cloned.Description.Values)["en"] == "mutated" {
		t.Fatalf("cloneManifest() shares mutable state: %#v", cloned)
	}
}

func TestExternalRegistrationAttemptMapsSuccessAndRejectsInvalidRegistration(t *testing.T) {
	t.Parallel()

	descriptor := registrationDescriptor(Manifest{
		ID:          "sealed",
		DisplayName: LocalizedValue{Value: "Sealed"},
		Aliases:     []string{"sealed-alias"},
		MaximumExecutionCapabilities: ExecutionCapabilities{
			PromptSubmission: true, ImageInput: true, SessionResume: true, StructuredOutput: true,
		},
	})
	if descriptor.ID != "sealed" || len(descriptor.Aliases) != 1 || len(descriptor.Capabilities) != 4 {
		t.Fatalf("registrationDescriptor() = %#v, want identity, alias, and four capabilities", descriptor)
	}

	attempt, err := externalRegistrationAttempt(Registration{
		Manifest:    Manifest{ID: "sealed"},
		Integration: ProgressingExternalIntegration("sealed", "attempt result"),
	})
	if err != nil {
		t.Fatalf("externalRegistrationAttempt() error = %v", err)
	}
	result, err := attempt.Attempt(context.Background(), providers.ExecuteRequest{
		Provider: providers.ID("sealed"), AttemptID: "attempt-1", Model: "model-1", UserMessage: "hello",
	})
	if err != nil || result.Content != "attempt result" || result.Diagnostics == nil {
		t.Fatalf("external attempt = (%#v, %v), want result with diagnostics", result, err)
	}

	if _, err := externalRegistrationAttempt(Registration{Manifest: Manifest{ID: "missing"}}); err == nil || !strings.Contains(err.Error(), "integration is required") {
		t.Fatalf("missing integration error = %v, want validation error", err)
	}
	if _, err := externalRegistrationAttempt(Registration{
		Manifest:    Manifest{ID: "manifest"},
		Integration: ProgressingExternalIntegration("different", "ignored"),
	}); err == nil || !strings.Contains(err.Error(), "does not match manifest") {
		t.Fatalf("identity mismatch error = %v, want validation error", err)
	}
}

type recordingResponseWriter struct {
	events     int
	completion *Completion
}

func (writer *recordingResponseWriter) WriteEvent(context.Context, EventDraft) error {
	writer.events++
	return nil
}

func (writer *recordingResponseWriter) Close(_ context.Context, completion Completion) error {
	writer.completion = &completion
	return nil
}

var _ ResponseWriter = (*recordingResponseWriter)(nil)
