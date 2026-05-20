package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/replay"

	"go.uber.org/zap"
)

// ActivateNamedFactory builds a replacement runtime from a persisted named
// factory directory and swaps it in only after the current runtime is idle.
func (fs *FactoryService) ActivateNamedFactory(ctx context.Context, name string) error {
	if fs == nil {
		return fmt.Errorf("factory service is required")
	}
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return err
	}

	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir)
	if err != nil {
		return fmt.Errorf("%w: build replacement factory %q: %v", ErrInvalidNamedFactory, name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}
	return fs.activateReplacementRuntime(ctx, rootDir, name, replacement)
}

func (fs *FactoryService) activateReplacementRuntime(
	ctx context.Context,
	rootDir string,
	name string,
	replacement *replacementFactoryRuntime,
) error {
	runState := fs.currentRunState()
	if runState == nil || runState.runtime == nil || runState.ctx == nil {
		if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, name); err != nil {
			return err
		}
		fs.swapActiveRuntime(replacement)
		return nil
	}

	restoreCurrentSidecars := false
	serviceMode := fs.cfg != nil && runtimeModeOrDefault(fs.cfg.RuntimeMode) == interfaces.RuntimeModeService
	if serviceMode {
		fs.stopLiveRuntimeSidecars(runState.runtime)
		restoreCurrentSidecars = true
		defer func() {
			if restoreCurrentSidecars {
				fs.restoreLiveRuntimeSidecars(runState)
			}
		}()
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return err
	}

	replacementHandle := fs.startLiveRuntime(runState.ctx, replacement)
	if err := fs.waitForLiveRuntimeStart(ctx, replacementHandle); err != nil {
		_ = fs.stopLiveRuntime(replacementHandle)
		return fmt.Errorf("start replacement runtime: %w", err)
	}

	if serviceMode {
		if err := fs.startLiveRuntimeSidecars(runState.ctx, replacementHandle); err != nil {
			_ = fs.stopLiveRuntime(replacementHandle)
			return fmt.Errorf("start replacement runtime sidecars: %w", err)
		}
	}
	if err := factoryconfig.WriteCurrentFactoryPointer(rootDir, name); err != nil {
		if serviceMode {
			fs.stopLiveRuntimeSidecars(replacementHandle)
		}
		_ = fs.stopLiveRuntime(replacementHandle)
		return err
	}

	fs.publishFactoryChangeEvent(ctx, runState.runtime, replacement)
	restoreCurrentSidecars = false
	fs.swapActiveRuntime(replacement)
	fs.setRunState(runState.ctx, replacementHandle)
	if err := fs.stopLiveRuntime(runState.runtime); err != nil && err != context.Canceled {
		fs.logger.Warn("prior runtime shutdown failed", zap.Error(err))
	}
	return nil
}

func (fs *FactoryService) requireIdleRuntime(ctx context.Context) error {
	snapshot, err := fs.GetEngineStateSnapshot(ctx)
	if err != nil {
		return fmt.Errorf("read current runtime status: %w", err)
	}
	if snapshot.RuntimeStatus != interfaces.RuntimeStatusIdle {
		return fmt.Errorf("%w: current runtime status is %s", ErrFactoryActivationRequiresIdle, snapshot.RuntimeStatus)
	}
	if snapshotHasActiveWork(snapshot) {
		return fmt.Errorf("%w: current runtime has active work", ErrFactoryActivationRequiresIdle)
	}
	return nil
}

func snapshotHasActiveWork(snapshot *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) bool {
	if snapshot == nil {
		return false
	}
	if snapshot.InFlightCount > 0 || len(snapshot.Dispatches) > 0 {
		return true
	}
	for _, token := range snapshot.Marking.Tokens {
		if token == nil || token.Color.DataType == interfaces.DataTypeResource {
			continue
		}
		if snapshot.Topology == nil {
			return true
		}
		category := snapshot.Topology.StateCategoryForPlace(token.PlaceID)
		if category != state.StateCategoryTerminal && category != state.StateCategoryFailed {
			return true
		}
	}
	return false
}

func (fs *FactoryService) currentRuntimeBundle() *replacementFactoryRuntime {
	if fs == nil {
		return nil
	}
	fs.runtimeMu.RLock()
	defer fs.runtimeMu.RUnlock()
	if fs.factory == nil {
		return nil
	}
	return &replacementFactoryRuntime{
		dir:          fs.cfg.Dir,
		eventHistory: fs.eventHistory,
		factory:      fs.factory,
		listener:     fs.listener,
		net:          fs.net,
		runtimeCfg:   fs.runtimeCfg,
	}
}

func (fs *FactoryService) publishFactoryChangeEvent(
	ctx context.Context,
	currentRuntime *liveRuntimeHandle,
	replacement *replacementFactoryRuntime,
) {
	if replacement == nil || replacement.eventHistory == nil {
		return
	}

	payload, ok := replacementFactoryChangePayload(replacement.eventHistory.Events())
	if !ok {
		return
	}

	eventTime := factory.EnsureClock(fs.clock).Now()
	replacement.eventHistory.RecordFactoryChange(1, payload, eventTime)

	if currentRuntime == nil || currentRuntime.runtime == nil || currentRuntime.runtime.eventHistory == nil {
		return
	}

	snapshot, err := currentRuntime.runtime.factory.GetEngineStateSnapshot(ctx)
	if err != nil {
		fs.logger.Warn("read current runtime tick for factory-change event failed", zap.Error(err))
		return
	}
	currentRuntime.runtime.eventHistory.RecordFactoryChange(snapshot.TickCount+1, payload, eventTime)
}

func replacementFactoryChangePayload(events []factoryapi.FactoryEvent) (factoryapi.FactoryChangeEventPayload, bool) {
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInitialStructureRequest {
			continue
		}
		payload, err := event.Payload.AsInitialStructureRequestEventPayload()
		if err != nil {
			return factoryapi.FactoryChangeEventPayload{}, false
		}
		return factoryapi.FactoryChangeEventPayload{
			Factory:         payload.Factory,
			Metadata:        payload.Metadata,
			SourceDirectory: payload.SourceDirectory,
		}, true
	}
	return factoryapi.FactoryChangeEventPayload{}, false
}

// CreateNamedFactory persists one named-factory payload under the canonical
// layout and activates it through the idle-only runtime swap path.
func (fs *FactoryService) CreateNamedFactory(ctx context.Context, namedFactory factoryapi.Factory) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	if err := apisurface.ValidateWritableNamedFactoryName(namedFactory.Name); err != nil {
		return factoryapi.Factory{}, err
	}

	payload, err := json.Marshal(namedFactory)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("marshal factory payload: %w", err)
	}

	factoryDir, err := factoryconfig.PersistNamedFactory(rootDir, string(namedFactory.Name), payload)
	if err != nil {
		switch {
		case errors.Is(err, factoryconfig.ErrNamedFactoryAlreadyExists):
			return factoryapi.Factory{}, factoryconfig.ErrNamedFactoryAlreadyExists
		case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
			return factoryapi.Factory{}, fmt.Errorf("%w: %v", ErrInvalidNamedFactory, err)
		default:
			return factoryapi.Factory{}, err
		}
	}

	if err := fs.ActivateNamedFactory(ctx, string(namedFactory.Name)); err != nil {
		return factoryapi.Factory{}, err
	}

	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	created, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load created named factory %q: %w", namedFactory.Name, err)
	}
	return fs.serializeNamedFactory(namedFactory.Name, created, false)
}

// GetEditableFactoryDefinition returns the complete current factory definition
// with persisted version metadata for graph-editor draft saves.
func (fs *FactoryService) GetEditableFactoryDefinition(ctx context.Context) (factoryapi.EditableFactoryDefinition, error) {
	current, err := fs.GetCurrentNamedFactory(ctx)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	version, err := fs.currentFactoryDefinitionVersion(current.Name)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	return factoryapi.EditableFactoryDefinition{
		FactoryDefinition: current,
		Version:           version,
	}, nil
}

// SaveEditableFactoryDefinition replaces the current named-factory definition
// with a complete submitted Factory payload and activates the resulting runtime.
func (fs *FactoryService) SaveEditableFactoryDefinition(ctx context.Context, request factoryapi.SaveEditableFactoryDefinitionRequest) (factoryapi.EditableFactoryDefinition, error) {
	if fs == nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("factory service is required")
	}

	current, err := fs.GetCurrentNamedFactory(ctx)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return factoryapi.EditableFactoryDefinition{}, ErrCurrentNamedFactoryNotFound
	}
	if request.FactoryDefinition.Name != current.Name {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: editable save must preserve current factory name %q", ErrInvalidNamedFactoryName, current.Name)
	}
	if err := apisurface.ValidateWritableNamedFactoryName(request.FactoryDefinition.Name); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := validateEditableFactoryTopology(request.FactoryDefinition); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	payload, err := json.Marshal(request.FactoryDefinition)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("marshal editable factory payload: %w", err)
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := fs.requireFreshEditableFactoryVersion(request.BaseVersion, current.Name); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	factoryDir, err := factoryconfig.ReplaceNamedFactory(rootDir, string(request.FactoryDefinition.Name), payload)
	if err != nil {
		switch {
		case errors.Is(err, factoryconfig.ErrInvalidNamedFactory):
			return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: %v", ErrInvalidNamedFactory, err)
		default:
			return factoryapi.EditableFactoryDefinition{}, err
		}
	}

	replacement, err := fs.buildReplacementFactoryRuntime(ctx, factoryDir)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, fmt.Errorf("%w: build replacement factory %q: %v", ErrInvalidNamedFactory, request.FactoryDefinition.Name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	if err := fs.activateReplacementRuntime(ctx, rootDir, string(request.FactoryDefinition.Name), replacement); err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}

	saved, err := fs.GetCurrentNamedFactory(ctx)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	version, err := fs.currentFactoryDefinitionVersion(saved.Name)
	if err != nil {
		return factoryapi.EditableFactoryDefinition{}, err
	}
	return factoryapi.EditableFactoryDefinition{
		FactoryDefinition: saved,
		Version:           version,
	}, nil
}

func (fs *FactoryService) requireFreshEditableFactoryVersion(baseVersion *factoryapi.HybridLogicalTimestamp, name factoryapi.FactoryName) error {
	if baseVersion == nil {
		return nil
	}
	currentVersion, err := fs.currentFactoryDefinitionVersion(name)
	if err != nil {
		return err
	}
	if compareEditableFactoryVersions(*baseVersion, currentVersion) < 0 {
		return fmt.Errorf("%w: base version logical=%d physical=%s current logical=%d physical=%s",
			apisurface.ErrEditableFactoryVersionStale,
			baseVersion.Logical,
			baseVersion.Physical.UTC().Format(time.RFC3339Nano),
			currentVersion.Logical,
			currentVersion.Physical.UTC().Format(time.RFC3339Nano),
		)
	}
	return nil
}

func compareEditableFactoryVersions(left, right factoryapi.HybridLogicalTimestamp) int {
	if left.Logical < right.Logical {
		return -1
	}
	if left.Logical > right.Logical {
		return 1
	}

	leftPhysical := left.Physical.UTC()
	rightPhysical := right.Physical.UTC()
	switch {
	case leftPhysical.Before(rightPhysical):
		return -1
	case leftPhysical.After(rightPhysical):
		return 1
	default:
		return 0
	}
}

func validateEditableFactoryTopology(submitted factoryapi.Factory) error {
	var targets []factoryapi.ErrorTarget
	targets = append(targets, duplicateNameTargets("workTypes", workTypeNames(submitted.WorkTypes), "node")...)
	targets = append(targets, duplicateNameTargets("workers", workerNames(submitted.Workers), "node")...)
	targets = append(targets, duplicateNameTargets("resources", resourceNames(submitted.Resources), "node")...)
	targets = append(targets, duplicateNameTargets("workstations", workstationNames(submitted.Workstations), "node")...)
	targets = append(targets, duplicateWorkStateTargets(submitted.WorkTypes)...)
	targets = append(targets, danglingFactoryReferenceTargets(submitted)...)
	if len(targets) == 0 {
		return nil
	}
	return apisurface.NewTopologyValidationError("Factory topology contains invalid graph references.", targets)
}

func duplicateNameTargets(collection string, names []string, kind string) []factoryapi.ErrorTarget {
	seen := make(map[string]int, len(names))
	var targets []factoryapi.ErrorTarget
	for index, name := range names {
		field := fmt.Sprintf("factoryDefinition.%s[%d].name", collection, index)
		if strings.TrimSpace(name) == "" {
			targets = append(targets, editableFactoryErrorTarget("field", "", field))
			continue
		}
		if firstIndex, ok := seen[name]; ok {
			targets = append(targets,
				editableFactoryErrorTarget(kind, name, fmt.Sprintf("factoryDefinition.%s[%d].name", collection, firstIndex)),
				editableFactoryErrorTarget(kind, name, field),
			)
			continue
		}
		seen[name] = index
	}
	return targets
}

func duplicateWorkStateTargets(workTypes *[]factoryapi.WorkType) []factoryapi.ErrorTarget {
	if workTypes == nil {
		return nil
	}
	var targets []factoryapi.ErrorTarget
	for workTypeIndex, workType := range *workTypes {
		seen := make(map[string]int, len(workType.States))
		for stateIndex, state := range workType.States {
			field := fmt.Sprintf("factoryDefinition.workTypes[%d].states[%d].name", workTypeIndex, stateIndex)
			if strings.TrimSpace(state.Name) == "" {
				targets = append(targets, editableFactoryErrorTarget("field", workType.Name, field))
				continue
			}
			if firstIndex, ok := seen[state.Name]; ok {
				id := workType.Name + ":" + state.Name
				targets = append(targets,
					editableFactoryErrorTarget("node", id, fmt.Sprintf("factoryDefinition.workTypes[%d].states[%d].name", workTypeIndex, firstIndex)),
					editableFactoryErrorTarget("node", id, field),
				)
				continue
			}
			seen[state.Name] = stateIndex
		}
	}
	return targets
}

func danglingFactoryReferenceTargets(factory factoryapi.Factory) []factoryapi.ErrorTarget {
	workStates := workStateSet(factory.WorkTypes)
	workers := stringSet(workerNames(factory.Workers))
	resources := stringSet(resourceNames(factory.Resources))
	var targets []factoryapi.ErrorTarget
	if factory.Workstations == nil {
		return targets
	}
	for workstationIndex, workstation := range *factory.Workstations {
		if strings.TrimSpace(workstation.Worker) == "" || !workers[workstation.Worker] {
			targets = append(targets, editableFactoryErrorTarget("field", workstation.Name, fmt.Sprintf("factoryDefinition.workstations[%d].worker", workstationIndex)))
		}
		targets = append(targets, danglingIOTargets(workstation.Name, workstation.Inputs, workStates, fmt.Sprintf("factoryDefinition.workstations[%d].inputs", workstationIndex))...)
		targets = append(targets, danglingIOTargets(workstation.Name, workstation.Outputs, workStates, fmt.Sprintf("factoryDefinition.workstations[%d].outputs", workstationIndex))...)
		if workstation.OnContinue != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.OnContinue, workStates, fmt.Sprintf("factoryDefinition.workstations[%d].onContinue", workstationIndex))...)
		}
		if workstation.OnFailure != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.OnFailure, workStates, fmt.Sprintf("factoryDefinition.workstations[%d].onFailure", workstationIndex))...)
		}
		if workstation.OnRejection != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.OnRejection, workStates, fmt.Sprintf("factoryDefinition.workstations[%d].onRejection", workstationIndex))...)
		}
		if workstation.Resources != nil {
			for resourceIndex, resource := range *workstation.Resources {
				if strings.TrimSpace(resource.Name) == "" || !resources[resource.Name] {
					targets = append(targets, editableFactoryErrorTarget("edge", workstation.Name+"->"+resource.Name, fmt.Sprintf("factoryDefinition.workstations[%d].resources[%d].name", workstationIndex, resourceIndex)))
				}
			}
		}
	}
	return targets
}

func danglingIOTargets(workstation string, ios []factoryapi.WorkstationIO, workStates map[string]bool, fieldPrefix string) []factoryapi.ErrorTarget {
	var targets []factoryapi.ErrorTarget
	for index, io := range ios {
		id := workstation + "->" + io.WorkType + ":" + io.State
		if !workStates[io.WorkType+":"+io.State] {
			targets = append(targets, editableFactoryErrorTarget("edge", id, fmt.Sprintf("%s[%d]", fieldPrefix, index)))
		}
	}
	return targets
}

func workTypeNames(workTypes *[]factoryapi.WorkType) []string {
	if workTypes == nil {
		return nil
	}
	names := make([]string, 0, len(*workTypes))
	for _, workType := range *workTypes {
		names = append(names, workType.Name)
	}
	return names
}

func workerNames(workers *[]factoryapi.Worker) []string {
	if workers == nil {
		return nil
	}
	names := make([]string, 0, len(*workers))
	for _, worker := range *workers {
		names = append(names, worker.Name)
	}
	return names
}

func resourceNames(resources *[]factoryapi.Resource) []string {
	if resources == nil {
		return nil
	}
	names := make([]string, 0, len(*resources))
	for _, resource := range *resources {
		names = append(names, resource.Name)
	}
	return names
}

func workstationNames(workstations *[]factoryapi.Workstation) []string {
	if workstations == nil {
		return nil
	}
	names := make([]string, 0, len(*workstations))
	for _, workstation := range *workstations {
		names = append(names, workstation.Name)
	}
	return names
}

func workStateSet(workTypes *[]factoryapi.WorkType) map[string]bool {
	states := make(map[string]bool)
	if workTypes == nil {
		return states
	}
	for _, workType := range *workTypes {
		for _, state := range workType.States {
			states[workType.Name+":"+state.Name] = true
		}
	}
	return states
}

func stringSet(values []string) map[string]bool {
	set := make(map[string]bool, len(values))
	for _, value := range values {
		set[value] = true
	}
	return set
}

func editableFactoryErrorTarget(kind, id, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: kind}
	if id != "" {
		target.Id = &id
	}
	if field != "" {
		target.Field = &field
	}
	return target
}

// GetCurrentNamedFactory returns the durable current named-factory read model
// resolved entirely from the persisted pointer and canonical on-disk layout.
func (fs *FactoryService) GetCurrentNamedFactory(_ context.Context) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	name, err := factoryconfig.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			currentRuntime := fs.currentRuntimeConfig()
			if currentRuntime != nil && sameFactoryDir(currentRuntime.FactoryDir(), rootDir) {
				return fs.serializeNamedFactory(apisurface.DefaultCurrentFactoryName, currentRuntime, true)
			}
			return factoryapi.Factory{}, ErrCurrentNamedFactoryNotFound
		}
		return factoryapi.Factory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("resolve current named factory %q: %w", name, err)
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current named factory %q: %w", name, err)
	}

	return fs.serializeNamedFactory(factoryapi.FactoryName(name), current, true)
}

func (fs *FactoryService) currentFactoryDefinitionVersion(name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}

	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}

	info, err := os.Stat(filepath.Join(factoryDir, interfaces.FactoryConfigFile))
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("stat current factory definition: %w", err)
	}
	modified := info.ModTime().UTC()
	logical := modified.UnixNano()
	if logical < 0 {
		logical = 0
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  logical,
		Physical: modified,
	}, nil
}

func (fs *FactoryService) serializeNamedFactory(
	name factoryapi.FactoryName,
	current *factoryconfig.LoadedFactoryConfig,
	inlineBundledFiles bool,
) (factoryapi.Factory, error) {
	factoryCfg := current.FactoryConfig()
	if inlineBundledFiles && factoryCfg != nil {
		clonedFactoryCfg, err := factoryconfig.CloneFactoryConfig(factoryCfg)
		if err != nil {
			return factoryapi.Factory{}, fmt.Errorf("clone named factory config: %w", err)
		}
		if err := factoryconfig.ApplySupportedPortableBundledFiles(current.FactoryDir(), clonedFactoryCfg, true); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline named factory bundled files: %w", err)
		}
		factoryCfg = clonedFactoryCfg
	}
	generatedFactory, err := replay.GeneratedFactoryFromRuntimeConfig(
		current.FactoryDir(),
		factoryCfg,
		current,
		replay.WithGeneratedFactorySourceDirectory(current.FactoryDir()),
		replay.WithGeneratedFactoryWorkflowID(fs.workflowID()),
	)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("serialize current named factory: %w", err)
	}
	generatedFactory.Name = factoryapi.FactoryName(name)
	return generatedFactory, nil
}

func sameFactoryDir(left, right string) bool {
	if strings.TrimSpace(left) == "" || strings.TrimSpace(right) == "" {
		return false
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
