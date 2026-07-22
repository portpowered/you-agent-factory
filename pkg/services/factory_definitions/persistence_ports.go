package factorydefinitions

import (
	"io/fs"
)

import contracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"

type CurrentFactoryPointerReader = contracts.CurrentFactoryPointerReader
type CurrentFactoryPointerWriter = contracts.CurrentFactoryPointerWriter

// DirectoryReplacementStore is the exact filesystem effect required to
// atomically replace a persisted Factory definition directory. Factory
// Definitions owns when replacement and rollback occur; Platform implements
// only the directory mechanics.
type DirectoryReplacementStore interface {
	Commit(parentDir, targetDir, stagingDir string) (backupDir string, err error)
	Restore(targetDir, backupDir string)
}

// PersistenceFileSystem is the exact local filesystem role used to stage,
// publish, inspect, and discard persisted Factory Definition directories.
// Atomic directory replacement remains a separate capability because it has
// rollback semantics beyond individual filesystem calls.
type PersistenceFileSystem interface {
	MkdirTemp(string, string) (string, error)
	RemoveAll(string) error
	Rename(string, string) error
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
}
type FactoryLayoutPayloadMapper = contracts.FactoryLayoutPayloadMapper
type FactoryLayoutPayloadPreparer = contracts.FactoryLayoutPayloadPreparer
type NamedFactoryPersister = contracts.NamedFactoryPersister
type FactoryLayoutReplacer = contracts.FactoryLayoutReplacer
type FactoryLayoutFlattener = contracts.FactoryLayoutFlattener
type FactoryLayoutExpander = contracts.FactoryLayoutExpander
type FactoryConfigJSONDecoder = contracts.FactoryConfigJSONDecoder
type CanonicalFactoryJSONLoader = contracts.CanonicalFactoryJSONLoader
type PortableFactoryConfigPreparer = contracts.PortableFactoryConfigPreparer
type FactoryConfigCloner = contracts.FactoryConfigCloner
type PortableBundledFilesApplier = contracts.PortableBundledFilesApplier
type FactoryStarterWorkApplier = contracts.FactoryStarterWorkApplier
type PortableBundledFilesMaterializer = contracts.PortableBundledFilesMaterializer
type PortableBundledFileWritesValidator = contracts.PortableBundledFileWritesValidator
type PortableBundledFilesCopier = contracts.PortableBundledFilesCopier
type PortableBundledDocsPruner = contracts.PortableBundledDocsPruner
type PortableBundledFileSourceResolver = contracts.PortableBundledFileSourceResolver
type FactorySnapshotCapturer = contracts.FactorySnapshotCapturer
type LoadedFactorySnapshotCapturer = contracts.LoadedFactorySnapshotCapturer
type FactorySnapshotObjectMapper = contracts.FactorySnapshotObjectMapper
