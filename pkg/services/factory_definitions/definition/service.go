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
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation"
	compilationwire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/wire"
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
// activation policy. UnimplementedService keeps the remaining CTR-DEF root slice
// methods assignable until nested IMP-DEF collaborators are wired. Compilation
// is already delegated to the private nested compilation subservice.
type Service struct {
	factoryroot.UnimplementedService
	host              Host
	versionFileSystem factoryroot.VersionFileSystem
	compilation       compilation.Service
}

// New constructs a factory-definition read collaborator with explicit dependencies.
func New(host Host, versionFileSystems ...factoryroot.VersionFileSystem) *Service {
	var versionFileSystem factoryroot.VersionFileSystem
	if len(versionFileSystems) > 0 {
		versionFileSystem = versionFileSystems[0]
	}
	return &Service{
		host:              host,
		versionFileSystem: versionFileSystem,
		compilation:       compilationwire.NewService(),
	}
}

// CompileEffectiveFactorySource delegates authored/canonical → effective-source
// compile to the private nested compilation subservice so peers keep calling
// the public Definitions root contract.
func (s *Service) CompileEffectiveFactorySource(
	ctx context.Context,
	request factoryroot.CompileEffectiveFactorySourceRequest,
) (factoryroot.CompileEffectiveFactorySourceResult, error) {
	if s == nil || s.compilation == nil {
		return factoryroot.CompileEffectiveFactorySourceResult{}, factoryroot.ErrInvalidAuthoredFactorySource
	}
	return s.compilation.CompileEffectiveFactorySource(ctx, request)
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
