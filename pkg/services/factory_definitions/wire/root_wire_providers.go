package wire

import (
	"os/exec"

	"github.com/portpowered/infinite-you/pkg/platform/portablefiles"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	internalnamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories"
	compilationloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"
	internalportableconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/portableconfig"
)

// NewNamedFactoryCatalog constructs the persisted named-Factory catalog for
// the Automations-leased root pkg/wire composition surface.
func NewNamedFactoryCatalog(
	paths factorydefinitions.NamedPathResolver,
	fileSystem factorydefinitions.NamedFactoryCatalogFileSystem,
) (factorydefinitions.NamedFactoryCatalog, error) {
	return internalnamedfactories.New(paths, fileSystem)
}

// NewPathRequiredToolChecker binds host tool lookup to Factory Definitions
// loading validation for root pkg/wire composition.
func NewPathRequiredToolChecker(
	lookPath factorydefinitions.RequiredToolPathLookup,
	versionProbe factorydefinitions.RequiredToolVersionProbe,
) (factorydefinitions.RequiredToolChecker, error) {
	return compilationloading.NewPathRequiredToolChecker(lookPath, versionProbe)
}

// NewPortableBundledFilesApplier binds portable authored-file discovery for
// root pkg/wire composition.
func NewPortableBundledFilesApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledFilesApplier, error) {
	return internalportableconfig.NewPortableBundledFilesApplier(fileSystem)
}

// NewFactoryStarterWorkApplier binds starter-Work discovery for root pkg/wire
// composition.
func NewFactoryStarterWorkApplier(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.FactoryStarterWorkApplier, error) {
	return internalportableconfig.NewFactoryStarterWorkApplier(fileSystem)
}

// NewPortableBundledDocsPruner binds obsolete authored-document cleanup for
// root pkg/wire composition.
func NewPortableBundledDocsPruner(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledDocsPruner, error) {
	return internalportableconfig.NewPortableBundledDocsPruner(fileSystem)
}

// NewPortableBundledFilesMaterializer binds portable file writes for root
// pkg/wire composition.
func NewPortableBundledFilesMaterializer(
	fileSystem portablefiles.FileSystem,
) factorydefinitions.PortableBundledFilesMaterializer {
	return internalportableconfig.NewMaterializer(fileSystem)
}

// NewPortableBundledFileWritesValidator binds portable write checks for root
// pkg/wire composition.
func NewPortableBundledFileWritesValidator(
	fileSystem portablefiles.FileSystem,
) factorydefinitions.PortableBundledFileWritesValidator {
	return internalportableconfig.NewWritesValidator(fileSystem)
}

// NewPortableBundledFilesCopier binds portable file copy for root pkg/wire
// composition.
func NewPortableBundledFilesCopier(
	fileSystem portablefiles.FileSystem,
) factorydefinitions.PortableBundledFilesCopier {
	return internalportableconfig.NewFilesCopier(fileSystem)
}

// NewPortableBundledFileSourceResolver binds portable source resolution for
// root pkg/wire composition.
func NewPortableBundledFileSourceResolver(
	fileSystem portablefiles.FileSystem,
) (factorydefinitions.PortableBundledFileSourceResolver, error) {
	return internalportableconfig.NewSupportedSourceResolver(fileSystem)
}

// DefaultRequiredToolPathLookup returns the host exec.LookPath adapter used by
// root pkg/wire when no test edge override is supplied.
func DefaultRequiredToolPathLookup() factorydefinitions.RequiredToolPathLookup {
	return exec.LookPath
}
