package factorydefinition

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
)

// ErrCurrentFactoryNotFound reports that no durable current-factory pointer
// could be resolved for canonical current-factory reads.
var ErrCurrentFactoryNotFound = interfaces.ErrCurrentFactoryNotFound

// SaveMode selects the Factory-owned persistence and activation policy.
type SaveMode string

const (
	SaveModeReplaceCurrent         SaveMode = "REPLACE_CURRENT"
	SaveModeUpsertNamedAndActivate SaveMode = "UPSERT_NAMED_AND_ACTIVATE"
)

// Service owns current and named factory definition reads, persistence, and
// activation policy.
type Service struct {
	host Host
}

// New constructs a factory-definition read collaborator with explicit dependencies.
func New(host Host) *Service { return &Service{host: host} }

// Save coordinates the session-scoped definition submission pipeline for the
// requested persistence and activation policy.
func (s *Service) Save(ctx context.Context, sessionID string, mode SaveMode, request EditableFactory) (EditableFactory, error) {
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
	name, err := configpersist.ReadCurrentFactoryPointer(rootDir)
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
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return nil, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, s.host.WorkstationLoader())
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
	rootDir := factorysessions.SessionFactoryRootDir(s.host.PersistRootDir(), session)
	factoryName := factorysessions.FactoryName(rootDir, runtimeCfg)
	versionRootDir := rootDir
	if persistRoot := s.host.SessionFactoryPersistRoot(session); persistRoot != "" {
		if pointerName, pointerErr := configpersist.ReadCurrentFactoryPointer(persistRoot); pointerErr == nil {
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

// CurrentFactoryDefinitionVersionAtRoot returns optimistic-concurrency metadata.
func (s *Service) CurrentFactoryDefinitionVersionAtRoot(rootDir, name string) (interfaces.FactoryVersion, error) {
	if s == nil {
		return interfaces.FactoryVersion{}, fmt.Errorf("factory definition service is required")
	}
	return s.currentFactoryDefinitionVersionAtRoot(rootDir, name)
}

// SerializeNamedFactory returns the canonical editable Factory snapshot.
func (s *Service) SerializeNamedFactory(name string, current *factoryconfig.LoadedFactoryConfig, inlineBundledFiles bool) (*interfaces.FactorySnapshot, error) {
	if s == nil {
		return nil, fmt.Errorf("factory definition service is required")
	}
	return s.serializeNamedFactory(name, current, inlineBundledFiles)
}

func sameFactoryDir(left, right string) bool { return factorysessions.SameFactoryDir(left, right) }

// SessionFactoryPersistRoot resolves the on-disk factory root for session-scoped definition persistence.
func SessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return factorysessions.SessionFactoryRootDir(serviceRootDir, session)
}
