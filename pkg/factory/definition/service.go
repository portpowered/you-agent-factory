package factorydefinition

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	configload "github.com/portpowered/infinite-you/pkg/config/load"
	configpersist "github.com/portpowered/infinite-you/pkg/config/persist"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
)

// ErrCurrentFactoryNotFound reports that no durable current-factory pointer
// could be resolved for canonical current-factory reads.
var ErrCurrentFactoryNotFound = apisurface.ErrCurrentFactoryNotFound

// Service owns current and named factory definition reads, persistence, and
// activation policy.
type Service struct {
	host Host
}

// New constructs a factory-definition read collaborator with explicit dependencies.
func New(host Host) *Service {
	return &Service{host: host}
}

// Save coordinates the session-scoped definition submission pipeline for the
// requested persistence and activation policy.
func (s *Service) Save(
	ctx context.Context,
	sessionID string,
	mode factoryapi.FactorySaveMode,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	if mode == factoryapi.FactorySaveModeUpsertNamedAndActivate {
		return s.SaveUpsertNamedAndActivateForSession(ctx, sessionID, request)
	}
	return s.SaveReplaceCurrentForSession(ctx, sessionID, request)
}

// SaveReplaceCurrentForSession maps the generated compatibility request into
// the Factory-owned save contract and maps its detached result back at the edge.
func (s *Service) SaveReplaceCurrentForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	domainRequest, err := editableFactoryFromAPI(request)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	saved, err := s.SaveReplaceCurrentSnapshotForSession(ctx, sessionID, domainRequest)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return factorySnapshotToAPI(saved)
}

// SaveUpsertNamedAndActivateForSession maps the generated compatibility request
// into the Factory-owned named-upsert contract and maps its result back.
func (s *Service) SaveUpsertNamedAndActivateForSession(
	ctx context.Context,
	sessionID string,
	request factoryapi.Factory,
) (factoryapi.Factory, error) {
	domainRequest, err := editableFactoryFromAPI(request)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	saved, err := s.SaveUpsertNamedSnapshotAndActivateForSession(ctx, sessionID, domainRequest)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	mapped, err := factorySnapshotToAPI(saved.Snapshot)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if saved.Version != nil {
		version := factoryVersionToAPI(*saved.Version)
		mapped.Version = &version
	}
	return mapped, nil
}

func editableFactoryFromAPI(request factoryapi.Factory) (EditableFactory, error) {
	snapshot, err := interfaces.NewFactorySnapshot(request)
	if err != nil {
		return EditableFactory{}, fmt.Errorf("capture editable factory snapshot: %w", err)
	}
	return EditableFactory{
		Name:     string(request.Name),
		Snapshot: snapshot,
		Version:  factoryVersionFromAPI(request.Version),
	}, nil
}

func factorySnapshotToAPI(snapshot *interfaces.FactorySnapshot) (factoryapi.Factory, error) {
	mapped, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map editable factory snapshot: %w", err)
	}
	return *mapped, nil
}

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved from the persisted pointer and canonical on-disk layout.
func (s *Service) GetCurrentNamedFactory(context.Context) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}

	rootDir := s.host.PersistRootDir()
	name, err := configpersist.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := s.host.CurrentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return s.serializeNamedFactoryToAPI(string(apisurface.DefaultCurrentFactoryName), currentRuntime, true)
			}
			return factoryapi.Factory{}, ErrCurrentFactoryNotFound
		}
		return factoryapi.Factory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	current, err := configload.LoadRuntimeConfig(factoryDir, s.host.WorkstationLoader())
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current factory %q: %w", name, err)
	}

	return s.serializeNamedFactoryToAPI(name, current, true)
}

// GetCurrentFactoryForSession returns the editable factory definition read model
// for one live session, including durable version metadata when available.
func (s *Service) GetCurrentFactoryForSession(_ context.Context, sessionID string) (factoryapi.Factory, error) {
	if s == nil || s.host == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}

	session, err := s.host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	runtimeCfg, err := s.host.SessionRuntimeConfig(sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	rootDir := factorysessions.SessionFactoryRootDir(s.host.PersistRootDir(), session)
	factoryName := factoryapi.FactoryName(factorysessions.FactoryName(rootDir, runtimeCfg))
	versionRootDir := rootDir
	if persistRoot := s.host.SessionFactoryPersistRoot(session); persistRoot != "" {
		if pointerName, err := configpersist.ReadCurrentFactoryPointer(persistRoot); err == nil {
			pointerFactoryName := factoryapi.FactoryName(pointerName)
			if session.IsDefault || pointerFactoryName == factoryName {
				factoryName = pointerFactoryName
			}
		}
		if sameFactoryDir(persistRoot, rootDir) {
			versionRootDir = persistRoot
		}
	}
	serialized, err := s.serializeNamedFactoryToAPI(string(factoryName), runtimeCfg, true)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return s.withCurrentFactoryVersion(versionRootDir, serialized.Name, serialized)
}

// CurrentFactoryDefinitionVersionAtRoot returns optimistic-concurrency metadata
// for the factory definition stored at rootDir.
func (s *Service) CurrentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	if s == nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("factory definition service is required")
	}
	version, err := s.currentFactoryDefinitionVersionAtRoot(rootDir, string(name))
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, err
	}
	return factoryVersionToAPI(version), nil
}

// SerializeNamedFactory returns the canonical editable factory read model for one
// loaded runtime config.
func (s *Service) SerializeNamedFactory(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (factoryapi.Factory, error) {
	if s == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	return s.serializeNamedFactoryToAPI(string(name), current, inlineBundledFiles)
}

func (s *Service) serializeNamedFactoryToAPI(
	name string,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (factoryapi.Factory, error) {
	snapshot, err := s.serializeNamedFactory(name, current, inlineBundledFiles)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	generatedFactory, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map current factory snapshot: %w", err)
	}
	return *generatedFactory, nil
}

func (s *Service) withCurrentFactoryVersion(
	rootDir string,
	name factoryapi.FactoryName,
	serialized factoryapi.Factory,
) (factoryapi.Factory, error) {
	version, err := s.currentFactoryDefinitionVersionAtRoot(rootDir, string(name))
	if err != nil {
		return factoryapi.Factory{}, err
	}
	generatedVersion := factoryVersionToAPI(version)
	serialized.Version = &generatedVersion
	return serialized, nil
}

func factoryVersionFromAPI(version *factoryapi.HybridLogicalTimestamp) *interfaces.FactoryVersion {
	if version == nil {
		return nil
	}
	mapped := factoryVersionFromAPIValue(*version)
	return &mapped
}

func factoryVersionFromAPIValue(version factoryapi.HybridLogicalTimestamp) interfaces.FactoryVersion {
	return interfaces.FactoryVersion{
		Logical:  version.Logical.Int64(),
		Physical: version.Physical.UTC(),
	}
}

func factoryVersionToAPI(version interfaces.FactoryVersion) factoryapi.HybridLogicalTimestamp {
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(version.Logical),
		Physical: version.Physical.UTC(),
	}
}

func sameFactoryDir(left, right string) bool {
	return factorysessions.SameFactoryDir(left, right)
}

// SessionFactoryPersistRoot resolves the on-disk factory root for
// session-scoped definition persistence.
func SessionFactoryPersistRoot(serviceRootDir string, session *factorysessions.LiveSession) string {
	if session != nil && !session.IsDefault && strings.TrimSpace(session.FolderPath) != "" {
		return session.FolderPath
	}
	return factorysessions.SessionFactoryRootDir(serviceRootDir, session)
}
