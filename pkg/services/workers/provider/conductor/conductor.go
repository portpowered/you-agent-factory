// Package conductor owns provider-neutral invocation orchestration for
// registry-selected integrations. Capability preflight rejects escalation
// before any Discover, request-sensitive capability negotiation, or Invoke I/O.
package conductor

import (
	"context"
	"fmt"
	"maps"

	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/registry"
)

// Symbolic capability-preflight invariants.
const (
	InvariantUnknownCapability       = "unknown_capability"
	InvariantCapabilityDependency    = "capability_dependency"
	InvariantCapabilityEscalation    = "capability_escalation"
	InvariantMissingPromptSubmission = "missing_prompt_submission"
)

// Conductor is the shared orchestration entry for registry-selected
// integrations. It owns capability preflight before provider I/O and the
// structured response writer/closer that stamps conductor correlation.
type Conductor struct {
	providers *registry.Registry
}

// New constructs a conductor over an authoritative provider registry.
func New(providers *registry.Registry) *Conductor {
	return &Conductor{providers: providers}
}

// Rejection is a deterministic symbolic failure raised before provider I/O.
type Rejection struct {
	invariant   string
	capability  string
	diagnostics map[string]string
	message     string
}

func (r *Rejection) Error() string { return r.message }

// Invariant returns the stable symbolic invariant name.
func (r *Rejection) Invariant() string { return r.invariant }

// Capability returns the offending capability name when applicable.
func (r *Rejection) Capability() string { return r.capability }

// Diagnostics returns a detached map of stable symbolic diagnostic pairs.
func (r *Rejection) Diagnostics() map[string]string {
	return maps.Clone(r.diagnostics)
}

// Discover validates requested capabilities against the selected integration's
// registry/manifest maximum before delegating live readiness discovery.
func (c *Conductor) Discover(
	ctx context.Context,
	identity string,
	required inference.CapabilitySet,
) (inference.Discovery, error) {
	if err := c.preflight(identity, required); err != nil {
		return inference.Discovery{}, err
	}
	return c.providers.Discover(ctx, identity)
}

// Capabilities validates requested capabilities against the selected
// integration's registry/manifest maximum before request-sensitive negotiation.
func (c *Conductor) Capabilities(
	ctx context.Context,
	identity string,
	request inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	if err := c.preflight(identity, request.RequiredCapabilities()); err != nil {
		return inference.CapabilitySet{}, err
	}
	return c.providers.Capabilities(ctx, identity, request)
}

// Invoke validates requested capabilities against the selected integration's
// registry/manifest maximum before any provider Invoke I/O, then invokes
// through the structured response writer composed with ExecuteInvocation.
func (c *Conductor) Invoke(
	ctx context.Context,
	identity string,
	request inference.InvocationRequest,
	destination inference.ResponseWriter,
) error {
	if err := c.preflight(identity, request.RequiredCapabilities()); err != nil {
		return err
	}
	integration, err := c.providers.Integration(identity)
	if err != nil {
		return err
	}
	return inference.ExecuteInvocation(ctx, correlatingIntegration{Integration: integration}, request, destination)
}

func (c *Conductor) preflight(identity string, required inference.CapabilitySet) error {
	if c == nil || c.providers == nil {
		return rejection(InvariantCapabilityEscalation, "", "provider registry is required")
	}
	maximum, err := c.providers.MaximumCapabilities(identity)
	if err != nil {
		return err
	}
	if err := validateRequestedCapabilities(maximum, required); err != nil {
		return err
	}
	return nil
}

func validateRequestedCapabilities(maximum, requested inference.CapabilitySet) error {
	if !maximum.Has(inference.CapabilityPromptSubmission) {
		return rejection(
			InvariantMissingPromptSubmission,
			string(inference.CapabilityPromptSubmission),
			"registry maximum capabilities must include prompt_submission",
		)
	}
	for _, unknown := range requested.Values() {
		if knownCapability(unknown) {
			continue
		}
		return rejection(
			InvariantUnknownCapability,
			string(unknown),
			fmt.Sprintf("requested capability %q is unknown", unknown),
		)
	}
	for _, dependency := range capabilityDependencies() {
		if !requested.Has(dependency.capability) {
			continue
		}
		for _, required := range dependency.requires {
			if requested.Has(required) {
				continue
			}
			return rejectionWithRequires(
				InvariantCapabilityDependency,
				string(dependency.capability),
				string(required),
				fmt.Sprintf("requested capability %q requires %q", dependency.capability, required),
			)
		}
	}
	for _, capability := range canonicalCapabilities() {
		if !requested.Has(capability) {
			continue
		}
		if maximum.Has(capability) {
			continue
		}
		return rejection(
			InvariantCapabilityEscalation,
			string(capability),
			fmt.Sprintf("requested capability %q exceeds registry manifest maximum", capability),
		)
	}
	return nil
}

type capabilityDependency struct {
	capability inference.Capability
	requires   []inference.Capability
}

func capabilityDependencies() []capabilityDependency {
	return []capabilityDependency{
		{inference.CapabilityMessageDeltas, []inference.Capability{inference.CapabilityNativeStreaming}},
		{
			inference.CapabilityToolOutputDeltas,
			[]inference.Capability{inference.CapabilityToolLifecycle, inference.CapabilityNativeStreaming},
		},
		{inference.CapabilityProviderReconnect, []inference.Capability{inference.CapabilitySessionResume}},
	}
}

func canonicalCapabilities() []inference.Capability {
	return []inference.Capability{
		inference.CapabilityPromptSubmission,
		inference.CapabilityImageInput,
		inference.CapabilitySessionResume,
		inference.CapabilityStructuredOutput,
		inference.CapabilityNativeStreaming,
		inference.CapabilityMessageDeltas,
		inference.CapabilityMessageSnapshots,
		inference.CapabilityReasoningSummaries,
		inference.CapabilityToolLifecycle,
		inference.CapabilityToolOutputDeltas,
		inference.CapabilityFileChanges,
		inference.CapabilityPlans,
		inference.CapabilityUsage,
		inference.CapabilityStableItemIDs,
		inference.CapabilityProviderReconnect,
	}
}

func knownCapability(capability inference.Capability) bool {
	for _, known := range canonicalCapabilities() {
		if capability == known {
			return true
		}
	}
	return false
}

func rejection(invariant, capability, message string) *Rejection {
	diagnostics := map[string]string{"invariant": invariant}
	if capability != "" {
		diagnostics["capability"] = capability
	}
	return &Rejection{
		invariant:   invariant,
		capability:  capability,
		diagnostics: diagnostics,
		message:     message,
	}
}

func rejectionWithRequires(invariant, capability, requires, message string) *Rejection {
	result := rejection(invariant, capability, message)
	result.diagnostics["requires"] = requires
	return result
}
