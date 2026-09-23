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
	if strings.HasPrefix(importPath, functionalPackagePrefix) {
		return importPath != functionalSupportPackage
	}
	// These two controlled diagnostic fixtures are runnable only when the
	// retained CI workflow selects their exact package paths. They stay outside
	// the ordinary ./tests/functional/... discovery pattern.
	switch importPath {
	case ModulePath + "/cmd/gocoveragecheck/testdata/rawfailure",
		ModulePath + "/cmd/gocoveragecheck/testdata/rawfailurepeer":
		return true
	default:
		return false
	}
}
