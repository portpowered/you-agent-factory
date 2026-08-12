// Package runtimesnapshot owns the Definitions-side conversion from a loaded
// Factory source into a detached Runtime input.
package runtimesnapshot

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service resolves authored or canonical sources through injected Definitions
// loaders. The loaded source is used only during the operation and never
// escapes in the result.
type Service struct {
	loadCanonical     factorydefinitions.CanonicalFactoryJSONLoader
	loadFactory       factorydefinitions.LoadedFactoryLoader
	workstationLoader func() factorydefinitions.WorkstationLoader
}

// New constructs a snapshot resolver from the two source-loading forms used
// by Factory Definitions.
func New(
	loadCanonical factorydefinitions.CanonicalFactoryJSONLoader,
	loadFactory factorydefinitions.LoadedFactoryLoader,
	workstationLoader func() factorydefinitions.WorkstationLoader,
) (*Service, error) {
	if loadCanonical == nil {
		return nil, fmt.Errorf("canonical Factory loader is required")
	}
	if loadFactory == nil {
		return nil, fmt.Errorf("Factory directory loader is required")
	}
	return &Service{
		loadCanonical:     loadCanonical,
		loadFactory:       loadFactory,
		workstationLoader: workstationLoader,
	}, nil
}

// ResolveRuntimeSnapshot loads, validates, and deeply detaches one Factory
// source. Loader errors remain available through the typed resolution error's
// unwrap chain, so validation callers can still inspect Definitions findings.
func (s *Service) ResolveRuntimeSnapshot(
	ctx context.Context,
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (factorydefinitions.ResolveRuntimeSnapshotResult, error) {
	if ctx == nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, invalidRequest(
			"context",
			"invocation context is required",
			nil,
		)
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, canceled(err)
	}

	factoryDir, sourcePath, hasCanonical, err := normalizeRequest(request)
	if err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, err
	}

	var loaded factorydefinitions.MutableLoadedFactorySource
	var workstationLoader factorydefinitions.WorkstationLoader
	if s.workstationLoader != nil {
		workstationLoader = s.workstationLoader()
	}
	if hasCanonical {
		canonical := append([]byte(nil), request.Canonical...)
		loaded, err = s.loadCanonical(canonical, workstationLoader)
	} else {
		loaded, err = s.loadFactory(factoryDirOrSourcePath(factoryDir, sourcePath), workstationLoader)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, canceled(contextErr)
	}
	if err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, invalidDefinition(
			"source",
			"Factory source could not be loaded or validated",
			err,
		)
	}
	if loaded == nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, invalidDefinition(
			"source",
			"Factory loader returned no source",
			nil,
		)
	}

	if baseDir := strings.TrimSpace(request.ExecutionBaseDir); baseDir != "" {
		loaded.SetRuntimeBaseDir(baseDir)
	}
	if err := ctx.Err(); err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, canceled(err)
	}

	snapshot, err := detach(loaded, request.Invocation)
	if err != nil {
		return factorydefinitions.ResolveRuntimeSnapshotResult{}, invalidDefinition(
			"effectiveFactory",
			"effective Factory could not be detached",
			err,
		)
	}
	return factorydefinitions.ResolveRuntimeSnapshotResult{Snapshot: snapshot}, nil
}

func normalizeRequest(
	request factorydefinitions.ResolveRuntimeSnapshotRequest,
) (string, string, bool, error) {
	factoryDir := strings.TrimSpace(request.FactoryDir)
	sourcePath := strings.TrimSpace(request.SourcePath)
	hasCanonical := len(request.Canonical) > 0

	if factoryDir != "" && sourcePath != "" && factoryDir != sourcePath {
		return "", "", false, invalidRequest(
			"FactoryDir",
			"FactoryDir and SourcePath identify different sources",
			nil,
		)
	}
	if hasCanonical && (factoryDir != "" || sourcePath != "") {
		return "", "", false, invalidRequest(
			"source",
			"exactly one of a Factory path or canonical source is required",
			nil,
		)
	}
	if !hasCanonical && factoryDir == "" && sourcePath == "" {
		return "", "", false, invalidRequest(
			"source",
			"a Factory path or canonical source is required",
			nil,
		)
	}
	return factoryDir, sourcePath, hasCanonical, nil
}

func factoryDirOrSourcePath(factoryDir, sourcePath string) string {
	if factoryDir != "" {
		return factoryDir
	}
	return sourcePath
}

func detach(
	loaded factorydefinitions.MutableLoadedFactorySource,
	invocation factorydefinitions.RuntimeSnapshotInvocationContext,
) (factorydefinitions.RuntimeSnapshot, error) {
	sourceConfig := loaded.FactoryConfig()
	config, err := factorydefinitions.CloneFactoryConfig(sourceConfig)
	if err != nil {
		return factorydefinitions.RuntimeSnapshot{}, err
	}
	if config == nil {
		return factorydefinitions.RuntimeSnapshot{}, fmt.Errorf("loaded Factory has no effective configuration")
	}
	// CloneFactoryConfig intentionally follows the persisted JSON shape. The
	// loaded runtime source also carries value-only worker metadata that is
	// excluded from that shape (for example concurrency and operator defaults),
	// so restore those fields through the worker/workstation clone helpers.
	for index := range config.Workers {
		if index < len(sourceConfig.Workers) {
			config.Workers[index] = factorydefinitions.CloneWorkerConfig(sourceConfig.Workers[index])
		}
	}
	for index := range config.Workstations {
		if index < len(sourceConfig.Workstations) {
			config.Workstations[index] = factorydefinitions.CloneWorkstationConfig(sourceConfig.Workstations[index])
		}
	}

	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir:        loaded.FactoryDir(),
		RuntimeBaseDir:    loaded.RuntimeBaseDir(),
		Invocation:        invocation,
		EffectiveFactory:  *config,
		Workers:           make([]factorydefinitions.FactoryWorkerConfig, 0, len(config.Workers)),
		Workstations:      make([]factorydefinitions.FactoryWorkstationConfig, 0, len(config.Workstations)),
		AutomationSources: make([]factorydefinitions.RuntimeAutomationSource, 0),
		PromptSources:     make([]factorydefinitions.RuntimePromptSource, 0),
		BundledFiles:      append([]factorydefinitions.PortableBundledFileReplacement(nil), loaded.PortableBundledFileReplacements()...),
	}
	if config.Version != nil {
		version := *config.Version
		snapshot.DefinitionVersion = &version
	}

	workersByName := make(map[string]factorydefinitions.FactoryWorkerConfig, len(config.Workers))
	for _, worker := range config.Workers {
		cloned := factorydefinitions.CloneWorkerConfig(worker)
		snapshot.Workers = append(snapshot.Workers, cloned)
		workersByName[cloned.Name] = cloned
	}
	for _, workstation := range config.Workstations {
		cloned := factorydefinitions.CloneWorkstationConfig(workstation)
		snapshot.Workstations = append(snapshot.Workstations, cloned)
		worker, hasWorker := workersByName[cloned.WorkerTypeName]
		if source, ok := automationSource(cloned, worker, hasWorker); ok {
			snapshot.AutomationSources = append(snapshot.AutomationSources, source)
		}
	}

	if promptSources, ok := loaded.(factorydefinitions.RuntimePromptSourceLookup); ok {
		for _, worker := range snapshot.Workers {
			if source, exists := promptSources.WorkerPromptSource(worker.Name); exists {
				snapshot.PromptSources = append(snapshot.PromptSources, factorydefinitions.RuntimePromptSource{
					Role:       "worker",
					Name:       worker.Name,
					Path:       source.Path,
					IsTemplate: source.IsTemplate,
				})
			}
		}
		for _, workstation := range snapshot.Workstations {
			if source, exists := promptSources.WorkstationPromptSource(workstation.Name); exists {
				snapshot.PromptSources = append(snapshot.PromptSources, factorydefinitions.RuntimePromptSource{
					Role:       "workstation",
					Name:       workstation.Name,
					Path:       source.Path,
					IsTemplate: source.IsTemplate,
				})
			}
		}
	}

	return snapshot, nil
}

func automationSource(
	workstation factorydefinitions.FactoryWorkstationConfig,
	worker factorydefinitions.FactoryWorkerConfig,
	hasWorker bool,
) (factorydefinitions.RuntimeAutomationSource, bool) {
	kind := factorydefinitions.RuntimeAutomationSourceKindWorkstation
	switch {
	case workstation.Cron != nil:
		kind = factorydefinitions.RuntimeAutomationSourceKindCron
	case workstation.Type == factorydefinitions.WorkstationTypeScript:
		kind = factorydefinitions.RuntimeAutomationSourceKindScript
	case workstation.Type == factorydefinitions.WorkstationTypePoller:
		kind = factorydefinitions.RuntimeAutomationSourceKindPoller
	case hasWorker && worker.Type == factorydefinitions.WorkerTypeHosted:
		kind = factorydefinitions.RuntimeAutomationSourceKindHosted
	default:
		return factorydefinitions.RuntimeAutomationSource{}, false
	}

	workerName := workstation.WorkerTypeName
	var workerValue *factorydefinitions.FactoryWorkerConfig
	if hasWorker {
		cloned := factorydefinitions.CloneWorkerConfig(worker)
		workerValue = &cloned
	}
	source := factorydefinitions.RuntimeAutomationSource{
		ID:              workstation.ID,
		Kind:            kind,
		WorkstationName: workstation.Name,
		WorkerName:      workerName,
		Workstation:     factorydefinitions.CloneWorkstationConfig(workstation),
		Worker:          workerValue,
	}
	if source.ID == "" {
		source.ID = source.WorkstationName
	}
	if workstation.Cron != nil {
		source.Schedule = workstation.Cron.Schedule
		source.Every = workstation.Cron.Every
		source.TriggerAtStart = workstation.Cron.TriggerAtStart
	}
	return source, true
}

func invalidRequest(field, message string, cause error) error {
	return &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticInvalidRequest,
			Field:   field,
			Message: message,
		},
		Cause: cause,
	}
}

func invalidDefinition(field, message string, cause error) error {
	return &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticInvalidDefinition,
			Field:   field,
			Message: message,
		},
		Cause: cause,
	}
}

func canceled(cause error) error {
	return &factorydefinitions.RuntimeSnapshotResolutionError{
		Diagnostic: factorydefinitions.RuntimeSnapshotDiagnostic{
			Code:    factorydefinitions.RuntimeSnapshotDiagnosticCanceled,
			Message: "runtime snapshot resolution was canceled",
		},
		Cause: cause,
	}
}
