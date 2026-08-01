// Package effects contains the private construction ports for Factory
// Definitions. They are grouped by the owning service only to keep the public
// root contract free of filesystem, clock, process, and peer callback
// interfaces; the concrete adapters are still selected by owner Wire.
package effects

import (
	"context"
	"io"
	"io/fs"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

type DefinitionActivationGateway interface {
	RunSessionID() string
	SessionForActivation(string) *factorycontracts.DefinitionSession
	RequireSession(string) (*factorycontracts.DefinitionSession, error)
	SessionFactoryPersistRoot(*factorycontracts.DefinitionSession) string
	NamedFactoryActivationPaths(*factorycontracts.DefinitionSession) (persistRoot, folderPath string)
	SaveNow() time.Time

	WithActivationLock(func() error) error
	RequireIdleRuntimeForSession(context.Context, string) error
	RequireIdleBeforeNamedFactoryActivation(context.Context, string, *factorycontracts.DefinitionSession) error
	ActivateSessionEditableFactory(context.Context, *factorycontracts.DefinitionSession, string, string, string, string, string) error
	SwapPersistedNamedFactoryRuntime(context.Context, string, *factorycontracts.DefinitionSession, string, string, string, string) error
}

type AuthoredLayoutReaderFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

type AuthoredLayoutWriterFileSystem interface {
	AuthoredLayoutReaderFileSystem
	MkdirAll(string, fs.FileMode) error
	ReadDir(string) ([]fs.DirEntry, error)
	RemoveAll(string) error
	WriteFile(string, []byte, fs.FileMode) error
}

type InputInboxSentinelEnsurer interface {
	EnsureInputInboxGitkeep(targetDir, relativePath string) error
}

type FactoryConfigPathSource interface {
	Stat(string) (fs.FileInfo, error)
}

type DecisionEnvelopeService interface {
	UsesDecisionEnvelopeOutcome(*factorycontracts.FactoryWorkstationConfig) bool
	UsesGoalRoutingDecisionEnvelope(*factorycontracts.FactoryWorkstationConfig) bool
	WorkResultFromDecisionEnvelopeJSONOrFailed(string, string, string) workerexecution.WorkResult
	WorkResultFromGoalRoutingDecisionEnvelopeJSONOrFailed(string, string, string) workerexecution.WorkResult
}

type InvocationInterpolationService interface {
	ValidateInvocationInterpolation(*factorycontracts.FactoryConfig, *work.InvocationArguments, FileReader) error
	InterpolateWorkerConfig(factorycontracts.FactoryWorkerConfig, *work.InvocationArguments, FileReader) (factorycontracts.FactoryWorkerConfig, error)
	InterpolateWorkstationConfig(factorycontracts.FactoryWorkstationConfig, *work.InvocationArguments, FileReader) (factorycontracts.FactoryWorkstationConfig, error)
}

type InvocationOutputShapingService interface {
	ShouldFormatInvocationSummary(*factorycontracts.FactoryWorkstationConfig) bool
	SummaryContentFromWorkerOutput(string, string) ([]work.WorkContentPart, error)
	ShouldFormatInvocationResponse(*factorycontracts.FactoryWorkstationConfig) bool
	ResponseContentFromWorkerOutput(string, string) ([]work.WorkContentPart, error)
	ShouldFormatTTSInvocationMetadata(*factorycontracts.FactoryWorkstationConfig) bool
	TTSBackendLabelFromWorker(*factorycontracts.FactoryWorkerConfig) string
	TTSMetadataContentFromWorkerOutput(string, string, string, string) ([]work.WorkContentPart, error)
}

type InvocationWorkTypeService interface {
	DefaultWorkType(*factorycontracts.FactoryConfig) (string, error)
}

type Clock interface {
	Now() time.Time
}

type VersionFileSystem interface {
	Stat(string) (fs.FileInfo, error)
}

type LoadingFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
}

type RequiredToolPathLookup func(string) (string, error)
type RequiredToolVersionProbe func(string, ...string) ([]byte, error)

type NamedFactoryCatalogFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	ReadDir(string) ([]fs.DirEntry, error)
	RemoveAll(string) error
}

type NamedPathFileSystem interface {
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

type NamedPathResolver interface {
	ResolveCandidatePaths(projectRoot, globalRoot, name string) (factorycontracts.NamedFactoryCandidatePaths, error)
	ResolveExistingDir(rootDir, name string) (string, error)
	RequireDefinitionDir(factoryDir string) error
	ResolveCurrentDir(rootDir string) (string, error)
	ReadCurrentPointer(rootDir string) (string, error)
	WriteCurrentPointer(rootDir, name string) error
}

type NamedFactoryCandidatePaths = factorycontracts.NamedFactoryCandidatePaths
type NamedFactoryCandidatePathsResolver func(string, string, string) (NamedFactoryCandidatePaths, error)
type DefinitionDirectoryRequirer func(string) error

type PackagedFactoryInstaller interface {
	EnsurePackagedFactories(context.Context, string, []factorydefinitions.PackagedDefinition) ([]factorydefinitions.PackagedFactoryInstallResult, error)
}
type PackagedFactoryInstallOutcome = factorydefinitions.PackagedFactoryInstallOutcome
type PackagedFactoryInstallResult = factorydefinitions.PackagedFactoryInstallResult
type PackagedFactoryInstallParams = factorydefinitions.PackagedFactoryInstallParams

type PackagedInstallationFileSystem interface {
	Stat(string) (fs.FileInfo, error)
}

type PackagedGoalPromptFileSystem interface {
	ReadFile(string) ([]byte, error)
}

type DirectoryReplacementStore interface {
	Commit(parentDir, targetDir, stagingDir string) (backupDir string, err error)
	Restore(targetDir, backupDir string)
}

type PersistenceFileSystem interface {
	MkdirTemp(string, string) (string, error)
	RemoveAll(string) error
	Rename(string, string) error
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
}

type PortableBundledFileInspection interface {
	Stat(string) (fs.FileInfo, error)
}

type QuorumPolicyService interface {
	IsPackagedQuorumFactory(*factorycontracts.FactoryConfig) bool
	WorkRelations(string, string, string, []QuorumLineageInput) []work.Relation
}

type QuorumLineageInput = factorydefinitions.QuorumLineageInput

type ReplayRuntimeConfig interface {
	factorycontracts.RuntimeConfigLookup
	WorkstationByID(string) (*factorycontracts.FactoryWorkstationConfig, bool)
}

type ReplayRuntimeConfigDecoder func(*factorycontracts.FactorySnapshot) (ReplayRuntimeConfig, error)
type FactorySnapshotJSONDecoder func([]byte) (*factorycontracts.FactorySnapshot, error)
type FactorySnapshotDirectoryLoader func(string) (*factorycontracts.FactorySnapshot, error)

type ScaffoldFileSystem interface {
	Stat(string) (fs.FileInfo, error)
	MkdirAll(string, fs.FileMode) error
	WriteFile(string, []byte, fs.FileMode) error
}

type ScaffoldOutput interface{ io.Writer }

type TTSObservabilityService interface {
	IsPackagedTTSFactory(*factorycontracts.FactoryConfig) bool
	TTSBackendRuntimeLabel() string
	ClassifyTTSInvocationWait(factorycontracts.FactoryWorldState, string, bool) (TTSInvocationWaitOutcome, *TTSInvocationFailure)
	IsTTSModelNotReadyFailure(string) bool
}

type TTSInvocationWaitOutcome = factorydefinitions.TTSInvocationWaitOutcome
type TTSInvocationFailure = factorydefinitions.TTSInvocationFailure

type ValidationOperations interface {
	factorycontracts.Validator
	factorycontracts.DefinitionValidationOperation
	factorycontracts.SubmittedDefinitionValidationOperation
	factorycontracts.EffectiveDefinitionValidationOperation
}

type WorkPropagationPolicyService interface {
	Mode(*factorycontracts.FactoryWorkstationConfig) factorycontracts.WorkPropagationMode
}

type WorkPropagationPolicyFunc func(*factorycontracts.FactoryWorkstationConfig) factorycontracts.WorkPropagationMode

func (resolve WorkPropagationPolicyFunc) Mode(workstation *factorycontracts.FactoryWorkstationConfig) factorycontracts.WorkPropagationMode {
	return resolve(workstation)
}

type WorkstationExecutionPolicyService interface {
	ExecutionTimeout(*factorycontracts.FactoryWorkstationConfig) (time.Duration, error)
}

type FileReader = factorydefinitions.FileReader

// These aliases keep construction-only function contracts in the owning
// private package. The public wire package republishes only the aliases needed
// by the application composition boundary.
type CurrentFactoryPointerReader = factorycontracts.CurrentFactoryPointerReader
type CurrentFactoryPointerWriter = factorycontracts.CurrentFactoryPointerWriter
type Persistence = factorycontracts.Persistence
type FactoryLayoutPayloadMapper = factorycontracts.FactoryLayoutPayloadMapper
type FactoryLayoutPayloadPreparer = factorycontracts.FactoryLayoutPayloadPreparer
type NamedFactoryPersister = factorycontracts.NamedFactoryPersister
type FactoryLayoutReplacer = factorycontracts.FactoryLayoutReplacer
type FactoryLayoutFlattener = factorycontracts.FactoryLayoutFlattener
type FactoryLayoutExpander = factorycontracts.FactoryLayoutExpander
type FactoryConfigJSONDecoder = factorycontracts.FactoryConfigJSONDecoder
type FactoryConfigJSONEncoder = factorycontracts.FactoryConfigJSONEncoder
type CanonicalFactoryJSONLoader = factorycontracts.CanonicalFactoryJSONLoader
type PortableFactoryConfigPreparer = factorycontracts.PortableFactoryConfigPreparer
type FactoryConfigCloner = factorycontracts.FactoryConfigCloner
type PortableBundledFilesApplier = factorycontracts.PortableBundledFilesApplier
type FactoryStarterWorkApplier = factorycontracts.FactoryStarterWorkApplier
type PortableBundledFilesMaterializer = factorycontracts.PortableBundledFilesMaterializer
type PortableBundledFileWritesValidator = factorycontracts.PortableBundledFileWritesValidator
type PortableBundledFilesCopier = factorycontracts.PortableBundledFilesCopier
type PortableBundledDocsPruner = factorycontracts.PortableBundledDocsPruner
type PortableBundledFileSourceResolver = factorycontracts.PortableBundledFileSourceResolver
type FactorySnapshotCapturer = factorycontracts.FactorySnapshotCapturer
type LoadedFactorySnapshotCapturer = factorycontracts.LoadedFactorySnapshotCapturer
type FactorySnapshotObjectMapper = factorycontracts.FactorySnapshotObjectMapper
type LoadedFactoryLoader = factorycontracts.LoadedFactoryLoader
type LoadedFactorySourceFactory = factorycontracts.LoadedFactorySourceFactory
