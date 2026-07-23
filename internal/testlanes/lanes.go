// Package testlanes owns the repository's primary Go test-lane classification.
package testlanes

import "strings"

const ModulePath = "github.com/portpowered/infinite-you"

type Lane string

const (
	LaneUnit        Lane = "unit"
	LaneMaintenance Lane = "maintenance"
	LaneIntegration Lane = "integration"
	LaneContract    Lane = "contract"
	LaneFunctional  Lane = "functional"
	LaneStress      Lane = "stress"
	LaneRelease     Lane = "release"
)

var specializedPackageSegments = map[string]Lane{
	"contracttests":    LaneContract,
	"exhaustiontests":  LaneMaintenance,
	"functionaltests":  LaneContract,
	"integrationtests": LaneIntegration,
	"paritytests":      LaneContract,
	"runtimetests":     LaneIntegration,
	"servertests":      LaneIntegration,
}

var explicitlyClassifiedPackages = map[string]Lane{
	ModulePath + "/packages/packaged-factories":                               LaneMaintenance,
	ModulePath + "/packages/model-providers":                                  LaneMaintenance,
	ModulePath + "/pkg/services/factory_sessions/internal/execution/fixtures": LaneIntegration,
	ModulePath + "/pkg/transports/cli/baseline":                               LaneFunctional,
	ModulePath + "/pkg/transports/cli/clicontract":                            LaneContract,
	ModulePath + "/pkg/transports/cli/cliinputs":                              LaneFunctional,
	ModulePath + "/pkg/transports/cli/climanifestgen":                         LaneContract,
	ModulePath + "/pkg/transports/cli/commandidentity":                        LaneFunctional,
}

// ForImportPath returns the primary lane for a repository package. Packages
// outside the maintained Go test surfaces return ok=false.
func ForImportPath(importPath string) (lane Lane, ok bool) {
	if lane, found := explicitlyClassifiedPackages[importPath]; found {
		return lane, true
	}
	switch {
	case importPath == ModulePath+"/contracts":
		return LaneContract, true
	case importPath == ModulePath+"/cmd" || strings.HasPrefix(importPath, ModulePath+"/cmd/"):
		return LaneMaintenance, true
	case importPath == ModulePath+"/internal" || strings.HasPrefix(importPath, ModulePath+"/internal/"):
		return LaneMaintenance, true
	case importPath == ModulePath+"/ui" || strings.HasPrefix(importPath, ModulePath+"/ui/"):
		return LaneMaintenance, true
	case importPath == ModulePath+"/tests/functional/internal" || strings.HasPrefix(importPath, ModulePath+"/tests/functional/internal/"):
		return LaneMaintenance, true
	case importPath == ModulePath+"/tests/functional" || strings.HasPrefix(importPath, ModulePath+"/tests/functional/"):
		return LaneFunctional, true
	case importPath == ModulePath+"/tests/stress" || strings.HasPrefix(importPath, ModulePath+"/tests/stress/"):
		return LaneStress, true
	case importPath == ModulePath+"/tests/release" || strings.HasPrefix(importPath, ModulePath+"/tests/release/"):
		return LaneRelease, true
	case importPath == ModulePath+"/pkg" || strings.HasPrefix(importPath, ModulePath+"/pkg/"):
		for _, segment := range strings.Split(strings.TrimPrefix(importPath, ModulePath+"/"), "/") {
			if specializedLane, found := specializedPackageSegments[segment]; found {
				return specializedLane, true
			}
		}
		return LaneUnit, true
	default:
		return "", false
	}
}

func IsUnitPackage(importPath string) bool {
	lane, ok := ForImportPath(importPath)
	return ok && lane == LaneUnit
}
