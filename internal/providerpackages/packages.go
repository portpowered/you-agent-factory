// Package providerpackages validates repository-owned provider package
// boundaries. A package combines the public provider manifest with the
// provider-local runtime definition; it is intentionally independent from
// runtime construction and performs no process, filesystem-probe, or network
// work beyond reading the supplied repository fs.FS.
package providerpackages

import (
	"sort"
	"strings"
	"unicode"

	"github.com/portpowered/infinite-you/internal/providerprofiles"
)

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

// RuntimeCatalog is the generated, executable subset of the reviewed ACP
// package set. Catalog-only packages intentionally do not appear here.
type RuntimeCatalog struct {
	ACP []RuntimeIntegration `json:"acp"`
}

// RuntimeIntegration is the package-owned runtime projection consumed by the
// Providers composition boundary. Executable and Arguments are authoritative
// shell-free launch values; Command is retained as the legacy representation
// and is checked against both values by the runtime loader.
type RuntimeIntegration struct {
	Name           string                `json:"name"`
	Aliases        []string              `json:"aliases,omitempty"`
	Transport      Transport             `json:"transport"`
	Executable     string                `json:"executable"`
	Command        string                `json:"command"`
	Arguments      []string              `json:"arguments,omitempty"`
	Posture        LaunchPosture         `json:"posture"`
	Implementation RuntimeImplementation `json:"implementation"`
}

// RuntimeImplementation identifies the typed Providers implementation profile
// selected by one package.
type RuntimeImplementation struct {
	Kind    ImplementationKind `json:"kind"`
	Profile string             `json:"profile"`
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

// RuntimeProjection deterministically projects selectable package launch data
// into the generated runtime catalog. It is pure and assumes Validate has
// already established the package invariants.
func RuntimeProjection(packages []Package) RuntimeCatalog {
	result := RuntimeCatalog{ACP: make([]RuntimeIntegration, 0, len(packages))}
	for _, provider := range packages {
		if !provider.Selectable() || provider.Harness.Implementation == nil || provider.Harness.Launch == nil {
			continue
		}
		launch := provider.Harness.Launch
		arguments := append([]string(nil), launch.Arguments...)
		result.ACP = append(result.ACP, RuntimeIntegration{
			Name:       provider.ID,
			Aliases:    append([]string(nil), provider.Aliases...),
			Transport:  launch.Transport,
			Executable: launch.Command,
			Command:    legacyLaunchCommand(launch.Command, arguments),
			Arguments:  arguments,
			Posture:    launch.Posture,
			Implementation: RuntimeImplementation{
				Kind:    provider.Harness.Implementation.Kind,
				Profile: strings.TrimSpace(provider.Harness.Implementation.Profile),
			},
		})
	}
	sort.Slice(result.ACP, func(i, j int) bool { return result.ACP[i].Name < result.ACP[j].Name })
	return result
}

// legacyLaunchCommand preserves the historical command string while keeping
// its executable and explicit argument vector lossless for the runtime loader.
// Simple tokens retain their existing spelling so generated output remains
// stable; tokens with shell-word syntax are quoted only for shellwords.Parse,
// never for shell execution.
func legacyLaunchCommand(executable string, arguments []string) string {
	parts := make([]string, 0, len(arguments)+1)
	parts = append(parts, legacyLaunchToken(executable))
	for _, argument := range arguments {
		parts = append(parts, legacyLaunchToken(argument))
	}
	return strings.Join(parts, " ")
}

func legacyLaunchToken(value string) string {
	if isLegacyShellWord(value) {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func isLegacyShellWord(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		switch r {
		case '.', '/', ':', '@', '%', '+', '=', ',', '-', '_', '~', '^':
		default:
			return false
		}
	}
	return true
}

// RuntimeProfilesFromIDs adapts the runtime owner's registered profile
// identities to the offline validator's small data-only input type.
func RuntimeProfilesFromIDs(ids []string) []RuntimeProfile {
	profiles := make([]RuntimeProfile, 0, len(ids))
	for _, id := range ids {
		profiles = append(profiles, RuntimeProfile{ID: strings.TrimSpace(id)})
	}
	return profiles
}

// DefaultRuntimeProfiles returns the ACP profile identities currently known
// to the Providers runtime. Each registered profile is required to be owned by
// exactly one selectable package during offline validation.
func DefaultRuntimeProfiles() []RuntimeProfile {
	return RuntimeProfilesFromIDs(providerprofiles.RegisteredACPProfileIDs())
}
