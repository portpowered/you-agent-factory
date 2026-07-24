package factorydefinition

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	factoryroot "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	namedfactorypath "github.com/portpowered/infinite-you/pkg/services/factory_definitions/namedpaths"
)

// ErrCurrentFactoryNotFound reports that no durable current-factory pointer
// could be resolved for canonical current-factory reads.
var ErrCurrentFactoryNotFound = interfaces.ErrCurrentFactoryNotFound

const (
	SaveModeReplaceCurrent         = factorydefinitions.SaveModeReplaceCurrent
	SaveModeUpsertNamedAndActivate = factorydefinitions.SaveModeUpsertNamedAndActivate
)

// Service owns current and named factory definition reads, persistence, and
// activation policy.
type Service struct {
	host              Host
	versionFileSystem factoryroot.VersionFileSystem
}

// New constructs a factory-definition read collaborator with explicit dependencies.
func New(host Host, versionFileSystems ...factoryroot.VersionFileSystem) *Service {
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	return &Service{host: host, versionFileSystem: versionFileSystem}
}

// Save coordinates the session-scoped definition submission pipeline for the
// requested persistence and activation policy.
func (s *Service) Save(ctx context.Context, sessionID string, mode factorydefinitions.SaveMode, request EditableFactory) (EditableFactory, error) {
	if s == nil || s.host == nil {
		return EditableFactory{}, fmt.Errorf("factory definition service is required")
	}
	if mode == SaveModeUpsertNamedAndActivate {
		return s.SaveUpsertNamedSnapshotAndActivateForSession(ctx, sessionID, request)
	}
	snapshot, err := s.SaveReplaceCurrentSnapshotForSession(ctx, sessionID, request)
	if err != nil {
		return EditableFactory{}, err
	}
	return EditableFactory{Snapshot: snapshot}, nil
}

// GetCurrentNamedFactory returns the durable current named-factory snapshot
// resolved from the persisted pointer and canonical on-disk layout.
func (s *Service) GetCurrentNamedFactory(context.Context) (*interfaces.FactorySnapshot, error) {
	if s == nil || s.host == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	rootDir := s.host.PersistRootDir()
	name, err := readCurrentFactoryPointerFromHost(s.host, rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := s.host.CurrentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return s.serializeNamedFactory(interfaces.DefaultCurrentFactoryName, currentRuntime, true)
			}
			return nil, ErrCurrentFactoryNotFound
		}
		return nil, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := resolveExistingFactoryDirFromHost(s.host, rootDir, name)
	if err != nil {
		return nil, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	current, err := loadFactoryFromHost(s.host, factoryDir, s.host.WorkstationLoader())
	if err != nil {
		return nil, fmt.Errorf("load current factory %q: %w", name, err)
	}
	return s.serializeNamedFactory(name, current, true)
}

// GetCurrentFactoryForSession returns the editable Factory snapshot and durable
// optimistic-concurrency version for one live session.
func (s *Service) GetCurrentFactoryForSession(_ context.Context, sessionID string) (EditableFactory, error) {
	if s == nil || s.host == nil {
		return EditableFactory{}, fmt.Errorf("factory definition service is required")
	}
	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return EditableFactory{}, err
	}
	rootDir := sessionFactoryRootDir(s.host.PersistRootDir(), session)
	factoryName := factoryName(rootDir, runtimeCfg)
	versionRootDir := rootDir
	if persistRoot := s.host.SessionFactoryPersistRoot(session); persistRoot != "" {
		if pointerName, pointerErr := readCurrentFactoryPointerFromHost(s.host, persistRoot); pointerErr == nil {
			if session.IsDefault || pointerName == factoryName {
				factoryName = pointerName
			}
		}
		if sameFactoryDir(persistRoot, rootDir) {
			versionRootDir = persistRoot
		}
	}
	snapshot, err := s.serializeNamedFactory(factoryName, runtimeCfg, true)
	if err != nil {
		return EditableFactory{}, err
	}
	version, err := s.CurrentFactoryDefinitionVersionAtRoot(versionRootDir, factoryName)
	if err != nil {
		return EditableFactory{}, err
	}
	return EditableFactory{Name: factoryName, Snapshot: snapshot, Version: &version}, nil
}

// CurrentFactorySnapshotForSession reads one editable session definition
// without requiring a Definition service to refer back to itself through its
// host adapter.
func CurrentFactorySnapshotForSession(
	ctx context.Context,
	host Host,
	sessionID string,
) (*interfaces.FactorySnapshot, error) {
	current, err := New(host).GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if current.Snapshot == nil {
		return nil, fmt.Errorf("current factory snapshot is unavailable")
	}
	return current.Snapshot, nil
}

// CurrentFactoryDefinitionVersionAtRoot returns optimistic-concurrency metadata.
func (s *Service) CurrentFactoryDefinitionVersionAtRoot(rootDir, name string) (interfaces.FactoryVersion, error) {
	if s == nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("factory definition service is required")
	}
	return s.currentFactoryDefinitionVersionAtRoot(rootDir, name)
}

// ListNamedFactories satisfies the root catalog slice. Nested catalog wiring
// remains an IMP-DEF concern; this method keeps the root Service assignable.
func (s *Service) ListNamedFactories(
	context.Context,
	factoryroot.ListNamedFactoriesRequest,
) (factoryroot.ListNamedFactoriesResult, error) {
	return factoryroot.ListNamedFactoriesResult{}, fmt.Errorf("named factory catalog collaborator is required")
}

// GetNamedFactory satisfies the root catalog slice.
func (s *Service) GetNamedFactory(
	context.Context,
	factoryroot.GetNamedFactoryRequest,
) (factoryroot.GetNamedFactoryResult, error) {
	return factoryroot.GetNamedFactoryResult{}, factoryroot.ErrNamedFactoryNotFound
}

// ResolveNamedFactory satisfies the root catalog slice.
func (s *Service) ResolveNamedFactory(
	context.Context,
	factoryroot.ResolveNamedFactoryRequest,
) (factoryroot.ResolveNamedFactoryResult, error) {
	return factoryroot.ResolveNamedFactoryResult{}, factoryroot.ErrNamedFactoryNotFound
}

// DeleteNamedFactory satisfies the root catalog slice.
func (s *Service) DeleteNamedFactory(
	context.Context,
	factoryroot.DeleteNamedFactoryRequest,
) (factoryroot.DeleteNamedFactoryResult, error) {
	return factoryroot.DeleteNamedFactoryResult{}, factoryroot.ErrNamedFactoryNotFound
}

// GetCurrentFactoryPointer satisfies the root catalog slice using the host
// pointer reader when available.
func (s *Service) GetCurrentFactoryPointer(
	_ context.Context,
	request factoryroot.GetCurrentFactoryPointerRequest,
) (factoryroot.GetCurrentFactoryPointerResult, error) {
	if s == nil || s.host == nil {
		return factoryroot.GetCurrentFactoryPointerResult{}, fmt.Errorf("factory definition service is required")
	}
	name, err := readCurrentFactoryPointerFromHost(s.host, request.RootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return factoryroot.GetCurrentFactoryPointerResult{}, ErrCurrentFactoryNotFound
		}
		return factoryroot.GetCurrentFactoryPointerResult{}, err
	}
	factoryDir, resolveErr := resolveExistingFactoryDirFromHost(s.host, request.RootDir, name)
	if resolveErr != nil {
		if errors.Is(resolveErr, factoryroot.ErrNamedFactoryNotFound) ||
			errors.Is(resolveErr, factoryroot.ErrInvalidNamedFactoryName) {
			return factoryroot.GetCurrentFactoryPointerResult{}, resolveErr
		}
		return factoryroot.GetCurrentFactoryPointerResult{Name: name}, nil
	}
	return factoryroot.GetCurrentFactoryPointerResult{Name: name, FactoryDir: factoryDir}, nil
}

// SetCurrentFactoryPointer satisfies the root catalog slice using the host
// pointer writer when available.
func (s *Service) SetCurrentFactoryPointer(
	_ context.Context,
	request factoryroot.SetCurrentFactoryPointerRequest,
) (factoryroot.SetCurrentFactoryPointerResult, error) {
	if s == nil || s.host == nil {
		return factoryroot.SetCurrentFactoryPointerResult{}, fmt.Errorf("factory definition service is required")
	}
	if err := writeCurrentFactoryPointerFromHost(s.host, request.RootDir, request.Name); err != nil {
		if errors.Is(err, factoryroot.ErrInvalidNamedFactoryName) ||
			errors.Is(err, factoryroot.ErrNamedFactoryNotFound) {
			return factoryroot.SetCurrentFactoryPointerResult{}, err
		}
		return factoryroot.SetCurrentFactoryPointerResult{}, err
	}
	return factoryroot.SetCurrentFactoryPointerResult{Name: request.Name}, nil
}

// PrepareFactoryLayout satisfies the root authoring slice. Nested layout
// wiring remains an IMP-DEF concern; this method keeps the root Service
// assignable.
func (s *Service) PrepareFactoryLayout(
	context.Context,
	factoryroot.PrepareFactoryLayoutRequest,
) (factoryroot.PrepareFactoryLayoutResult, error) {
	return factoryroot.PrepareFactoryLayoutResult{}, factoryroot.ErrMalformedFactoryLayoutPayload
}

// FlattenFactoryLayout satisfies the root authoring slice.
func (s *Service) FlattenFactoryLayout(
	context.Context,
	factoryroot.FlattenFactoryLayoutRequest,
) (factoryroot.FlattenFactoryLayoutResult, error) {
	return factoryroot.FlattenFactoryLayoutResult{}, fmt.Errorf("factory layout collaborator is required")
}

// ExpandFactoryLayout satisfies the root authoring slice.
func (s *Service) ExpandFactoryLayout(
	context.Context,
	factoryroot.ExpandFactoryLayoutRequest,
) (factoryroot.ExpandFactoryLayoutResult, error) {
	return factoryroot.ExpandFactoryLayoutResult{}, fmt.Errorf("factory layout collaborator is required")
}

// CreateNamedFactory satisfies the root authoring slice.
func (s *Service) CreateNamedFactory(
	context.Context,
	factoryroot.CreateNamedFactoryRequest,
) (factoryroot.CreateNamedFactoryResult, error) {
	return factoryroot.CreateNamedFactoryResult{}, &factoryroot.AtomicFactoryWriteFailure{
		PreviousPreserved: true,
		Cause:             fmt.Errorf("factory layout collaborator is required"),
	}
}

// ReplaceNamedFactory satisfies the root authoring slice.
func (s *Service) ReplaceNamedFactory(
	context.Context,
	factoryroot.ReplaceNamedFactoryRequest,
) (factoryroot.ReplaceNamedFactoryResult, error) {
	return factoryroot.ReplaceNamedFactoryResult{}, &factoryroot.AtomicFactoryWriteFailure{
		PreviousPreserved: true,
		Cause:             fmt.Errorf("factory layout collaborator is required"),
	}
}

// CompileEffectiveFactorySource satisfies the root compile slice. Nested
// loading wiring remains an IMP-DEF concern; this method keeps the root
// Service assignable.
func (s *Service) CompileEffectiveFactorySource(
	context.Context,
	factoryroot.CompileEffectiveFactorySourceRequest,
) (factoryroot.CompileEffectiveFactorySourceResult, error) {
	return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
}

// ValidateStructuralFactoryDefinition satisfies the root validate slice.
// Nested validator wiring remains an IMP-DEF concern; this method keeps the
// root Service assignable.
func (s *Service) ValidateStructuralFactoryDefinition(
	context.Context,
	factoryroot.ValidateStructuralFactoryDefinitionRequest,
) (factoryroot.ValidateStructuralFactoryDefinitionResult, error) {
	return factoryroot.ValidateStructuralFactoryDefinitionResult{}, factoryroot.ErrInvalidFactoryDefinitionPayload
}

// ValidateEffectiveFactoryDefinition satisfies the root validate slice.
func (s *Service) ValidateEffectiveFactoryDefinition(
	context.Context,
	factoryroot.ValidateEffectiveFactoryDefinitionRequest,
) (factoryroot.ValidateEffectiveFactoryDefinitionResult, error) {
	return factoryroot.ValidateEffectiveFactoryDefinitionResult{}, factoryroot.ErrInvalidFactoryDefinitionPayload
}

// SerializeNamedFactory returns the canonical editable Factory snapshot.
func (s *Service) SerializeNamedFactory(name string, current factorydefinitions.LoadedFactorySource, inlineBundledFiles bool) (*interfaces.FactorySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	return s.serializeNamedFactory(name, current, inlineBundledFiles)
}

func sameFactoryDir(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}

func sessionFactoryRootDir(
	serviceRootDir string,
	session *factorydefinitions.DefinitionSession,
) string {
	if session == nil {
		return ""
	}
	rootDir := session.FolderPath
	if session.FolderPath == "" {
		return rootDir
	}
	if session.FactoryDir == "" || !sameFactoryDir(session.FactoryDir, session.FolderPath) {
		return rootDir
	}
	serviceRoot := filepath.Clean(serviceRootDir)
	if serviceRoot != "" && filepath.Dir(session.FactoryDir) == serviceRoot {
		return serviceRoot
	}
	return rootDir
}

func factoryName(
	rootDir string,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
) string {
	if runtimeCfg == nil {
		return interfaces.DefaultCurrentFactoryName
	}
	factoryDir := runtimeCfg.FactoryDir()
	cleanRoot := filepath.Clean(rootDir)
	if sameFactoryDir(factoryDir, cleanRoot) {
		return interfaces.DefaultCurrentFactoryName
	}
	if rootDir != "" && filepath.Dir(factoryDir) == cleanRoot {
		name := filepath.Base(factoryDir)
		if _, err := namedfactorypath.PathSegments(name); err == nil {
			return name
		}
	}
	cfg := runtimeCfg.FactoryConfig()
	if cfg != nil {
		if name := strings.TrimSpace(cfg.Name); name != "" {
			return name
		}
		if project := strings.TrimSpace(cfg.Project); project != "" {
			return project
		}
	}
	return "factory"
}

// SessionFactoryPersistRoot resolves the on-disk factory root for session-scoped definition persistence.
func SessionFactoryPersistRoot(serviceRootDir string, session *factorydefinitions.DefinitionSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return sessionFactoryRootDir(serviceRootDir, session)
}
