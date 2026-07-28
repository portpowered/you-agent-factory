package factorydefinitions

import (
	"io/fs"

	catalognamedpaths "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedpaths"
)

// NamedPathFileSystem is the exact filesystem effect used to resolve and
// persist Current Factory pointers and canonical named Factory paths.
type NamedPathFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

// NamedPathResolver owns canonical named-Factory filesystem resolution.
type NamedPathResolver interface {
	ResolveCandidatePaths(projectRoot, globalRoot, name string) (NamedFactoryCandidatePaths, error)
	ResolveExistingDir(rootDir, name string) (string, error)
	RequireDefinitionDir(factoryDir string) error
	ResolveCurrentDir(rootDir string) (string, error)
	ReadCurrentPointer(rootDir string) (string, error)
	WriteCurrentPointer(rootDir, name string) error
}

// NamedFactoryCandidatePaths contains the detached, ordered paths used to
// diagnose a failed cross-root named Factory lookup. Project remains first in
// the same precedence order used by the named Factory catalog.
type NamedFactoryCandidatePaths = catalognamedpaths.CandidatePaths

// NamedFactoryCandidatePathsResolver is the exact Factory Definitions
// operation supplied to callers that need detached candidate paths. It keeps
// canonical hierarchical path policy inside the owning service.
type NamedFactoryCandidatePathsResolver func(
	projectRoot, globalRoot, name string,
) (NamedFactoryCandidatePaths, error)

type DefinitionDirectoryRequirer func(string) error
