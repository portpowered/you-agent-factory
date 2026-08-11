// Package providerpackages validates repository-owned provider package
// boundaries. A package combines the public provider manifest with the
// provider-local runtime definition; it is intentionally independent from
// runtime construction and performs no process, filesystem-probe, or network
// work beyond reading the supplied repository fs.FS.
package providerpackages

const (
	ProviderRoot = "packages/model-providers/providers"
	HarnessFile  = "harness.yaml"
)

// LaunchPosture describes how a reviewed provider package is supplied.
type LaunchPosture string

const (
	LaunchPostureBundled             LaunchPosture = "bundled"
	LaunchPosturePackageRunner       LaunchPosture = "package_runner"
	LaunchPostureInstalledExecutable LaunchPosture = "installed_executable"
	LaunchPostureCatalogOnly         LaunchPosture = "catalog_only"
)

// ImplementationKind identifies the typed runtime implementation selected by
// a package. The binding is data, not a service locator: the runtime profile
// must already be registered by the Providers composition graph.
type ImplementationKind string

const ImplementationKindACPAgent ImplementationKind = "acp_agent"

// Transport identifies the launch transport owned by a package.
type Transport string

const TransportStdio Transport = "stdio"

// RuntimeProfile is the small registry value used by offline package
// validation. Execution behavior remains in the Providers service.
type RuntimeProfile struct {
	ID string
}

// ImplementationBinding selects one registered typed runtime profile.
type ImplementationBinding struct {
	Kind    ImplementationKind `yaml:"kind"`
	Profile string             `yaml:"profile"`
}

// LaunchDefinition contains shell-free launch data. Command and arguments are
// kept separate so generation never needs to reinterpret a shell command.
type LaunchDefinition struct {
	Posture   LaunchPosture `yaml:"posture"`
	Transport Transport     `yaml:"transport"`
	Command   string        `yaml:"command"`
	Arguments []string      `yaml:"arguments"`
}

// HarnessPackage is the provider-local runtime definition in harness.yaml.
type HarnessPackage struct {
	Implementation *ImplementationBinding `yaml:"implementation"`
	Launch         *LaunchDefinition      `yaml:"launch"`
}

// Package is a validated provider package summary. Manifest is kept as a
// normalized map because the public provider catalog owns its typed contract;
// this package only needs the fields that govern publication posture.
type Package struct {
	Directory string
	ID        string
	Aliases   []string
	Manifest  map[string]any
	Harness   *HarnessPackage
}

// Selectable reports whether the package is allowed to project a runtime
// registration. Catalog-only packages are intentionally discoverable but not
// executable.
func (provider Package) Selectable() bool {
	return provider.Harness != nil && provider.Harness.Launch != nil && provider.Harness.Launch.Posture != LaunchPostureCatalogOnly
}

// DefaultRuntimeProfiles returns the ACP profile identities currently known
// to the Providers runtime. The list is deliberately data-only; ownership and
// runtime projection parity are validated by the later ACP inventory story.
func DefaultRuntimeProfiles() []RuntimeProfile {
	return []RuntimeProfile{
		{ID: "acp-stdio"},
		{ID: "pi-acp"},
		{ID: "openclaw-acp"},
		{ID: "gemini-acp"},
		{ID: "cursor-acp"},
		{ID: "copilot-acp"},
		{ID: "droid-acp"},
		{ID: "fast-agent-acp"},
		{ID: "grok-build-acp"},
		{ID: "iflow-acp"},
		{ID: "kilocode-acp"},
		{ID: "kimi-acp"},
		{ID: "kiro-acp"},
		{ID: "mux-acp"},
		{ID: "opencode-acp"},
		{ID: "pool-acp"},
		{ID: "qoder-acp"},
		{ID: "qwen-acp"},
		{ID: "reasonix-acp"},
		{ID: "trae-acp"},
		{ID: "zeroclaw-acp"},
	}
}
