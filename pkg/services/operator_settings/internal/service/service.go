// Package service implements the published Operator Settings root Service by
// delegating document operations and effective resolution to parent-private
// owner subservices.
package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	identityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/identityinputinventory"
	settingsdocument "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document"
	resolution "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/resolution"
)

// Service fulfills the published Operator Settings root contract.
type Service struct {
	document    settingsdocument.Service
	resolution  resolution.Service
	files       operatorsettings.FileSystem
	createTemp  operatorsettings.CreateTemporaryFile
	decoder     operatorsettings.ConfigDecoder
	encoder     operatorsettings.ConfigEncoder
	idGenerator operatorsettings.IDGenerator
	writeMu     sync.Mutex
}

var _ operatorsettings.Service = (*Service)(nil)

// New constructs an inert Operator Settings root facade over the private
// document and resolution capabilities.
func New(
	documentService settingsdocument.Service,
	resolutionService resolution.Service,
	files operatorsettings.FileSystem,
	createTemp operatorsettings.CreateTemporaryFile,
	decoder operatorsettings.ConfigDecoder,
	encoder operatorsettings.ConfigEncoder,
	idGenerator operatorsettings.IDGenerator,
) (operatorsettings.Service, error) {
	if documentService == nil {
		return nil, fmt.Errorf("construct Operator Settings: document is required")
	}
	if resolutionService == nil {
		return nil, fmt.Errorf("construct Operator Settings: resolution is required")
	}
	return &Service{
		document:    documentService,
		resolution:  resolutionService,
		files:       files,
		createTemp:  createTemp,
		decoder:     decoder,
		encoder:     encoder,
		idGenerator: idGenerator,
	}, nil
}

func (s *Service) LoadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if s == nil || s.document == nil {
		return operatorsettings.LoadDocumentResult{}, fmt.Errorf("operator settings document service is required")
	}
	return s.document.LoadDocument(request)
}

func (s *Service) ApplyDocumentUpdate(
	request operatorsettings.ApplyDocumentUpdateRequest,
) (operatorsettings.ApplyDocumentUpdateResult, error) {
	if s == nil || s.document == nil {
		return operatorsettings.ApplyDocumentUpdateResult{}, fmt.Errorf("operator settings document service is required")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.document.ApplyDocumentUpdate(request)
}

func (s *Service) ResolveEffective(
	request operatorsettings.ResolveEffectiveRequest,
) (operatorsettings.ResolveEffectiveResult, error) {
	if s == nil || s.resolution == nil {
		return operatorsettings.ResolveEffectiveResult{}, fmt.Errorf("operator settings resolution service is required")
	}
	return s.resolution.ResolveEffective(request)
}

func (s *Service) DefaultConfigPath(homeDir string) string {
	return filepath.Join(strings.TrimSpace(homeDir), ".you-agent-factory", "config.json")
}

func (s *Service) LoadFileConfig(path string) (operatorsettings.Config, error) {
	if s == nil || s.files == nil {
		return operatorsettings.Config{}, fmt.Errorf("operator settings filesystem is required")
	}
	if s.decoder == nil {
		return operatorsettings.Config{}, fmt.Errorf("operator settings decoder is required")
	}
	return loadFileConfig(s.files, s.decoder, path)
}

func (s *Service) ResolveFromHomeWithEnvironment(
	homeDir string,
	environment operatorsettings.Defaults,
	flags operatorsettings.FlagOverrides,
) (operatorsettings.ResolvedDefaults, error) {
	if s == nil {
		return operatorsettings.ResolvedDefaults{}, fmt.Errorf("operator settings service is required")
	}
	configPath := s.DefaultConfigPath(homeDir)
	config, err := s.LoadFileConfig(configPath)
	if err != nil {
		return operatorsettings.ResolvedDefaults{}, err
	}
	if s.resolution == nil {
		return operatorsettings.ResolvedDefaults{}, fmt.Errorf("operator settings resolution service is required")
	}
	resolved, err := s.resolution.ResolveEffective(operatorsettings.ResolveEffectiveRequest{
		DocumentBaseline: operatorsettings.DocumentDefaults{
			WorkerModelProvider: config.Defaults.WorkerModelProvider,
			WorkerModel:         config.Defaults.WorkerModel,
		},
		WorkerPresets: documentWorkerPresets(config.WorkerPresets),
		EnvironmentOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: strings.TrimSpace(environment.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(environment.WorkerModel),
		},
		InvocationOverrides: operatorsettings.EffectiveOverrideFacts{
			WorkerModelProvider: strings.TrimSpace(flags.WorkerModelProvider),
			WorkerModel:         strings.TrimSpace(flags.WorkerModel),
		},
		ConfigPath: configPath,
	})
	if err != nil {
		var failure operatorsettings.ResolutionFailure
		if errors.As(err, &failure) &&
			failure.Kind == operatorsettings.ResolutionFailureKindInvalidInput &&
			strings.Contains(failure.Message, "symbolic DEFAULT") {
			return operatorsettings.ResolvedDefaults{}, fmt.Errorf(
				"%w; set defaults.workerModelProvider, YOU_DEFAULT_WORKER_MODEL_PROVIDER, or you run --provider to a supported provider",
				err,
			)
		}
		return operatorsettings.ResolvedDefaults{}, err
	}
	return operatorsettings.ResolvedDefaults{
		WorkerModelProvider:       resolved.Selection.WorkerModelProvider,
		WorkerModel:               resolved.Selection.WorkerModel,
		WorkerModelProviderSource: operatorsettings.Source(resolved.Selection.WorkerModelProviderSource),
		WorkerModelSource:         operatorsettings.Source(resolved.Selection.WorkerModelSource),
		ConfigPath:                resolved.Selection.ConfigPath,
	}, nil
}

func documentWorkerPresets(presets []operatorsettings.WorkerPreset) []operatorsettings.DocumentWorkerPreset {
	if presets == nil {
		return nil
	}
	converted := make([]operatorsettings.DocumentWorkerPreset, len(presets))
	for index, preset := range presets {
		converted[index] = operatorsettings.DocumentWorkerPreset{
			ID: preset.ID, ModelProvider: preset.ModelProvider,
			Model: preset.Model, ReasoningEffort: preset.ReasoningEffort,
		}
	}
	return converted
}

func (s *Service) EnsureLocalBackendScope(path string) (operatorsettings.ResolvedBackendScope, error) {
	if s == nil {
		return operatorsettings.ResolvedBackendScope{}, fmt.Errorf("operator settings service is required")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.ensureLocalBackendScope(path)
}

func (s *Service) ProjectInputInventory() operatorsettings.InputInventory {
	if s == nil {
		return operatorsettings.InputInventory{}
	}
	return identityinventory.ProjectInputInventory()
}

func (s *Service) DeriveProviderBackendScopeID(provider, kind, boundary string) string {
	return deriveProviderBackendScopeID(provider, kind, boundary)
}

func (s *Service) IsLocalBackendScopeID(value string) bool {
	return isLocalBackendScopeID(value)
}

func (s *Service) ConfigureACPIntegrationAdd(
	ctx context.Context,
	path string,
	integration operatorsettings.ACPIntegration,
) (operatorsettings.Document, error) {
	return s.mutateDocument(ctx, path, func(document operatorsettings.Document) (operatorsettings.Document, error) {
		config := documentConfig(document)
		config.Workers.ACP.Integrations = append(config.Workers.ACP.Integrations, integration)
		return normalizedDocument(config)
	})
}

func (s *Service) ConfigureACPIntegrationDelete(
	ctx context.Context,
	path string,
	name string,
) (operatorsettings.Document, error) {
	return s.mutateDocument(ctx, path, func(document operatorsettings.Document) (operatorsettings.Document, error) {
		config := documentConfig(document)
		name = strings.TrimSpace(name)
		filtered := make([]operatorsettings.ACPIntegration, 0, len(config.Workers.ACP.Integrations))
		found := false
		for _, integration := range config.Workers.ACP.Integrations {
			if integration.Name == name {
				found = true
				continue
			}
			filtered = append(filtered, integration)
		}
		if !found {
			return operatorsettings.Document{}, fmt.Errorf("%w: %q", operatorsettings.ErrACPIntegrationNotFound, name)
		}
		config.Workers.ACP.Integrations = filtered
		return normalizedDocument(config)
	})
}

func (s *Service) EnsurePackagedACPIntegrations(
	ctx context.Context,
	path string,
	defaults []operatorsettings.ACPIntegration,
) (operatorsettings.Document, error) {
	return s.mutateDocument(ctx, path, func(document operatorsettings.Document) (operatorsettings.Document, error) {
		config := documentConfig(document)
		if config.Workers.ACP.Integrations != nil {
			return document, nil
		}
		config.Workers.ACP.Integrations = append([]operatorsettings.ACPIntegration(nil), defaults...)
		return normalizedDocument(config)
	})
}

// ResolveACPAgentProfile resolves the effective ACP Agent profile for the
// operator document at path through the existing injected document
// capability, without mutating or persisting the document. An absent profile
// resolves to the safe Factory Builder default; a malformed stored profile
// fails explicitly instead of falling back silently.
func (s *Service) ResolveACPAgentProfile(path string) (operatorsettings.ACPAgentProfile, error) {
	if s == nil || s.document == nil {
		return operatorsettings.ACPAgentProfile{}, fmt.Errorf("operator settings document service is required")
	}
	loaded, err := s.document.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.ACPAgentProfile{}, err
	}
	profile := loaded.Document.Workers.ACP.AgentProfile
	if profile == nil {
		return operatorsettings.DefaultACPAgentProfile(), nil
	}
	return profile.Clone().Normalize()
}

// UpdateACPAgentProfile validates the complete candidate profile before any
// persistence side effect, then atomically stores the normalized profile
// while preserving every other operator setting.
func (s *Service) UpdateACPAgentProfile(
	ctx context.Context,
	path string,
	profile operatorsettings.ACPAgentProfile,
) (operatorsettings.ACPAgentProfile, error) {
	normalized, err := profile.Normalize()
	if err != nil {
		return operatorsettings.ACPAgentProfile{}, err
	}
	updated, err := s.mutateDocument(ctx, path, func(document operatorsettings.Document) (operatorsettings.Document, error) {
		config := documentConfig(document)
		candidate := normalized
		config.Workers.ACP.AgentProfile = &candidate
		return normalizedDocument(config)
	})
	if err != nil {
		return operatorsettings.ACPAgentProfile{}, err
	}
	if updated.Workers.ACP.AgentProfile == nil {
		return operatorsettings.ACPAgentProfile{}, fmt.Errorf("operator settings: update ACP Agent profile: persisted document is missing the profile")
	}
	return updated.Workers.ACP.AgentProfile.Clone(), nil
}

func (s *Service) mutateDocument(
	ctx context.Context,
	path string,
	update func(operatorsettings.Document) (operatorsettings.Document, error),
) (operatorsettings.Document, error) {
	if ctx == nil {
		return operatorsettings.Document{}, fmt.Errorf("operator config context is required")
	}
	if err := ctx.Err(); err != nil {
		return operatorsettings.Document{}, err
	}
	if s == nil || s.document == nil {
		return operatorsettings.Document{}, fmt.Errorf("operator settings document service is required")
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	loaded, err := s.document.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return operatorsettings.Document{}, err
	}
	candidate, err := update(loaded.Document)
	if err != nil {
		return operatorsettings.Document{}, err
	}
	if err := s.document.PersistDocument(ctx, operatorsettings.PersistDocumentRequest{Path: path, Document: candidate}); err != nil {
		return operatorsettings.Document{}, err
	}
	return candidate, nil
}

func documentConfig(document operatorsettings.Document) operatorsettings.Config {
	return operatorsettings.Config{
		BackendScopeID: document.BackendScopeID,
		Defaults: operatorsettings.Defaults{
			WorkerModelProvider: document.Defaults.WorkerModelProvider,
			WorkerModel:         document.Defaults.WorkerModel,
		},
		Runtime: operatorsettings.RuntimeSettings{
			Logging: operatorsettings.RuntimeArtifactSettings(document.Runtime.Logging),
			Metrics: operatorsettings.RuntimeArtifactSettings(document.Runtime.Metrics),
		},
		Workers: operatorsettings.WorkerSettings{ACP: operatorsettings.ACPSettings{
			Integrations: append([]operatorsettings.ACPIntegration(nil), document.Workers.ACP.Integrations...),
			AgentProfile: cloneACPAgentProfilePointer(document.Workers.ACP.AgentProfile),
		}},
		WorkerPresets: workerPresetsFromDocument(document.WorkerPresets),
	}
}

// cloneACPAgentProfilePointer returns a detached copy of an optional ACP
// Agent profile pointer, preserving nil for an absent profile.
func cloneACPAgentProfilePointer(profile *operatorsettings.ACPAgentProfile) *operatorsettings.ACPAgentProfile {
	if profile == nil {
		return nil
	}
	cloned := profile.Clone()
	return &cloned
}

func workerPresetsFromDocument(presets []operatorsettings.DocumentWorkerPreset) []operatorsettings.WorkerPreset {
	if presets == nil {
		return nil
	}
	converted := make([]operatorsettings.WorkerPreset, len(presets))
	for index, preset := range presets {
		converted[index] = operatorsettings.WorkerPreset{
			ID: preset.ID, ModelProvider: preset.ModelProvider,
			Model: preset.Model, ReasoningEffort: preset.ReasoningEffort,
		}
	}
	return converted
}

func normalizedDocument(config operatorsettings.Config) (operatorsettings.Document, error) {
	normalized, err := config.Normalize()
	if err != nil {
		return operatorsettings.Document{}, err
	}
	document := operatorsettings.Document{
		BackendScopeID: normalized.BackendScopeID,
		Defaults: operatorsettings.DocumentDefaults{
			WorkerModelProvider: normalized.Defaults.WorkerModelProvider,
			WorkerModel:         normalized.Defaults.WorkerModel,
		},
		Runtime: operatorsettings.DocumentRuntimeSettings{
			Logging: operatorsettings.DocumentRuntimeArtifactSettings(normalized.Runtime.Logging),
			Metrics: operatorsettings.DocumentRuntimeArtifactSettings(normalized.Runtime.Metrics),
		},
		Workers: operatorsettings.DocumentWorkerSettings{ACP: operatorsettings.DocumentACPSettings{
			Integrations: append([]operatorsettings.ACPIntegration(nil), normalized.Workers.ACP.Integrations...),
			AgentProfile: cloneACPAgentProfilePointer(normalized.Workers.ACP.AgentProfile),
		}},
		WorkerPresets: make([]operatorsettings.DocumentWorkerPreset, len(normalized.WorkerPresets)),
	}
	for index, preset := range normalized.WorkerPresets {
		document.WorkerPresets[index] = operatorsettings.DocumentWorkerPreset{
			ID: preset.ID, ModelProvider: preset.ModelProvider,
			Model: preset.Model, ReasoningEffort: preset.ReasoningEffort,
		}
	}
	if normalized.WorkerPresets == nil {
		document.WorkerPresets = nil
	}
	return document, nil
}
