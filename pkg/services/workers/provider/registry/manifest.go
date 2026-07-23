package registry

// ImplementationAvailability describes how a manifest's implementation is
// supplied. It is publication metadata, not live readiness.
type ImplementationAvailability string

const (
	ImplementationBundled            ImplementationAvailability = "bundled"
	ImplementationExternallySupplied ImplementationAvailability = "externally-supplied"
	ImplementationCatalogOnly        ImplementationAvailability = "catalog-only"
)

// TechnicalSupportLevel is the catalog's maintainer-verified support posture.
type TechnicalSupportLevel string

const (
	SupportProduction   TechnicalSupportLevel = "production"
	SupportExperimental TechnicalSupportLevel = "experimental"
	SupportNotSupported TechnicalSupportLevel = "not-supported"
)

// LocalizedValue is the typed catalog representation of customer-facing copy.
type LocalizedValue struct {
	ID      *string            `json:"id,omitempty"`
	Locales *[]string          `json:"locales,omitempty"`
	Type    string             `json:"type"`
	Value   string             `json:"value"`
	Values  *map[string]string `json:"values,omitempty"`
}

// Deprecation contains coherent lifecycle metadata for a deprecated provider.
type Deprecation struct {
	DeprecatedSince       string         `json:"deprecatedSince"`
	Reason                LocalizedValue `json:"reason"`
	ReplacementProviderID *string        `json:"replacementProviderId,omitempty"`
}

// DiscoveryPrerequisites contains static, credential-free discovery facts.
type DiscoveryPrerequisites struct {
	ConfigurationKeys []string `json:"configurationKeys"`
	EndpointKinds     []string `json:"endpointKinds"`
	ExecutableNames   []string `json:"executableNames"`
}

// DocumentationLink identifies one stable public provider resource.
type DocumentationLink struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// ExecutionCapabilities is the manifest's maximum execution feature set.
type ExecutionCapabilities struct {
	ImageInput       bool `json:"imageInput"`
	PromptSubmission bool `json:"promptSubmission"`
	SessionResume    bool `json:"sessionResume"`
	StructuredOutput bool `json:"structuredOutput"`
	ToolExecution    bool `json:"toolExecution"`
	WorkingDirectory bool `json:"workingDirectory"`
	Worktree         bool `json:"worktree"`
}

// ResponseFidelityCapabilities is the manifest's maximum response fidelity.
type ResponseFidelityCapabilities struct {
	FileChanges        bool `json:"fileChanges"`
	MessageDeltas      bool `json:"messageDeltas"`
	MessageSnapshots   bool `json:"messageSnapshots"`
	NativeStreaming    bool `json:"nativeStreaming"`
	Plans              bool `json:"plans"`
	ProviderReconnect  bool `json:"providerReconnect"`
	ReasoningSummaries bool `json:"reasoningSummaries"`
	StableItemIDs      bool `json:"stableItemIds"`
	ToolLifecycle      bool `json:"toolLifecycle"`
	ToolOutputDeltas   bool `json:"toolOutputDeltas"`
	Usage              bool `json:"usage"`
}

// Manifest is the registry-owned typed representation of the published
// Provider Manifest contract.
type Manifest struct {
	Aliases                             []string                     `json:"aliases"`
	Deprecation                         *Deprecation                 `json:"deprecation,omitempty"`
	Description                         LocalizedValue               `json:"description"`
	Discovery                           DiscoveryPrerequisites       `json:"discovery"`
	DisplayName                         LocalizedValue               `json:"displayName"`
	Documentation                       []DocumentationLink          `json:"documentation"`
	ID                                  string                       `json:"id"`
	ImplementationAvailability          ImplementationAvailability   `json:"implementationAvailability"`
	MaximumExecutionCapabilities        ExecutionCapabilities        `json:"maximumExecutionCapabilities"`
	MaximumResponseFidelityCapabilities ResponseFidelityCapabilities `json:"maximumResponseFidelityCapabilities"`
	TechnicalSupportLevel               TechnicalSupportLevel        `json:"technicalSupportLevel"`
}

type catalogDocument struct {
	Providers []Manifest `json:"providers"`
}
