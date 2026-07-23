package registry

import inference "github.com/portpowered/infinite-you/pkg/services/workers/provider/inferencecontract"

// ImplementationAvailability describes how a manifest's implementation is
// supplied. It is publication metadata, not live readiness.
type ImplementationAvailability = inference.ImplementationAvailability

const (
	ImplementationBundled            = inference.ImplementationBundled
	ImplementationExternallySupplied = inference.ImplementationExternallySupplied
	ImplementationCatalogOnly        = inference.ImplementationCatalogOnly
)

// TechnicalSupportLevel is the catalog's maintainer-verified support posture.
type TechnicalSupportLevel = inference.TechnicalSupportLevel

const (
	SupportProduction   = inference.SupportProduction
	SupportExperimental = inference.SupportExperimental
	SupportNotSupported = inference.SupportNotSupported
)

// LocalizedValue is the typed catalog representation of customer-facing copy.
type LocalizedValue = inference.LocalizedValue

// Deprecation contains coherent lifecycle metadata for a deprecated provider.
type Deprecation = inference.Deprecation

// DiscoveryPrerequisites contains static, credential-free discovery facts.
type DiscoveryPrerequisites = inference.DiscoveryPrerequisites

// DocumentationLink identifies one stable public provider resource.
type DocumentationLink = inference.DocumentationLink

// ExecutionCapabilities is the manifest's maximum execution feature set.
type ExecutionCapabilities = inference.ExecutionCapabilities

// ResponseFidelityCapabilities is the manifest's maximum response fidelity.
type ResponseFidelityCapabilities = inference.ResponseFidelityCapabilities

// Manifest is the registry-facing alias of the public typed Provider Manifest
// registration contract.
type Manifest = inference.Manifest

type catalogDocument struct {
	Providers []Manifest `json:"providers"`
}
