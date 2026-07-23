// Package registry joins the published provider manifest catalog to the
// provider-neutral inference Integration contract.
package registry

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	modelproviders "github.com/portpowered/infinite-you/packages/model-providers"
	inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"
)

type registrationKind uint8

const (
	catalogRegistration registrationKind = iota + 1
	externalRegistration
)

// Registration binds one provider-neutral Integration either to an existing
// catalog identity or to a typed external manifest. Its fields are private so
// callers cannot repeat or partially override catalog-owned metadata.
type Registration struct {
	kind        registrationKind
	identity    inference.Identity
	manifest    Manifest
	integration inference.Integration
}

// CatalogRegistration binds an Integration to an embedded catalog identity.
func CatalogRegistration(identity inference.Identity, integration inference.Integration) Registration {
	return Registration{
		kind:        catalogRegistration,
		identity:    identity,
		integration: integration,
	}
}

// ExternalRegistration contributes a detached typed manifest and binds it to
// an Integration without mutating the embedded first-party catalog.
func ExternalRegistration(manifest Manifest, integration inference.Integration) Registration {
	return Registration{
		kind:        externalRegistration,
		manifest:    cloneManifest(manifest),
		integration: integration,
	}
}

// Registry is the immutable result of a validated manifest-to-integration join.
// Query operations are added at this boundary rather than exposing its maps.
type Registry struct {
	manifests    map[string]Manifest
	integrations map[string]inference.Integration
	aliases      map[string]string
}

// New parses and validates the embedded catalog, then joins all registrations.
// Construction performs no provider discovery, capability negotiation, or
// invocation.
func New(registrations ...Registration) (*Registry, error) {
	payload := modelproviders.CatalogJSON()
	if err := validateSchema(modelproviders.ProviderCatalogSchemaJSON(), payload); err != nil {
		return nil, validationFailure([]string{"embedded catalog violates its published schema: " + err.Error()})
	}
	var catalog catalogDocument
	if err := json.Unmarshal(payload, &catalog); err != nil {
		return nil, fmt.Errorf("parse embedded provider catalog: %w", err)
	}
	return build(catalog.Providers, registrations)
}

func build(catalog []Manifest, registrations []Registration) (*Registry, error) {
	manifests, violations := collectManifests(catalog, registrations)
	violations = append(violations, validateIdentityClaims(manifests)...)
	byID := indexManifests(manifests)
	integrations, registrationViolations := validateRegistrations(byID, registrations)
	violations = append(violations, registrationViolations...)
	violations = append(violations, validateImplementationCoverage(byID, integrations)...)
	if len(violations) != 0 {
		return nil, validationFailure(violations)
	}
	return assembleRegistry(byID, integrations), nil
}

func collectManifests(catalog []Manifest, registrations []Registration) ([]manifestCandidate, []string) {
	manifests := make([]manifestCandidate, 0, len(catalog)+len(registrations))
	for _, manifest := range catalog {
		manifests = append(manifests, manifestCandidate{manifest: cloneManifest(manifest), bundled: true})
	}

	var violations []string
	for _, registration := range registrations {
		if registration.kind != externalRegistration {
			continue
		}
		manifest := cloneManifest(registration.manifest)
		if err := validateManifestSchema(manifest); err != nil {
			violations = append(violations, identityLabel(manifest.ID)+": external manifest violates the published schema: "+err.Error())
		}
		if manifest.ImplementationAvailability != ImplementationExternallySupplied {
			violations = append(violations, identityLabel(manifest.ID)+": external manifest implementation availability must be externally-supplied")
		}
		manifests = append(manifests, manifestCandidate{manifest: manifest})
	}
	return manifests, violations
}

func indexManifests(manifests []manifestCandidate) map[string][]manifestCandidate {
	byID := make(map[string][]manifestCandidate, len(manifests))
	for _, candidate := range manifests {
		identity := normalize(candidate.manifest.ID)
		byID[identity] = append(byID[identity], candidate)
	}
	return byID
}

func validateRegistrations(byID map[string][]manifestCandidate, registrations []Registration) (map[string][]inference.Integration, []string) {
	integrations := make(map[string][]inference.Integration, len(registrations))
	var violations []string
	for _, registration := range registrations {
		identity, registered, registrationViolations := validateRegistration(byID, registration)
		violations = append(violations, registrationViolations...)
		if registered {
			integrations[identity] = append(integrations[identity], registration.integration)
		}
	}
	return integrations, violations
}

func validateRegistration(byID map[string][]manifestCandidate, registration Registration) (string, bool, []string) {
	identity := registrationIdentity(registration)
	label := identityLabel(identity)
	if registration.kind != catalogRegistration && registration.kind != externalRegistration {
		return identity, false, []string{label + ": registration kind is invalid"}
	}
	var violations []string
	if err := inference.ValidateIdentity(inference.Identity(identity)); err != nil {
		violations = append(violations, label+": manifest binding identity is invalid: "+err.Error())
	}
	if isNilIntegration(registration.integration) {
		return identity, false, append(violations, label+": integration is nil")
	}
	integrationIdentity := normalize(string(registration.integration.Identity()))
	if err := inference.ValidateIdentity(inference.Identity(integrationIdentity)); err != nil {
		violations = append(violations, identityLabel(integrationIdentity)+": integration identity is invalid: "+err.Error())
	}
	if integrationIdentity != identity {
		violations = append(violations, label+": integration identity "+identityLabel(integrationIdentity)+" differs from its manifest binding")
	}
	candidates := byID[identity]
	if len(candidates) != 1 {
		if len(candidates) == 0 {
			violations = append(violations, label+": implementation has no matching manifest")
		}
		return identity, false, violations
	}
	manifest := candidates[0].manifest
	if !selectableManifest(manifest) {
		violations = append(violations, label+": non-selectable manifest cannot have a runnable integration")
	}
	violations = append(violations, validateMaximumCapabilities(identity, manifest, registration.integration)...)
	return identity, true, violations
}

func validateImplementationCoverage(byID map[string][]manifestCandidate, integrations map[string][]inference.Integration) []string {
	var violations []string
	for identity, candidates := range byID {
		if len(candidates) != 1 || !requiresBundledImplementation(candidates[0]) {
			continue
		}
		switch len(integrations[identity]) {
		case 0:
			violations = append(violations, identityLabel(identity)+": supported bundled manifest has no matching implementation")
		case 1:
		default:
			violations = append(violations, identityLabel(identity)+": multiple implementations bind the same manifest")
		}
	}
	return violations
}

func assembleRegistry(byID map[string][]manifestCandidate, integrations map[string][]inference.Integration) *Registry {
	registry := &Registry{
		manifests:    make(map[string]Manifest, len(byID)),
		integrations: make(map[string]inference.Integration, len(integrations)),
		aliases:      make(map[string]string),
	}
	for identity, candidates := range byID {
		manifest := cloneManifest(candidates[0].manifest)
		registry.manifests[identity] = manifest
		for _, alias := range manifest.Aliases {
			registry.aliases[normalize(alias)] = identity
		}
		if registered := integrations[identity]; len(registered) == 1 {
			registry.integrations[identity] = registered[0]
		}
	}
	return registry
}

type manifestCandidate struct {
	manifest Manifest
	bundled  bool
}

func registrationIdentity(registration Registration) string {
	if registration.kind == externalRegistration {
		return normalize(registration.manifest.ID)
	}
	return normalize(string(registration.identity))
}

func normalize(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func identityLabel(identity string) string {
	identity = normalize(identity)
	if identity == "" {
		return `"<empty>"`
	}
	return `"` + identity + `"`
}

func selectableManifest(manifest Manifest) bool {
	return manifest.TechnicalSupportLevel != SupportNotSupported &&
		manifest.ImplementationAvailability != ImplementationCatalogOnly
}

func requiresBundledImplementation(candidate manifestCandidate) bool {
	return candidate.bundled &&
		candidate.manifest.ImplementationAvailability == ImplementationBundled &&
		candidate.manifest.TechnicalSupportLevel != SupportNotSupported
}

func isNilIntegration(integration inference.Integration) bool {
	if integration == nil {
		return true
	}
	value := reflect.ValueOf(integration)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func cloneManifest(manifest Manifest) Manifest {
	clone := manifest
	clone.Aliases = append([]string{}, manifest.Aliases...)
	clone.Description = cloneLocalizedValue(manifest.Description)
	clone.DisplayName = cloneLocalizedValue(manifest.DisplayName)
	clone.Documentation = append([]DocumentationLink{}, manifest.Documentation...)
	clone.Discovery.ConfigurationKeys = append([]string{}, manifest.Discovery.ConfigurationKeys...)
	clone.Discovery.EndpointKinds = append([]string{}, manifest.Discovery.EndpointKinds...)
	clone.Discovery.ExecutableNames = append([]string{}, manifest.Discovery.ExecutableNames...)
	if manifest.Deprecation != nil {
		deprecation := *manifest.Deprecation
		deprecation.Reason = cloneLocalizedValue(manifest.Deprecation.Reason)
		if manifest.Deprecation.ReplacementProviderID != nil {
			replacement := *manifest.Deprecation.ReplacementProviderID
			deprecation.ReplacementProviderID = &replacement
		}
		clone.Deprecation = &deprecation
	}
	return clone
}

func cloneLocalizedValue(value LocalizedValue) LocalizedValue {
	clone := value
	if value.ID != nil {
		id := *value.ID
		clone.ID = &id
	}
	if value.Locales != nil {
		locales := append([]string{}, (*value.Locales)...)
		clone.Locales = &locales
	}
	if value.Values != nil {
		values := make(map[string]string, len(*value.Values))
		for locale, localized := range *value.Values {
			values[locale] = localized
		}
		clone.Values = &values
	}
	return clone
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	return slicesCompact(values)
}

func slicesCompact(values []string) []string {
	if len(values) < 2 {
		return values
	}
	output := values[:1]
	for _, value := range values[1:] {
		if value != output[len(output)-1] {
			output = append(output, value)
		}
	}
	return output
}
