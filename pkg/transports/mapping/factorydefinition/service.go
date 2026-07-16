// Package factorydefinition maps generated Factory definition contracts while
// delegating persistence and activation policy to the Factory owner.
package factorydefinition

import (
	"context"
	"fmt"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	domain "github.com/portpowered/infinite-you/pkg/factory/definition"
	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorysnapshot"
)

// Service adapts one Factory-owned definition service to generated contracts.
type Service struct{ domain *domain.Service }

// New constructs a generated-contract adapter around a Factory definition owner.
func New(owner *domain.Service) *Service { return &Service{domain: owner} }

// ActivateNamedFactory delegates named activation policy to the Factory owner.
func (s *Service) ActivateNamedFactory(ctx context.Context, name string) error {
	if s == nil || s.domain == nil {
		return fmt.Errorf("factory definition service is required")
	}
	return s.domain.ActivateNamedFactory(ctx, name)
}

func (s *Service) Save(ctx context.Context, sessionID string, mode factoryapi.FactorySaveMode, request factoryapi.Factory) (factoryapi.Factory, error) {
	if s == nil || s.domain == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	editable, err := editableFactoryFromAPI(request)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	saved, err := s.domain.Save(ctx, sessionID, saveModeFromAPI(mode), editable)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return editableFactoryToAPI(saved)
}

func (s *Service) GetCurrentNamedFactory(ctx context.Context) (factoryapi.Factory, error) {
	if s == nil || s.domain == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	snapshot, err := s.domain.GetCurrentNamedFactory(ctx)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return snapshotToAPI(snapshot)
}

func (s *Service) GetCurrentFactoryForSession(ctx context.Context, sessionID string) (factoryapi.Factory, error) {
	if s == nil || s.domain == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	editable, err := s.domain.GetCurrentFactoryForSession(ctx, sessionID)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return editableFactoryToAPI(editable)
}

func (s *Service) CurrentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	if s == nil || s.domain == nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("factory definition service is required")
	}
	version, err := s.domain.CurrentFactoryDefinitionVersionAtRoot(rootDir, string(name))
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, err
	}
	return factoryVersionToAPI(version), nil
}

func (s *Service) SerializeNamedFactory(name factoryapi.FactoryName, current *factoryconfig.LoadedFactoryConfig, inlineBundledFiles bool) (factoryapi.Factory, error) {
	if s == nil || s.domain == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory definition service is required")
	}
	snapshot, err := s.domain.SerializeNamedFactory(string(name), current, inlineBundledFiles)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return snapshotToAPI(snapshot)
}

func saveModeFromAPI(mode factoryapi.FactorySaveMode) domain.SaveMode {
	if mode == factoryapi.FactorySaveModeUpsertNamedAndActivate {
		return domain.SaveModeUpsertNamedAndActivate
	}
	return domain.SaveModeReplaceCurrent
}

func editableFactoryFromAPI(request factoryapi.Factory) (domain.EditableFactory, error) {
	snapshot, err := interfaces.NewFactorySnapshot(request)
	if err != nil {
		return domain.EditableFactory{}, fmt.Errorf("capture editable factory snapshot: %w", err)
	}
	return domain.EditableFactory{Name: string(request.Name), Snapshot: snapshot, Version: factoryVersionFromAPI(request.Version)}, nil
}

func editableFactoryToAPI(editable domain.EditableFactory) (factoryapi.Factory, error) {
	mapped, err := snapshotToAPI(editable.Snapshot)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if editable.Version != nil {
		version := factoryVersionToAPI(*editable.Version)
		mapped.Version = &version
	}
	return mapped, nil
}

func snapshotToAPI(snapshot *interfaces.FactorySnapshot) (factoryapi.Factory, error) {
	mapped, err := factorysnapshot.ToAPI(snapshot)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("map editable factory snapshot: %w", err)
	}
	if mapped == nil {
		return factoryapi.Factory{}, fmt.Errorf("editable factory snapshot is required")
	}
	return *mapped, nil
}

func factoryVersionFromAPI(version *factoryapi.HybridLogicalTimestamp) *interfaces.FactoryVersion {
	if version == nil {
		return nil
	}
	return &interfaces.FactoryVersion{Logical: version.Logical.Int64(), Physical: version.Physical.UTC()}
}

func factoryVersionToAPI(version interfaces.FactoryVersion) factoryapi.HybridLogicalTimestamp {
	return factoryapi.HybridLogicalTimestamp{Logical: apitypes.Int64String(version.Logical), Physical: version.Physical.UTC()}
}
