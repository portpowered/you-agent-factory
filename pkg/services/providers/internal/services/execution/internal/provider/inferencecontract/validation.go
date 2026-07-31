package inferencecontract

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	maxPrerequisiteCount     = 32
	maxPrerequisiteName      = 80
	maxPrerequisiteDetail    = 256
	maxFailureMessage        = 512
	maxFailureDiagnostics    = 16
	maxDiagnosticKeyLength   = 64
	maxDiagnosticValueLength = 256
)

// ValidationError reports one provider-neutral contract field that is invalid.
type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("invalid provider integration %s: %s", e.Field, e.Message)
}

func invalid(field, message string) error {
	return &ValidationError{Field: field, Message: message}
}

// ValidateIdentity checks that an opaque identity uses its canonical form.
func ValidateIdentity(identity Identity) error {
	if err := workers.ProviderIdentity(identity).Validate(); err != nil {
		return invalid("identity", err.Error())
	}
	return nil
}

// ValidateMaximumCapabilities checks declared capabilities and their required
// dependencies before an integration is used.
func ValidateMaximumCapabilities(capabilities CapabilitySet) error {
	if !capabilities.Has(CapabilityPromptSubmission) {
		return invalid("maximumCapabilities", "must include prompt_submission")
	}
	return validateCapabilitySet("maximumCapabilities", capabilities)
}

// ValidateNegotiatedCapabilities checks request-time capabilities against the
// integration's declared maximum.
func ValidateNegotiatedCapabilities(maximum, negotiated CapabilitySet) error {
	if err := ValidateMaximumCapabilities(maximum); err != nil {
		return err
	}
	if err := validateCapabilitySet("capabilities", negotiated); err != nil {
		return err
	}
	for _, capability := range negotiated.Values() {
		if !maximum.Has(capability) {
			return invalid("capabilities", fmt.Sprintf("%q exceeds the declared maximum", capability))
		}
	}
	return nil
}

func validateCapabilitySet(field string, capabilities CapabilitySet) error {
	for _, capability := range capabilities.Values() {
		if !knownCapability(capability) {
			return invalid(field, fmt.Sprintf("contains unknown capability %q", capability))
		}
	}
	dependencies := []struct {
		capability Capability
		required   []Capability
	}{
		{CapabilityMessageDeltas, []Capability{CapabilityNativeStreaming}},
		{CapabilityToolOutputDeltas, []Capability{CapabilityToolLifecycle, CapabilityNativeStreaming}},
		{CapabilityProviderReconnect, []Capability{CapabilitySessionResume}},
	}
	for _, dependency := range dependencies {
		if !capabilities.Has(dependency.capability) {
			continue
		}
		for _, required := range dependency.required {
			if !capabilities.Has(required) {
				return invalid(field, fmt.Sprintf("%q requires %q", dependency.capability, required))
			}
		}
	}
	return nil
}

func knownCapability(capability Capability) bool {
	switch capability {
	case CapabilityPromptSubmission, CapabilityImageInput, CapabilitySessionResume,
		CapabilityStructuredOutput, CapabilityNativeStreaming, CapabilityMessageDeltas,
		CapabilityMessageSnapshots, CapabilityReasoningSummaries, CapabilityToolLifecycle,
		CapabilityToolOutputDeltas, CapabilityFileChanges, CapabilityPlans, CapabilityUsage,
		CapabilityStableItemIDs, CapabilityProviderReconnect:
		return true
	default:
		return false
	}
}

// ValidateDiscovery checks readiness consistency and ensures that discovery
// guidance is bounded and safe to expose to customers.
func ValidateDiscovery(discovery Discovery) error {
	prerequisites := discovery.Prerequisites()
	if len(prerequisites) > maxPrerequisiteCount {
		return invalid("discovery.prerequisites", fmt.Sprintf("must not contain more than %d entries", maxPrerequisiteCount))
	}
	if err := validatePrerequisites(prerequisites); err != nil {
		return err
	}
	satisfied, missing := prerequisiteOutcomes(prerequisites)
	switch discovery.Readiness() {
	case ReadinessReady:
		if missing > 0 {
			return invalid("discovery.readiness", "ready cannot include missing prerequisites")
		}
	case ReadinessUnavailable:
		if missing == 0 {
			return invalid("discovery.readiness", "unavailable requires a missing prerequisite")
		}
	case ReadinessDegraded:
		if satisfied == 0 || missing == 0 {
			return invalid("discovery.readiness", "degraded requires both satisfied and missing prerequisites")
		}
	default:
		return invalid("discovery.readiness", fmt.Sprintf("contains unknown outcome %q", discovery.Readiness()))
	}
	return nil
}

func validatePrerequisites(prerequisites []Prerequisite) error {
	seen := make(map[string]struct{}, len(prerequisites))
	for index, prerequisite := range prerequisites {
		field := fmt.Sprintf("discovery.prerequisites[%d]", index)
		if !knownPrerequisiteKind(prerequisite.Kind()) {
			return invalid(field+".kind", fmt.Sprintf("contains unknown kind %q", prerequisite.Kind()))
		}
		if prerequisite.Status() != PrerequisiteSatisfied && prerequisite.Status() != PrerequisiteMissing {
			return invalid(field+".status", fmt.Sprintf("contains unknown status %q", prerequisite.Status()))
		}
		if err := validatePublicText(field+".name", prerequisite.Name(), maxPrerequisiteName); err != nil {
			return err
		}
		if err := validatePublicText(field+".description", prerequisite.Description(), maxPrerequisiteDetail); err != nil {
			return err
		}
		key := string(prerequisite.Kind()) + "\x00" + prerequisite.Name()
		if _, exists := seen[key]; exists {
			return invalid(field, "duplicates a prerequisite kind and name")
		}
		seen[key] = struct{}{}
	}
	return nil
}

func knownPrerequisiteKind(kind PrerequisiteKind) bool {
	return kind == PrerequisiteConfiguration || kind == PrerequisiteCredential || kind == PrerequisiteDependency
}

func prerequisiteOutcomes(prerequisites []Prerequisite) (satisfied, missing int) {
	for _, prerequisite := range prerequisites {
		if prerequisite.Status() == PrerequisiteSatisfied {
			satisfied++
		} else if prerequisite.Status() == PrerequisiteMissing {
			missing++
		}
	}
	return satisfied, missing
}

// ValidateFailure checks that a normalized failure has a known category and
// only bounded, customer-safe detail.
func ValidateFailure(failure Failure) error {
	if !knownFailureKind(failure.Kind()) {
		return invalid("failure.kind", fmt.Sprintf("contains unknown category %q", failure.Kind()))
	}
	if err := validatePublicText("failure.message", failure.Message(), maxFailureMessage); err != nil {
		return err
	}
	diagnostics := failure.Diagnostics()
	if len(diagnostics) > maxFailureDiagnostics {
		return invalid("failure.diagnostics", fmt.Sprintf("must not contain more than %d entries", maxFailureDiagnostics))
	}
	for key, value := range diagnostics {
		if len(key) > maxDiagnosticKeyLength || !canonicalIdentifier(key) {
			return invalid("failure.diagnostics", fmt.Sprintf("contains invalid key %q", key))
		}
		if err := validatePublicText("failure.diagnostics."+key, value, maxDiagnosticValueLength); err != nil {
			return err
		}
	}
	return nil
}

func knownFailureKind(kind FailureKind) bool {
	switch kind {
	case FailureAuthentication, FailureInvalidRequest, FailureThrottled, FailureTimeout,
		FailureCanceled, FailureDependency, FailureMalformedOutput, FailureUnknown:
		return true
	default:
		return false
	}
}

func validatePublicText(field, value string, maximum int) error {
	if value == "" || strings.TrimSpace(value) != value {
		return invalid(field, "must be non-empty canonical text without surrounding whitespace")
	}
	if len(value) > maximum {
		return invalid(field, fmt.Sprintf("must not exceed %d bytes", maximum))
	}
	if strings.ContainsAny(value, "\r\n\x00") || hasUnsafeDetail(value) {
		return invalid(field, "must not contain credentials, environment values, machine-local paths, or native payloads")
	}
	return nil
}

func hasUnsafeDetail(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	markers := []string{
		"authorization:", "bearer ", "api_key=", "api_key:", "api-key=", "api-key:",
		"apikey=", "apikey:", "token=", "token:", "secret=", "secret:", "password=",
		"password:", "credential=", "credential:", "prompt:", "-----begin", "sk-", "ghp_",
		"aiza", "ya29.", "/home/", "/users/", `c:\users\`, "$home", "${", "%appdata%",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return strings.ContainsAny(normalized, "{}[]") || hasEnvironmentAssignment(value)
}

func hasEnvironmentAssignment(value string) bool {
	for _, field := range strings.Fields(value) {
		name, _, found := strings.Cut(field, "=")
		if !found || !strings.Contains(name, "_") || name == "" {
			continue
		}
		allEnvironmentName := true
		for _, character := range name {
			if character != '_' && !unicode.IsUpper(character) && !unicode.IsDigit(character) {
				allEnvironmentName = false
				break
			}
		}
		if allEnvironmentName {
			return true
		}
	}
	return false
}

func canonicalIdentifier(value string) bool {
	for index, character := range value {
		if character >= 'a' && character <= 'z' {
			continue
		}
		if index > 0 && (character >= '0' && character <= '9' || character == '.' || character == '-') {
			if character != '.' && character != '-' || index+1 < len(value) {
				continue
			}
		}
		return false
	}
	return !strings.Contains(value, "..") && !strings.Contains(value, "--") &&
		!strings.Contains(value, ".-") && !strings.Contains(value, "-.")
}
