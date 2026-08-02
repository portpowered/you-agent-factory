package loading

import (
	"io/fs"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	authoringlayoutcontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/authoring_layout/contracts"
)

// FileSystem is the narrow inspection capability needed by the compilation
// loader. File contents are read by the authored-layout source loader.
type FileSystem interface {
	Stat(string) (fs.FileInfo, error)
}

type AuthoredFactorySourceLoader = authoringlayoutcontracts.FactorySourceLoader
type CurrentFactoryDirectoryResolver func(string) (string, error)
type LoadedFactorySourceFactory func(
	string,
	*factorydefinitions.FactoryConfig,
	factorydefinitions.RuntimeDefinitionLookup,
	[]factorydefinitions.PortableBundledFileReplacement,
) (factorydefinitions.MutableLoadedFactorySource, error)

type FactoryConfigDecoder func([]byte) (*factorydefinitions.FactoryConfig, error)
type FactoryConfigEncoder func(*factorydefinitions.FactoryConfig) ([]byte, error)
type AuthoredFactoryNormalizer func(*factorydefinitions.FactoryConfig) (*factorydefinitions.FactoryConfig, error)
type CanonicalFactoryNormalizer func([]byte) (*factorydefinitions.FactoryConfig, error)
type ManifestValidator func(string, *factorydefinitions.FactoryConfig) error
type BlockingLoadValidator func(*factorydefinitions.FactoryConfig) factorydefinitions.ValidationResult
type PortableBundledFilesApplier func(string, *factorydefinitions.FactoryConfig, bool, bool) error
type FactoryStarterWorkApplier func(string, *factorydefinitions.FactoryConfig) error
type PortableBundledFilesMaterializer func(
	string,
	*factorydefinitions.FactoryConfig,
) ([]factorydefinitions.PortableBundledFileReplacement, error)
type PortableBundledFileWritesValidator func(string, *factorydefinitions.FactoryConfig) error
type WorkerLoader func(string) (*factorydefinitions.FactoryWorkerConfig, error)
type WorkstationLoaderFunc func(string) (*factorydefinitions.FactoryWorkstationConfig, error)
type WorkerBodyLoader func(string) (string, bool, error)
type WorkstationBodyLoader func(string) (string, bool, error)
type WorkstationPromptLoader func(string, string) (string, error)
type LayoutSegmentResolver func(string, string) (string, error)
type RuntimeEntityExists func(string) bool

type RequiredToolPathLookup func(string) (string, error)
type RequiredToolVersionProbe func(string, ...string) ([]byte, error)
type RequiredToolChecker interface {
	Check(factorydefinitions.RequiredToolConfig) factorydefinitions.RequiredToolCheckResult
}
