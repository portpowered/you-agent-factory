package testlanes

import (
	"fmt"
	"slices"
	"strings"
)

const (
	FunctionalPackagePattern = "./tests/functional/..."
	functionalPackagePrefix  = ModulePath + "/tests/functional/"
	functionalSupportPackage = functionalPackagePrefix + "internal/support"
)

var requiredProviderFunctionalPackages = []string{
	functionalPackagePrefix + "providers/agy",
	functionalPackagePrefix + "providers/claude",
	functionalPackagePrefix + "providers/codex",
	functionalPackagePrefix + "providers/contract",
	functionalPackagePrefix + "providers/cursor",
	functionalPackagePrefix + "providers/gemini",
	functionalPackagePrefix + "providers/kiro",
	functionalPackagePrefix + "providers/mock_workers",
	functionalPackagePrefix + "providers/observability",
	functionalPackagePrefix + "providers/opencode",
	functionalPackagePrefix + "providers/pi",
	functionalPackagePrefix + "providers/script",
}

// IsRunnableFunctionalPackage reports whether a discovered package belongs in
// the maintained functional lanes. Shared composition support is compiled as a
// dependency of scenarios rather than executed as its own functional package.
func IsRunnableFunctionalPackage(importPath string) bool {
	return strings.HasPrefix(importPath, functionalPackagePrefix) && importPath != functionalSupportPackage
}

// RequiredProviderFunctionalPackages returns the provider and provider-domain
// destinations that every maintained functional lane must discover.
func RequiredProviderFunctionalPackages() []string {
	return slices.Clone(requiredProviderFunctionalPackages)
}

// ValidateProviderFunctionalPackages prevents a successful lane from silently
// omitting one of the repository-owned provider destinations.
func ValidateProviderFunctionalPackages(packages []string) error {
	discovered := make(map[string]struct{}, len(packages))
	for _, pkg := range packages {
		discovered[pkg] = struct{}{}
	}

	var missing []string
	for _, required := range requiredProviderFunctionalPackages {
		if _, ok := discovered[required]; !ok {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("required provider functional packages are missing from discovery: %s", strings.Join(missing, ", "))
	}
	return nil
}
