package lifecycle

import (
	"context"
	"fmt"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Host supplies session and runtime collaborators required for definition reads
// and current-factory save orchestration.
type Host interface {
	PersistRootDir() string
	WorkstationLoader() factorydefinitions.WorkstationLoader
	CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource
	WorkflowID() string

	RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error)
	SessionRuntimeConfig(sessionID string) (factorydefinitions.LoadedFactorySource, error)
	SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string
	ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error

	GetCurrentFactorySnapshotForSession(ctx context.Context, sessionID string) (*factorydefinitions.FactorySnapshot, error)
	ReplaceFactoryLayoutAtDir(targetDir string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error)
}

type loadedFactoryHost interface {
	LoadFactory(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error)
}

type currentFactoryPointerHost interface {
	ReadCurrentFactoryPointer(string) (string, error)
}

type namedPathHost interface {
	ResolveExistingFactoryDir(string, string) (string, error)
}

func resolveExistingFactoryDirFromHost(host Host, rootDir, name string) (string, error) {
	paths, ok := host.(namedPathHost)
	if !ok {
		return "", fmt.Errorf("named Factory path resolver is required")
	}
	return paths.ResolveExistingFactoryDir(rootDir, name)
}

type factoryPersistenceHost interface {
	PrepareFactoryLayoutPayload(string, []byte) (*factorydefinitions.PreparedFactoryLayoutPayload, error)
	PersistNamedFactoryWithPrepared(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error)
	WriteCurrentFactoryPointer(string, string) error
}

type factorySnapshotHost interface {
	PreparePortableFactoryConfig(
		string,
		*factorydefinitions.FactoryConfig,
		bool,
	) (*factorydefinitions.FactoryConfig, error)
	CaptureFactorySnapshot(
		string,
		*factorydefinitions.FactoryConfig,
		factorydefinitions.RuntimeDefinitionLookup,
		string,
		map[string]string,
	) (*factorydefinitions.FactorySnapshot, error)
}

func loadFactoryFromHost(
	host Host,
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	loader, ok := host.(loadedFactoryHost)
	if !ok {
		return nil, fmt.Errorf("loaded factory loader is required")
	}
	return loader.LoadFactory(factoryDir, workstationLoader)
}

func readCurrentFactoryPointerFromHost(host Host, rootDir string) (string, error) {
	reader, ok := host.(currentFactoryPointerHost)
	if !ok {
		return "", fmt.Errorf("current factory pointer reader is required")
	}
	return reader.ReadCurrentFactoryPointer(rootDir)
}

func prepareFactoryLayoutPayloadFromHost(
	host Host,
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	persistence, ok := host.(factoryPersistenceHost)
	if !ok {
		return nil, fmt.Errorf("Factory Definition persistence is required")
	}
	return persistence.PrepareFactoryLayoutPayload(segment, payload)
}

func persistNamedFactoryWithPreparedFromHost(
	host Host,
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	persistence, ok := host.(factoryPersistenceHost)
	if !ok {
		return "", fmt.Errorf("Factory Definition persistence is required")
	}
	return persistence.PersistNamedFactoryWithPrepared(rootDir, name, prepared)
}

func writeCurrentFactoryPointerFromHost(host Host, rootDir, name string) error {
	persistence, ok := host.(factoryPersistenceHost)
	if !ok {
		return fmt.Errorf("Factory Definition persistence is required")
	}
	return persistence.WriteCurrentFactoryPointer(rootDir, name)
}

func preparePortableFactoryConfigFromHost(
	host Host,
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	snapshots, ok := host.(factorySnapshotHost)
	if !ok {
		return nil, fmt.Errorf("Factory Definition snapshot host is required")
	}
	return snapshots.PreparePortableFactoryConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
	)
}

func captureFactorySnapshotFromHost(
	host Host,
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	snapshots, ok := host.(factorySnapshotHost)
	if !ok {
		return nil, fmt.Errorf("Factory Definition snapshot host is required")
	}
	return snapshots.CaptureFactorySnapshot(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}

type dependencyHost struct {
	persistRootDir                      func() string
	workstationLoader                   func() factorydefinitions.WorkstationLoader
	loadFactory                         factorydefinitions.LoadedFactoryLoader
	readCurrentFactoryPointer           func(string) (string, error)
	prepareFactoryLayoutPayload         func(string, []byte) (*factorydefinitions.PreparedFactoryLayoutPayload, error)
	persistNamedFactoryWithPrepared     func(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error)
	writeCurrentFactoryPointer          func(string, string) error
	preparePortableFactoryConfig        factorydefinitions.PortableFactoryConfigPreparer
	captureFactorySnapshot              factorydefinitions.FactorySnapshotCapturer
	currentRuntimeConfig                func() factorydefinitions.LoadedFactorySource
	workflowID                          func() string
	resolveExistingFactoryDir           func(string, string) (string, error)
	requireSession                      func(string) (*factorydefinitions.DefinitionSession, error)
	sessionRuntimeConfig                func(string) (factorydefinitions.LoadedFactorySource, error)
	sessionFactoryPersistRoot           func(*factorydefinitions.DefinitionSession) string
	validateEditableFactorySnapshot     func(context.Context, *factorydefinitions.FactorySnapshot) error
	getCurrentFactorySnapshotForSession func(context.Context, string) (*factorydefinitions.FactorySnapshot, error)
	replaceFactoryLayoutAtDir           func(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error)
}

// NewHost adapts flat process callbacks to the canonical Definition Host.
func NewHost(
	persistRootDir func() string,
	workstationLoader func() factorydefinitions.WorkstationLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	readCurrentFactoryPointer func(string) (string, error),
	prepareFactoryLayoutPayload func(string, []byte) (*factorydefinitions.PreparedFactoryLayoutPayload, error),
	persistNamedFactoryWithPrepared func(string, string, *factorydefinitions.PreparedFactoryLayoutPayload) (string, error),
	writeCurrentFactoryPointer func(string, string) error,
	preparePortableFactoryConfig factorydefinitions.PortableFactoryConfigPreparer,
	captureFactorySnapshot factorydefinitions.FactorySnapshotCapturer,
	currentRuntimeConfig func() factorydefinitions.LoadedFactorySource,
	workflowID func() string,
	resolveExistingFactoryDir func(string, string) (string, error),
	requireSession func(string) (*factorydefinitions.DefinitionSession, error),
	sessionRuntimeConfig func(string) (factorydefinitions.LoadedFactorySource, error),
	sessionFactoryPersistRoot func(*factorydefinitions.DefinitionSession) string,
	validateEditableFactorySnapshot func(context.Context, *factorydefinitions.FactorySnapshot) error,
	getCurrentFactorySnapshotForSession func(context.Context, string) (*factorydefinitions.FactorySnapshot, error),
	replaceFactoryLayoutAtDir func(string, *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error),
) (Host, error) {
	return dependencyHost{
		persistRootDir: persistRootDir, workstationLoader: workstationLoader,
		loadFactory: loadFactory, readCurrentFactoryPointer: readCurrentFactoryPointer,
		prepareFactoryLayoutPayload:     prepareFactoryLayoutPayload,
		persistNamedFactoryWithPrepared: persistNamedFactoryWithPrepared,
		writeCurrentFactoryPointer:      writeCurrentFactoryPointer,
		preparePortableFactoryConfig:    preparePortableFactoryConfig,
		captureFactorySnapshot:          captureFactorySnapshot,
		currentRuntimeConfig:            currentRuntimeConfig, workflowID: workflowID,
		resolveExistingFactoryDir: resolveExistingFactoryDir,
		requireSession:            requireSession, sessionRuntimeConfig: sessionRuntimeConfig,
		sessionFactoryPersistRoot:           sessionFactoryPersistRoot,
		validateEditableFactorySnapshot:     validateEditableFactorySnapshot,
		getCurrentFactorySnapshotForSession: getCurrentFactorySnapshotForSession,
		replaceFactoryLayoutAtDir:           replaceFactoryLayoutAtDir,
	}, nil
}

func (h dependencyHost) ResolveExistingFactoryDir(rootDir, name string) (string, error) {
	if h.resolveExistingFactoryDir == nil {
		return "", fmt.Errorf("named Factory path resolver is required")
	}
	return h.resolveExistingFactoryDir(rootDir, name)
}

func (h dependencyHost) PreparePortableFactoryConfig(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	includeInlineContent bool,
) (*factorydefinitions.FactoryConfig, error) {
	if h.preparePortableFactoryConfig == nil {
		return nil, fmt.Errorf("portable Factory config preparer is required")
	}
	return h.preparePortableFactoryConfig(
		factoryDir,
		factoryConfig,
		includeInlineContent,
	)
}

func (h dependencyHost) CaptureFactorySnapshot(
	factoryDir string,
	factoryConfig *factorydefinitions.FactoryConfig,
	runtimeConfig factorydefinitions.RuntimeDefinitionLookup,
	sourceDirectory string,
	metadata map[string]string,
) (*factorydefinitions.FactorySnapshot, error) {
	if h.captureFactorySnapshot == nil {
		return nil, fmt.Errorf("Factory snapshot capturer is required")
	}
	return h.captureFactorySnapshot(
		factoryDir,
		factoryConfig,
		runtimeConfig,
		sourceDirectory,
		metadata,
	)
}

func (h dependencyHost) PrepareFactoryLayoutPayload(
	segment string,
	payload []byte,
) (*factorydefinitions.PreparedFactoryLayoutPayload, error) {
	if h.prepareFactoryLayoutPayload == nil {
		return nil, fmt.Errorf("Factory Definition payload preparer is required")
	}
	return h.prepareFactoryLayoutPayload(segment, payload)
}

func (h dependencyHost) PersistNamedFactoryWithPrepared(
	rootDir string,
	name string,
	prepared *factorydefinitions.PreparedFactoryLayoutPayload,
) (string, error) {
	if h.persistNamedFactoryWithPrepared == nil {
		return "", fmt.Errorf("Factory Definition persistence is required")
	}
	return h.persistNamedFactoryWithPrepared(rootDir, name, prepared)
}

func (h dependencyHost) WriteCurrentFactoryPointer(rootDir, name string) error {
	if h.writeCurrentFactoryPointer == nil {
		return fmt.Errorf("current Factory pointer writer is required")
	}
	return h.writeCurrentFactoryPointer(rootDir, name)
}

func (h dependencyHost) LoadFactory(
	factoryDir string,
	workstationLoader factorydefinitions.WorkstationLoader,
) (factorydefinitions.MutableLoadedFactorySource, error) {
	if h.loadFactory == nil {
		return nil, fmt.Errorf("loaded factory loader is required")
	}
	return h.loadFactory(factoryDir, workstationLoader)
}

func (h dependencyHost) ReadCurrentFactoryPointer(rootDir string) (string, error) {
	if h.readCurrentFactoryPointer == nil {
		return "", fmt.Errorf("current factory pointer reader is required")
	}
	return h.readCurrentFactoryPointer(rootDir)
}

func (h dependencyHost) PersistRootDir() string {
	if h.persistRootDir == nil {
		return ""
	}
	return h.persistRootDir()
}

func (h dependencyHost) WorkstationLoader() factorydefinitions.WorkstationLoader {
	if h.workstationLoader == nil {
		return nil
	}
	return h.workstationLoader()
}

func (h dependencyHost) CurrentRuntimeConfig() factorydefinitions.LoadedFactorySource {
	if h.currentRuntimeConfig == nil {
		return nil
	}
	return h.currentRuntimeConfig()
}

func (h dependencyHost) WorkflowID() string {
	if h.workflowID == nil {
		return ""
	}
	return h.workflowID()
}

func (h dependencyHost) RequireSession(sessionID string) (*factorydefinitions.DefinitionSession, error) {
	if h.requireSession == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.requireSession(sessionID)
}

func (h dependencyHost) SessionRuntimeConfig(sessionID string) (factorydefinitions.LoadedFactorySource, error) {
	if h.sessionRuntimeConfig == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.sessionRuntimeConfig(sessionID)
}

func (h dependencyHost) SessionFactoryPersistRoot(session *factorydefinitions.DefinitionSession) string {
	if h.sessionFactoryPersistRoot == nil {
		return ""
	}
	return h.sessionFactoryPersistRoot(session)
}

func (h dependencyHost) ValidateEditableFactorySnapshot(ctx context.Context, snapshot *factorydefinitions.FactorySnapshot) error {
	if h.validateEditableFactorySnapshot == nil {
		return fmt.Errorf("factory service is required")
	}
	return h.validateEditableFactorySnapshot(ctx, snapshot)
}

func (h dependencyHost) GetCurrentFactorySnapshotForSession(ctx context.Context, sessionID string) (*factorydefinitions.FactorySnapshot, error) {
	if h.getCurrentFactorySnapshotForSession == nil {
		return nil, fmt.Errorf("factory service is required")
	}
	return h.getCurrentFactorySnapshotForSession(ctx, sessionID)
}

func (h dependencyHost) ReplaceFactoryLayoutAtDir(targetDir string, prepared *factorydefinitions.PreparedFactoryLayoutPayload) (*factorydefinitions.FactorySplitLayoutReplaceResult, error) {
	if h.replaceFactoryLayoutAtDir == nil {
		return nil, fmt.Errorf("Factory Definition layout replacer is required")
	}
	return h.replaceFactoryLayoutAtDir(targetDir, prepared)
}

var _ Host = dependencyHost{}
