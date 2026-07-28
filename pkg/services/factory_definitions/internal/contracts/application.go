package factorycontracts

import (
	"context"

	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/validation/authoredmodel/workers"
)

// DefinitionSession is the Factory Definitions projection of a live Factory
// Session. It contains only identity and path values needed by definition
// persistence policy.
type DefinitionSession struct {
	ID         string
	IsDefault  bool
	FolderPath string
	FactoryDir string
}

type SaveMode string

const (
	SaveModeReplaceCurrent         SaveMode = "REPLACE_CURRENT"
	SaveModeUpsertNamedAndActivate SaveMode = "UPSERT_NAMED_AND_ACTIVATE"
)

type EditableFactory struct {
	Name     string
	Snapshot *FactorySnapshot
	Version  *FactoryVersion
}

// NamedFactoryListEntry describes one persisted named Factory under a Factory
// definition root.
type NamedFactoryListEntry struct {
	Name       string `json:"name"`
	FactoryDir string `json:"factoryDirectory"`
	Current    bool   `json:"current"`
}

type NamedFactoryResolutionSource string

const (
	NamedFactoryResolutionSourceProjectLocal NamedFactoryResolutionSource = "project-local"
	NamedFactoryResolutionSourceGlobal       NamedFactoryResolutionSource = "global"
)

type NamedFactoryPrecedenceDecision string

const (
	NamedFactoryPrecedenceDecisionNone              NamedFactoryPrecedenceDecision = "none"
	NamedFactoryPrecedenceDecisionProjectOverGlobal NamedFactoryPrecedenceDecision = "project-local-over-global"
)

type NamedFactoryResolution struct {
	Name               string
	FactoryDir         string
	Source             NamedFactoryResolutionSource
	ProjectRoot        string
	GlobalRoot         string
	PrecedenceDecision NamedFactoryPrecedenceDecision
}

// CurrentFactoryDirectoryResolver resolves the active Factory definition under
// one Factory definition root. Factory Definitions owns named-layout policy;
// transports receive this exact operation from the application injector.
type CurrentFactoryDirectoryResolver func(rootDir string) (string, error)

// NamedFactoryCatalog owns eager discovery and deletion of persisted Factory
// definitions. It is independent of a live Factory Runtime or Factory Session.
type NamedFactoryCatalog interface {
	ListNamedFactories(string) ([]NamedFactoryListEntry, error)
	DeleteNamedFactory(string, string) error
	ResolveNamedFactoryAcrossRoots(string, string, string) (*NamedFactoryResolution, error)
}

// FactoryLayoutPayloadMapper converts one serialized public Factory definition
// into a detached owner request. It performs representation conversion only;
// Factory Definitions invokes validation and interprets its result.
type FactoryLayoutPayloadMapper func([]byte) (DefinitionValidationRequest, error)

// NamedFactoryPersistenceMode selects whether one submitted definition creates
// a new named Factory or replaces an existing one.
type NamedFactoryPersistenceMode string

const (
	NamedFactoryPersistenceModeCreate  NamedFactoryPersistenceMode = "CREATE"
	NamedFactoryPersistenceModeReplace NamedFactoryPersistenceMode = "REPLACE"
)

// NamedFactoryPersistenceRequest is the complete input to the Factory
// Definitions-owned named persistence transaction.
type NamedFactoryPersistenceRequest struct {
	Mode       NamedFactoryPersistenceMode
	RootDir    string
	Name       string
	Payload    []byte
	SetCurrent bool
}

// NamedFactoryPersistenceResult identifies the target even when persistence
// fails after the target path has been resolved.
type NamedFactoryPersistenceResult struct {
	Name       string
	FactoryDir string
}

// NamedFactoryPersistenceOperation is the exact root operation injected into
// transports that accept serialized named Factory definitions.
type NamedFactoryPersistenceOperation func(
	context.Context,
	NamedFactoryPersistenceRequest,
) (NamedFactoryPersistenceResult, error)

// Persistence owns preparation and atomic persistence of Factory definition
// layouts. Callers provide operation values and do not select filesystem or
// serialization implementations.
type Persistence interface {
	PersistNamedFactory(context.Context, NamedFactoryPersistenceRequest) (NamedFactoryPersistenceResult, error)
	PrepareFactoryLayout(context.Context, string, []byte) (*PreparedFactoryLayoutPayload, error)
	ValidateFactoryLayout(string) error
	FlattenFactoryLayout(string) ([]byte, error)
	ExpandFactoryLayout(string) (string, LayoutExpansionReport, error)
	CreateNamedFactory(string, string, *PreparedFactoryLayoutPayload) (string, error)
	ReplaceNamedFactory(string, string, *PreparedFactoryLayoutPayload) (string, error)
	ReplaceFactoryLayout(string, *PreparedFactoryLayoutPayload) (*FactorySplitLayoutReplaceResult, error)
}

type Service interface {
	ActivateNamedFactory(context.Context, string) error
	Save(context.Context, string, SaveMode, EditableFactory) (EditableFactory, error)
	GetCurrentNamedFactory(context.Context) (*FactorySnapshot, error)
	GetCurrentFactoryForSession(context.Context, string) (EditableFactory, error)
	CurrentFactoryDefinitionVersionAtRoot(string, string) (FactoryVersion, error)
}

// SessionHost is the Factory Definitions-owned port for session-scoped
// persistence and activation behavior.
type SessionHost interface {
	PersistRootDir() string
	WorkstationLoader() WorkstationLoader
	CurrentRuntimeConfig() LoadedFactorySource
	WorkflowID() string
	RequireSession(string) (*DefinitionSession, error)
	SessionRuntimeConfig(string) (LoadedFactorySource, error)
	SessionFactoryPersistRoot(*DefinitionSession) string
	ValidateEditableFactorySnapshot(context.Context, *FactorySnapshot) error
	GetCurrentFactorySnapshotForSession(context.Context, string) (*FactorySnapshot, error)
	ReplaceFactoryLayoutAtDir(string, *PreparedFactoryLayoutPayload) (*FactorySplitLayoutReplaceResult, error)
}

type PortableBundledFileReplacement struct {
	TargetPath string
}

type LayoutExpansionReport struct {
	FactoryConfigPaths    int
	WorkerAgentPaths      int
	WorkstationAgentPaths int
	PromptPaths           int
	BundledReplacements   []PortableBundledFileReplacement
}

type FactorySnapshotSource interface {
	RuntimeDefinitionLookup
	FactoryDir() string
	FactoryConfig() *FactoryConfig
}

type LoadedFactorySource interface {
	FactorySnapshotSource
	RuntimeBaseDir() string
}

type MutableLoadedFactorySource interface {
	LoadedFactorySource
	SetRuntimeBaseDir(string)
	PortableBundledFileReplacements() []PortableBundledFileReplacement
	MutateWorkers(func(*workerconfig.Config) error) error
}

type PreparedFactoryLayoutPayload struct {
	Config       *FactoryConfig
	Canonical    []byte
	RootFileName string
}

type FactorySplitLayoutReplaceResult struct {
	Restore       func()
	DiscardBackup func()
}

type CurrentFactoryPointerReader func(string) (string, error)
type CurrentFactoryPointerWriter func(string, string) error

type FactoryLayoutPayloadPreparer func(
	context.Context,
	string,
	[]byte,
	Validator,
) (*PreparedFactoryLayoutPayload, error)

type NamedFactoryPersister func(
	string,
	string,
	*PreparedFactoryLayoutPayload,
) (string, error)

type FactoryLayoutReplacer func(
	string,
	*PreparedFactoryLayoutPayload,
) (*FactorySplitLayoutReplaceResult, error)

type FactoryLayoutFlattener func(string) ([]byte, error)
type FactoryLayoutExpander func(string) (string, LayoutExpansionReport, error)

type FactoryConfigJSONDecoder func([]byte) (*FactoryConfig, error)

type FactoryConfigJSONEncoder func(*FactoryConfig) ([]byte, error)

type CanonicalFactoryJSONLoader func(
	[]byte,
	WorkstationLoader,
) (MutableLoadedFactorySource, error)

type PortableFactoryConfigPreparer func(
	string,
	*FactoryConfig,
	bool,
) (*FactoryConfig, error)

type FactoryConfigCloner func(*FactoryConfig) (*FactoryConfig, error)
type PortableBundledFilesApplier func(string, *FactoryConfig, bool, bool) error
type FactoryStarterWorkApplier func(string, *FactoryConfig) error
type PortableBundledFilesMaterializer func(
	string,
	*FactoryConfig,
) ([]PortableBundledFileReplacement, error)
type PortableBundledFileWritesValidator func(string, *FactoryConfig) error
type PortableBundledFilesCopier func(string, string, *FactoryConfig) error
type PortableBundledDocsPruner func(string, *FactoryConfig) error
type PortableBundledFileSourceResolver func(
	string,
	BundledFileConfig,
) (string, bool)

type FactorySnapshotCapturer func(
	string,
	*FactoryConfig,
	RuntimeDefinitionLookup,
	string,
	map[string]string,
) (*FactorySnapshot, error)

type LoadedFactorySnapshotCapturer func(
	FactorySnapshotSource,
	string,
	map[string]string,
) (*FactorySnapshot, error)

type FactorySnapshotObjectMapper func(*FactoryConfig) (map[string]any, error)

type LoadedFactoryLoader func(
	string,
	WorkstationLoader,
) (MutableLoadedFactorySource, error)

type LoadedFactorySourceFactory func(
	string,
	*FactoryConfig,
	RuntimeDefinitionLookup,
	[]PortableBundledFileReplacement,
) (MutableLoadedFactorySource, error)
