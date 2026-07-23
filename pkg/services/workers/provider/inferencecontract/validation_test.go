package inferencecontract_test

import (
	"errors"
	"strings"
	"testing"

	contract "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

func TestValidateIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		identity contract.Identity
		wantErr  bool
	}{
		{name: "dotted", identity: "acme.alpha"},
		{name: "hyphenated", identity: "customer-provider-42"},
		{name: "empty", wantErr: true},
		{name: "uppercase is non-canonical", identity: "Acme", wantErr: true},
		{name: "whitespace", identity: "acme provider", wantErr: true},
		{name: "repeated separator", identity: "acme..provider", wantErr: true},
		{name: "leading digit", identity: "42-provider", wantErr: true},
		{name: "too long", identity: contract.Identity("a" + strings.Repeat("b", 128)), wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			err := contract.ValidateIdentity(test.identity)
			assertValidationOutcome(t, err, test.wantErr)
		})
	}
}

func TestValidateMaximumCapabilities(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		capabilities contract.CapabilitySet
		wantErr      bool
	}{
		{name: "final only", capabilities: contract.NewCapabilitySet(contract.CapabilityPromptSubmission)},
		{name: "streaming deltas", capabilities: contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.CapabilityNativeStreaming, contract.CapabilityMessageDeltas)},
		{name: "missing prompt submission", capabilities: contract.NewCapabilitySet(contract.CapabilityUsage), wantErr: true},
		{name: "delta without streaming", capabilities: contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.CapabilityMessageDeltas), wantErr: true},
		{name: "tool output without lifecycle", capabilities: contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.CapabilityNativeStreaming, contract.CapabilityToolOutputDeltas), wantErr: true},
		{name: "reconnect without resume", capabilities: contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.CapabilityProviderReconnect), wantErr: true},
		{name: "unknown", capabilities: contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.Capability("native_magic")), wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertValidationOutcome(t, contract.ValidateMaximumCapabilities(test.capabilities), test.wantErr)
		})
	}
}

func TestValidateNegotiatedCapabilitiesRejectsEscalation(t *testing.T) {
	t.Parallel()

	maximum := contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.CapabilityMessageSnapshots)
	if err := contract.ValidateNegotiatedCapabilities(maximum, contract.NewCapabilitySet(contract.CapabilityPromptSubmission)); err != nil {
		t.Fatalf("valid subset: %v", err)
	}
	err := contract.ValidateNegotiatedCapabilities(maximum, contract.NewCapabilitySet(contract.CapabilityPromptSubmission, contract.CapabilityNativeStreaming))
	assertValidationOutcome(t, err, true)
}

func TestValidateDiscovery(t *testing.T) {
	t.Parallel()

	satisfied := prerequisite(contract.PrerequisiteSatisfied, "provider configuration is available")
	missing := contract.NewPrerequisite(
		contract.PrerequisiteDependency,
		"provider endpoint",
		contract.PrerequisiteMissing,
		"make the provider endpoint available before use",
	)
	tests := []struct {
		name      string
		discovery contract.Discovery
		wantErr   bool
	}{
		{name: "ready", discovery: contract.NewDiscovery(contract.ReadinessReady, satisfied)},
		{name: "ready without prerequisites", discovery: contract.NewDiscovery(contract.ReadinessReady)},
		{name: "unavailable", discovery: contract.NewDiscovery(contract.ReadinessUnavailable, missing)},
		{name: "degraded", discovery: contract.NewDiscovery(contract.ReadinessDegraded, satisfied, missing)},
		{name: "unknown readiness", discovery: contract.NewDiscovery(contract.Readiness("starting")), wantErr: true},
		{name: "ready contradicts missing", discovery: contract.NewDiscovery(contract.ReadinessReady, missing), wantErr: true},
		{name: "unavailable lacks missing", discovery: contract.NewDiscovery(contract.ReadinessUnavailable, satisfied), wantErr: true},
		{name: "degraded lacks mixed outcomes", discovery: contract.NewDiscovery(contract.ReadinessDegraded, missing), wantErr: true},
		{name: "unknown kind", discovery: contract.NewDiscovery(contract.ReadinessReady, contract.NewPrerequisite("native", "provider", contract.PrerequisiteSatisfied, "available")), wantErr: true},
		{name: "unknown status", discovery: contract.NewDiscovery(contract.ReadinessReady, contract.NewPrerequisite(contract.PrerequisiteDependency, "provider", "checking", "available")), wantErr: true},
		{name: "duplicate prerequisite", discovery: contract.NewDiscovery(contract.ReadinessReady, satisfied, satisfied), wantErr: true},
		{name: "credential detail", discovery: contract.NewDiscovery(contract.ReadinessReady, prerequisite(contract.PrerequisiteSatisfied, "API_KEY=customer-secret")), wantErr: true},
		{name: "machine local path", discovery: contract.NewDiscovery(contract.ReadinessReady, prerequisite(contract.PrerequisiteSatisfied, `configuration loaded from C:\Users\customer\.provider`)), wantErr: true},
		{name: "native payload", discovery: contract.NewDiscovery(contract.ReadinessReady, prerequisite(contract.PrerequisiteSatisfied, `{"status":"ok"}`)), wantErr: true},
		{name: "unbounded detail", discovery: contract.NewDiscovery(contract.ReadinessReady, prerequisite(contract.PrerequisiteSatisfied, strings.Repeat("x", 257))), wantErr: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			assertValidationOutcome(t, contract.ValidateDiscovery(test.discovery), test.wantErr)
		})
	}
}

func TestValidateFailure(t *testing.T) {
	t.Parallel()

	for _, kind := range []contract.FailureKind{
		contract.FailureAuthentication,
		contract.FailureInvalidRequest,
		contract.FailureThrottled,
		contract.FailureTimeout,
		contract.FailureCanceled,
		contract.FailureDependency,
		contract.FailureMalformedOutput,
		contract.FailureUnknown,
	} {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			failure := contract.NewFailure(contract.FailureInput{Kind: kind, Message: "provider request did not complete", Diagnostics: map[string]string{"request.code": "unavailable"}})
			if err := contract.ValidateFailure(failure); err != nil {
				t.Fatalf("ValidateFailure: %v", err)
			}
		})
	}
}

func TestValidateFailureRejectsUnsafeOrUnboundedDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input contract.FailureInput
	}{
		{name: "unknown kind", input: contract.FailureInput{Kind: "native", Message: "failed"}},
		{name: "empty message", input: contract.FailureInput{Kind: contract.FailureUnknown}},
		{name: "long message", input: contract.FailureInput{Kind: contract.FailureUnknown, Message: strings.Repeat("x", 513)}},
		{name: "credential in message", input: contract.FailureInput{Kind: contract.FailureAuthentication, Message: "authorization: Bearer customer-token"}},
		{name: "prompt in native payload", input: contract.FailureInput{Kind: contract.FailureMalformedOutput, Message: `{"prompt":"private input"}`}},
		{name: "raw environment", input: contract.FailureInput{Kind: contract.FailureDependency, Message: "provider failed", Diagnostics: map[string]string{"detail": "OPENAI_API_KEY=private"}}},
		{name: "invalid diagnostic key", input: contract.FailureInput{Kind: contract.FailureUnknown, Message: "provider failed", Diagnostics: map[string]string{"Native Payload": "failure"}}},
		{name: "long diagnostic", input: contract.FailureInput{Kind: contract.FailureUnknown, Message: "provider failed", Diagnostics: map[string]string{"detail": strings.Repeat("x", 257)}}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			failure := contract.NewFailure(test.input)
			assertValidationOutcome(t, contract.ValidateFailure(failure), true)
		})
	}
}

func TestFailureDetachesDiagnostics(t *testing.T) {
	t.Parallel()

	diagnostics := map[string]string{"request.code": "unavailable"}
	failure := contract.NewFailure(contract.FailureInput{
		Kind:        contract.FailureDependency,
		Message:     "provider dependency is unavailable",
		Diagnostics: diagnostics,
	})
	diagnostics["request.code"] = "mutated"
	returned := failure.Diagnostics()
	returned["request.code"] = "mutated again"
	if got := failure.Diagnostics()["request.code"]; got != "unavailable" {
		t.Fatalf("diagnostic = %q, want detached unavailable", got)
	}
}

func prerequisite(status contract.PrerequisiteStatus, detail string) contract.Prerequisite {
	return contract.NewPrerequisite(contract.PrerequisiteConfiguration, "provider configuration", status, detail)
}

func assertValidationOutcome(t *testing.T, err error, wantErr bool) {
	t.Helper()
	if !wantErr && err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if !wantErr {
		return
	}
	if err == nil {
		t.Fatal("expected validation error")
	}
	var validationErr *contract.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Field == "" || validationErr.Message == "" {
		t.Fatalf("error = %#v, want actionable ValidationError", err)
	}
}
