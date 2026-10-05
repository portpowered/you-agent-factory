package testlanes

import "strings"

const (
	FunctionalPackagePattern = "./tests/functional/..."
	functionalPackagePrefix  = ModulePath + "/tests/functional/"
	functionalInternalPrefix = functionalPackagePrefix + "internal/"
)

// IsRunnableFunctionalPackage reports whether a discovered package belongs in
// the maintained functional lanes. Fixture support is compiled as a dependency
// of customer scenarios; its own tests belong to the maintenance lane.
func IsRunnableFunctionalPackage(importPath string) bool {
	if strings.HasPrefix(importPath, functionalPackagePrefix) {
		return !strings.HasPrefix(importPath, functionalInternalPrefix) && importPath != functionalPackagePrefix+"internal"
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
