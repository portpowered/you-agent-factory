package registry

import (
	"context"
	"fmt"
	"sort"
	"strings"

	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

// Availability identifies why a catalog entry is or is not selectable.
// Support posture is static catalog metadata and does not imply live readiness.
type Availability string

const (
	AvailabilityCatalogOnly             Availability = "catalog-only"
	AvailabilityNotSupported            Availability = "not-supported"
	AvailabilitySupportedButUnavailable Availability = "supported-but-unavailable"
	AvailabilitySelectable              Availability = "selectable"
)

// Entry is an immutable catalog projection. Mutable manifest values are
// detached when the entry is created and whenever they are returned.
type Entry struct {
	manifest   Manifest
	selectable bool
}

func newEntry(manifest Manifest, selectable bool) Entry {
	return Entry{manifest: canonicalManifest(manifest), selectable: selectable}
}

// Identity returns the canonical manifest identity.
func (e Entry) Identity() inference.Identity {
	return inference.Identity(e.manifest.ID)
}

// Manifest returns a detached manifest projection.
func (e Entry) Manifest() Manifest {
	return cloneManifest(e.manifest)
}

// Aliases returns detached aliases in deterministic normalized order.
func (e Entry) Aliases() []string {
	aliases := make([]string, 0, len(e.manifest.Aliases))
	for _, alias := range e.manifest.Aliases {
		aliases = append(aliases, normalize(alias))
	}
	return sortedUnique(aliases)
}

// DiscoveryPrerequisites returns detached static discovery metadata.
func (e Entry) DiscoveryPrerequisites() DiscoveryPrerequisites {
	return cloneDiscoveryPrerequisites(e.manifest.Discovery)
}

// MaximumCapabilities returns the manifest-authoritative maximum.
func (e Entry) MaximumCapabilities() inference.CapabilitySet {
	return manifestCapabilities(e.manifest)
}

// Selectable reports whether an Integration is bound to the entry.
func (e Entry) Selectable() bool {
	return e.selectable
}

// ProviderDiagnostic is an immutable static provider-status projection.
type ProviderDiagnostic struct {
	entry        Entry
	availability Availability
}

func (d ProviderDiagnostic) Entry() Entry {
	return newEntry(d.entry.manifest, d.entry.selectable)
}

func (d ProviderDiagnostic) Availability() Availability {
	return d.availability
}

// Entries returns every catalog entry in canonical identity order.
func (r *Registry) Entries() []Entry {
	identities := sortedManifestIdentities(r.manifests)
	entries := make([]Entry, 0, len(identities))
	for _, identity := range identities {
		_, selectable := r.integrations[identity]
		entries = append(entries, newEntry(r.manifests[identity], selectable))
	}
	return entries
}

// SupportedProviders returns deterministic diagnostics for all catalog entries.
func (r *Registry) SupportedProviders() []ProviderDiagnostic {
	entries := r.Entries()
	diagnostics := make([]ProviderDiagnostic, 0, len(entries))
	for _, entry := range entries {
		diagnostics = append(diagnostics, ProviderDiagnostic{
			entry:        entry,
			availability: availabilityFor(entry.manifest, entry.selectable),
		})
	}
	return diagnostics
}

// Lookup resolves a canonical identity or alias to a selectable entry.
func (r *Registry) Lookup(identity string) (Entry, error) {
	canonical, err := r.resolveSelectable(identity)
	if err != nil {
		return Entry{}, err
	}
	return newEntry(r.manifests[canonical], true), nil
}

// CanonicalIdentity resolves a selectable canonical ID or alias to its
// canonical string identity without exposing registry-owned values.
func (r *Registry) CanonicalIdentity(identity string) (string, error) {
	entry, err := r.Lookup(identity)
	if err != nil {
		return "", err
	}
	return string(entry.Identity()), nil
}

// MaximumCapabilities resolves a selectable provider without calling it and
// returns the manifest-authoritative maximum capability set.
func (r *Registry) MaximumCapabilities(identity string) (inference.CapabilitySet, error) {
	entry, err := r.Lookup(identity)
	if err != nil {
		return inference.CapabilitySet{}, err
	}
	return entry.MaximumCapabilities(), nil
}

// Capabilities explicitly delegates request-sensitive capability negotiation,
// then rejects provider results that violate the manifest or request.
func (r *Registry) Capabilities(
	ctx context.Context,
	identity string,
	request inference.InvocationRequest,
) (inference.CapabilitySet, error) {
	canonical, err := r.resolveSelectable(identity)
	if err != nil {
		return inference.CapabilitySet{}, err
	}
	maximum := manifestCapabilities(r.manifests[canonical])
	if err := inference.ValidateNegotiatedCapabilities(maximum, request.RequiredCapabilities()); err != nil {
		return inference.CapabilitySet{}, integrationContractError(canonical, "request capabilities", err)
	}
	negotiated, err := r.integrations[canonical].Capabilities(ctx, request)
	if err != nil {
		return inference.CapabilitySet{}, integrationOperationError(canonical, "capability negotiation", err)
	}
	if err := inference.ValidateNegotiatedCapabilities(maximum, negotiated); err != nil {
		return inference.CapabilitySet{}, integrationContractError(canonical, "returned invalid capabilities", err)
	}
	for _, required := range request.RequiredCapabilities().Values() {
		if !negotiated.Has(required) {
			return inference.CapabilitySet{}, fmt.Errorf(
				"provider %q capabilities omit required capability %q",
				canonical,
				required,
			)
		}
	}
	return canonicalCapabilities(negotiated), nil
}

// Discover explicitly delegates live readiness discovery and validates the
// provider-neutral result before returning it.
func (r *Registry) Discover(ctx context.Context, identity string) (inference.Discovery, error) {
	canonical, err := r.resolveSelectable(identity)
	if err != nil {
		return inference.Discovery{}, err
	}
	discovery, err := r.integrations[canonical].Discover(ctx)
	if err != nil {
		return inference.Discovery{}, integrationOperationError(canonical, "discovery", err)
	}
	if err := inference.ValidateDiscovery(discovery); err != nil {
		return inference.Discovery{}, integrationContractError(canonical, "returned invalid discovery", err)
	}
	return canonicalDiscovery(discovery), nil
}

// Integration resolves invocation access without invoking the provider.
func (r *Registry) Integration(identity string) (inference.Integration, error) {
	canonical, err := r.resolveSelectable(identity)
	if err != nil {
		return nil, err
	}
	return r.integrations[canonical], nil
}

func (r *Registry) resolveSelectable(identity string) (string, error) {
	normalized := normalize(identity)
	if err := inference.ValidateIdentity(inference.Identity(normalized)); err != nil {
		return "", fmt.Errorf("provider lookup %s is invalid: %w", identityLabel(normalized), err)
	}
	canonical := normalized
	if aliasTarget, ok := r.aliases[normalized]; ok {
		canonical = aliasTarget
	}
	manifest, known := r.manifests[canonical]
	if !known {
		return "", fmt.Errorf("provider %s is unknown", identityLabel(normalized))
	}
	if _, selectable := r.integrations[canonical]; !selectable {
		return "", fmt.Errorf(
			"provider %s is not selectable (%s)",
			identityLabel(canonical),
			availabilityFor(manifest, false),
		)
	}
	return canonical, nil
}

func sortedManifestIdentities(manifests map[string]Manifest) []string {
	identities := make([]string, 0, len(manifests))
	for identity := range manifests {
		identities = append(identities, identity)
	}
	sort.Strings(identities)
	return identities
}

func availabilityFor(manifest Manifest, selectable bool) Availability {
	switch {
	case manifest.TechnicalSupportLevel == SupportNotSupported:
		return AvailabilityNotSupported
	case manifest.ImplementationAvailability == ImplementationCatalogOnly:
		return AvailabilityCatalogOnly
	case selectable:
		return AvailabilitySelectable
	default:
		return AvailabilitySupportedButUnavailable
	}
}

type operationError struct {
	message string
	cause   error
}

func (e *operationError) Error() string { return e.message }
func (e *operationError) Unwrap() error { return e.cause }

func integrationOperationError(identity, operation string, cause error) error {
	return &operationError{
		message: fmt.Sprintf("provider %q %s failed", identity, operation),
		cause:   cause,
	}
}

func integrationContractError(identity, violation string, cause error) error {
	return &operationError{
		message: fmt.Sprintf("provider %q %s", identity, violation),
		cause:   cause,
	}
}

func canonicalManifest(manifest Manifest) Manifest {
	canonical := cloneManifest(manifest)
	canonical.Aliases = make([]string, 0, len(manifest.Aliases))
	for _, alias := range manifest.Aliases {
		canonical.Aliases = append(canonical.Aliases, normalize(alias))
	}
	canonical.Aliases = sortedUnique(canonical.Aliases)
	canonical.Discovery = cloneDiscoveryPrerequisites(manifest.Discovery)
	sort.Strings(canonical.Discovery.ConfigurationKeys)
	sort.Strings(canonical.Discovery.EndpointKinds)
	sort.Strings(canonical.Discovery.ExecutableNames)
	canonical.Description = canonicalLocalizedValue(canonical.Description)
	canonical.DisplayName = canonicalLocalizedValue(canonical.DisplayName)
	if canonical.Deprecation != nil {
		canonical.Deprecation.Reason = canonicalLocalizedValue(canonical.Deprecation.Reason)
	}
	sort.Slice(canonical.Documentation, func(i, j int) bool {
		left := canonical.Documentation[i].Kind + "\x00" + canonical.Documentation[i].URL
		right := canonical.Documentation[j].Kind + "\x00" + canonical.Documentation[j].URL
		return left < right
	})
	return canonical
}

func canonicalLocalizedValue(value LocalizedValue) LocalizedValue {
	canonical := cloneLocalizedValue(value)
	if canonical.Locales != nil {
		sort.Strings(*canonical.Locales)
	}
	return canonical
}

func canonicalCapabilities(capabilities inference.CapabilitySet) inference.CapabilitySet {
	ordered := make([]inference.Capability, 0, len(capabilities.Values()))
	for _, capability := range allManifestCapabilities() {
		if capabilities.Has(capability) {
			ordered = append(ordered, capability)
		}
	}
	return inference.NewCapabilitySet(ordered...)
}

func canonicalDiscovery(discovery inference.Discovery) inference.Discovery {
	prerequisites := discovery.Prerequisites()
	sort.Slice(prerequisites, func(i, j int) bool {
		left := prerequisiteSortKey(prerequisites[i])
		right := prerequisiteSortKey(prerequisites[j])
		return left < right
	})
	return inference.NewDiscovery(discovery.Readiness(), prerequisites...)
}

func prerequisiteSortKey(prerequisite inference.Prerequisite) string {
	return strings.Join([]string{
		string(prerequisite.Kind()),
		prerequisite.Name(),
		string(prerequisite.Status()),
		prerequisite.Description(),
	}, "\x00")
}

func cloneDiscoveryPrerequisites(prerequisites DiscoveryPrerequisites) DiscoveryPrerequisites {
	return DiscoveryPrerequisites{
		ConfigurationKeys: append([]string{}, prerequisites.ConfigurationKeys...),
		EndpointKinds:     append([]string{}, prerequisites.EndpointKinds...),
		ExecutableNames:   append([]string{}, prerequisites.ExecutableNames...),
	}
}
