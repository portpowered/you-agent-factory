package testlanes

import "strings"

const (
	FunctionalPackagePattern = "./tests/functional/..."
	functionalPackagePrefix  = ModulePath + "/tests/functional/"
	functionalSupportPackage = functionalPackagePrefix + "internal/support"
)

// IsRunnableFunctionalPackage reports whether a discovered package belongs in
// the maintained functional lanes. Shared composition support is compiled as a
// dependency of scenarios rather than executed as its own functional package.
func IsRunnableFunctionalPackage(importPath string) bool {
	return strings.HasPrefix(importPath, functionalPackagePrefix) && importPath != functionalSupportPackage
}
