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

	"github.com/portpowered/infinite-you/pkg/api/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"go.uber.org/zap"
)

// GetCurrentFactory returns the canonical current factory definition together
// with durable optimistic-concurrency metadata.
func (fs *FactoryService) GetCurrentFactory(ctx context.Context) (factoryapi.Factory, error) {
	return fs.GetCurrentNamedFactory(ctx)
}

// SaveCurrentFactory replaces the current named-factory definition with one
// complete canonical Factory payload and activates the resulting runtime.
func (fs *FactoryService) SaveCurrentFactory(ctx context.Context, request factoryapi.Factory) (factoryapi.Factory, error) {
	if fs == nil {
		return factoryapi.Factory{}, fmt.Errorf("factory service is required")
	}
	current, sanitized, err := fs.validateEditableFactorySave(ctx, request)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return fs.saveDefaultCurrentFactory(ctx, current, request, sanitized)
	}

	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireFreshEditableFactoryVersion(request.Version, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}

	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	nextVersion := nextEditableFactoryVersion(
		current.Version,
		factory.EnsureClock(fs.clock).Now().UTC(),
	)
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	factoryDir, err := fs.replaceEditableFactoryDefinition(rootDir, request.Name, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	replacement, err := fs.buildEditableFactoryReplacement(ctx, rootDir, factoryDir)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, request.Name, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.activateReplacementRuntime(ctx, rootDir, string(request.Name), replacement); err != nil {
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactory(ctx)
}

func (fs *FactoryService) validateEditableFactorySave(
	ctx context.Context,
	request factoryapi.Factory,
) (factoryapi.Factory, factoryapi.Factory, error) {
	current, err := fs.GetCurrentFactory(ctx)
	if err != nil {
		return factoryapi.Factory{}, factoryapi.Factory{}, err
	}
	_, sanitized, err := fs.prepareEditableFactoryDefinitionSave("", current, request)
	if err != nil {
		return factoryapi.Factory{}, factoryapi.Factory{}, err
	}
	return current, sanitized, nil
}

func (fs *FactoryService) buildEditableFactoryReplacement(
	ctx context.Context,
	rootDir string,
	factoryDir string,
) (*replacementFactoryRuntime, error) {
	sessionID := defaultFactorySessionID
	if runState := fs.currentRunState(); runState != nil && strings.TrimSpace(runState.sessionID) != "" {
		sessionID = runState.sessionID
	}
	return fs.buildReplacementFactoryRuntime(ctx, rootDir, factoryDir, sessionID)
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

const canonicalFactoryValidationRoot = "factory"

func (fs *FactoryService) prepareEditableFactoryDefinitionSave(
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
) (string, factoryapi.Factory, error) {
	if request.Name != current.Name {
		return "", factoryapi.Factory{}, fmt.Errorf("%w: editable save must preserve current factory name %q", ErrInvalidNamedFactoryName, current.Name)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
			return "", factoryapi.Factory{}, err
		}
	}
	sanitized := request
	sanitized.Version = nil
	if err := validateEditableFactoryTopology(sanitized); err != nil {
		return "", factoryapi.Factory{}, err
	}
	return sessionRootDir, sanitized, nil
}

func (fs *FactoryService) saveDefaultCurrentFactory(
	ctx context.Context,
	current factoryapi.Factory,
	request factoryapi.Factory,
	sanitized factoryapi.Factory,
) (factoryapi.Factory, error) {
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntime(ctx); err != nil {
		return factoryapi.Factory{}, err
	}
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, rootDir, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}
	nextVersion := nextEditableFactoryVersion(current.Version, factory.EnsureClock(fs.clock).Now().UTC())
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	restore, err := replaceDefaultFactoryDefinition(rootDir, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	replacement, err := fs.buildEditableFactoryReplacement(ctx, rootDir, rootDir)
	if err != nil {
		restore()
		return factoryapi.Factory{}, fmt.Errorf("%w: build replacement default factory: %w", ErrInvalidNamedFactory, err)
	}
	if err := fs.requireIdleRuntime(ctx); err != nil {
		restore()
		return factoryapi.Factory{}, err
	}
	if err := fs.activateDefaultReplacementRuntime(ctx, replacement); err != nil {
		restore()
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactory(ctx)
}

func (fs *FactoryService) saveDefaultCurrentFactoryForSession(
	ctx context.Context,
	sessionID string,
	session *liveFactorySession,
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.Factory,
	sanitized factoryapi.Factory,
) (factoryapi.Factory, error) {
	fs.activationMu.Lock()
	defer fs.activationMu.Unlock()

	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := fs.requireFreshEditableFactoryVersionAtRoot(request.Version, sessionRootDir, current.Name); err != nil {
		return factoryapi.Factory{}, err
	}
	nextVersion := nextEditableFactoryVersion(current.Version, factory.EnsureClock(fs.clock).Now().UTC())
	payload, err := marshalPersistedFactoryPayload(sanitized, nextVersion)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	restore, err := replaceDefaultFactoryDefinition(sessionRootDir, payload)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	replacement, err := fs.buildSessionEditableFactoryReplacement(ctx, sessionRootDir, sessionRootDir, sessionID, current.Name)
	if err != nil {
		restore()
		return factoryapi.Factory{}, err
	}
	if err := fs.requireIdleRuntimeForSession(ctx, sessionID); err != nil {
		restore()
		return factoryapi.Factory{}, err
	}
	if err := fs.replaceSessionRuntime(ctx, session, string(current.Name), replacement); err != nil {
		restore()
		return factoryapi.Factory{}, err
	}

	return fs.GetCurrentFactoryForSession(ctx, sessionID)
}

func (fs *FactoryService) activateDefaultReplacementRuntime(
	ctx context.Context,
	replacement *replacementFactoryRuntime,
) error {
	runState := fs.currentRunState()
	if runState == nil || runState.runtime == nil || runState.ctx == nil {
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

	replacementHandle, err := fs.startReplacementRuntime(ctx, runState.ctx, replacement, serviceMode)
	if err != nil {
		return err
	}
	fs.publishFactoryChangeEvent(ctx, runState.runtime, replacement)
	restoreCurrentSidecars = false
	fs.registerLiveSession(runState.sessionID, replacementHandle, true)
	fs.setRunState(runState.ctx, runState.sessionID, replacementHandle)
	if err := fs.stopLiveRuntime(runState.runtime); err != nil && !errors.Is(err, context.Canceled) {
		fs.logger.Warn("prior default runtime shutdown failed", zap.Error(err))
	}
	return nil
}

func (fs *FactoryService) replaceEditableFactoryDefinition(
	sessionRootDir string,
	name factoryapi.FactoryName,
	payload []byte,
) (string, error) {
	factoryDir, err := factoryconfig.ReplaceNamedFactory(sessionRootDir, string(name), payload)
	if err == nil {
		return factoryDir, nil
	}
	if errors.Is(err, factoryconfig.ErrInvalidNamedFactory) {
		return "", fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
	}
	return "", err
}

func replaceDefaultFactoryDefinition(rootDir string, payload []byte) (func(), error) {
	path := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	previous, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read default factory definition %s: %w", path, err)
	}
	if err := writeFactoryDefinitionFile(path, payload); err != nil {
		return nil, err
	}
	return func() {
		_ = writeFactoryDefinitionFile(path, previous)
	}, nil
}

func writeFactoryDefinitionFile(path string, payload []byte) error {
	staged, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".staging-")
	if err != nil {
		return fmt.Errorf("stage factory definition %s: %w", path, err)
	}
	stagedPath := staged.Name()
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(stagedPath)
		}
	}()
	if _, err := staged.Write(payload); err != nil {
		_ = staged.Close()
		return fmt.Errorf("write staged factory definition %s: %w", stagedPath, err)
	}
	if err := staged.Chmod(0o644); err != nil {
		_ = staged.Close()
		return fmt.Errorf("chmod staged factory definition %s: %w", stagedPath, err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("close staged factory definition %s: %w", stagedPath, err)
	}
	if err := os.Rename(stagedPath, path); err != nil {
		return fmt.Errorf("replace factory definition %s: %w", path, err)
	}
	committed = true
	return nil
}

func (fs *FactoryService) buildSessionEditableFactoryReplacement(
	ctx context.Context,
	sessionRootDir string,
	factoryDir string,
	sessionID string,
	name factoryapi.FactoryName,
) (*replacementFactoryRuntime, error) {
	replacement, err := fs.buildReplacementFactoryRuntime(ctx, sessionRootDir, factoryDir, sessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: build replacement factory %q: %w", ErrInvalidNamedFactory, name, err)
	}
	return replacement, nil
}

func (fs *FactoryService) requireFreshEditableFactoryVersion(baseVersion *factoryapi.HybridLogicalTimestamp, name factoryapi.FactoryName) error {
	rootDir := fs.factoryRootDir
	if rootDir == "" && fs.cfg != nil {
		rootDir = fs.cfg.Dir
	}
	return fs.requireFreshEditableFactoryVersionAtRoot(baseVersion, rootDir, name)
}

func (fs *FactoryService) requireFreshEditableFactoryVersionAtRoot(baseVersion *factoryapi.HybridLogicalTimestamp, rootDir string, name factoryapi.FactoryName) error {
	if baseVersion == nil {
		return fmt.Errorf("%w: save request must include an advanced factory version", apisurface.ErrFactoryVersionStale)
	}
	currentVersion, err := fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return err
	}
	if !isEditableFactoryVersionAdvanced(*baseVersion, currentVersion) {
		return fmt.Errorf("%w: submitted version logical=%d physical=%s must advance current logical=%d physical=%s",
			apisurface.ErrFactoryVersionStale,
			baseVersion.Logical,
			baseVersion.Physical.UTC().Format(time.RFC3339Nano),
			currentVersion.Logical,
			currentVersion.Physical.UTC().Format(time.RFC3339Nano),
		)
	}
	return nil
}

func nextEditableFactoryVersion(
	current *factoryapi.HybridLogicalTimestamp,
	now time.Time,
) factoryapi.HybridLogicalTimestamp {
	physical := now.UTC()
	logical := int64(1)
	if current != nil {
		logical = current.Logical.Int64() + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(logical),
		Physical: physical,
	}
}

func marshalPersistedFactoryPayload(
	sanitized factoryapi.Factory,
	version factoryapi.HybridLogicalTimestamp,
) ([]byte, error) {
	persisted := sanitized
	persisted.Version = &version
	payload, err := json.Marshal(persisted)
	if err != nil {
		return nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return payload, nil
}

func isEditableFactoryVersionAdvanced(candidate, current factoryapi.HybridLogicalTimestamp) bool {
	return candidate.Logical > current.Logical && candidate.Physical.UTC().After(current.Physical.UTC())
}

func validateEditableFactoryTopology(submitted factoryapi.Factory) error {
	var targets []factoryapi.ErrorTarget
	targets = append(targets, duplicateNameTargets("workTypes", workTypeNames(submitted.WorkTypes), "node")...)
	targets = append(targets, duplicateNameTargets("workers", workerNames(submitted.Workers), "node")...)
	targets = append(targets, duplicateNameTargets("resources", resourceNames(submitted.Resources), "node")...)
	targets = append(targets, duplicateNameTargets("workstations", workstationNames(submitted.Workstations), "node")...)
	targets = append(targets, duplicateWorkStateTargets(submitted.WorkTypes)...)
	targets = append(targets, danglingFactoryReferenceTargets(submitted)...)
	targets = append(targets, typeCountCollisionTargets(submitted)...)
	if len(targets) == 0 {
		return nil
	}
	return apisurface.NewTopologyValidationError("Factory topology contains invalid graph references.", targets)
}

func duplicateNameTargets(collection string, names []string, kind string) []factoryapi.ErrorTarget {
	seen := make(map[string]int, len(names))
	var targets []factoryapi.ErrorTarget
	for index, name := range names {
		field := fmt.Sprintf("%s.%s[%d].name", canonicalFactoryValidationRoot, collection, index)
		if strings.TrimSpace(name) == "" {
			targets = append(targets, editableFactoryErrorTarget("field", "", field))
			continue
		}
		if firstIndex, ok := seen[name]; ok {
			targets = append(targets,
				editableFactoryErrorTarget(kind, name, fmt.Sprintf("%s.%s[%d].name", canonicalFactoryValidationRoot, collection, firstIndex)),
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
			field := fmt.Sprintf("%s.workTypes[%d].states[%d].name", canonicalFactoryValidationRoot, workTypeIndex, stateIndex)
			if strings.TrimSpace(state.Name) == "" {
				targets = append(targets, editableFactoryErrorTarget("field", workType.Name, field))
				continue
			}
			if firstIndex, ok := seen[state.Name]; ok {
				id := workType.Name + ":" + state.Name
				targets = append(targets,
					editableFactoryErrorTarget("node", id, fmt.Sprintf("%s.workTypes[%d].states[%d].name", canonicalFactoryValidationRoot, workTypeIndex, firstIndex)),
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
			targets = append(targets, editableFactoryErrorTarget("field", workstation.Name, fmt.Sprintf("%s.workstations[%d].worker", canonicalFactoryValidationRoot, workstationIndex)))
		}
		targets = append(targets, danglingIOTargets(workstation.Name, workstation.Inputs, workStates, fmt.Sprintf("%s.workstations[%d].inputs", canonicalFactoryValidationRoot, workstationIndex))...)
		if workstation.Outputs != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.Outputs, workStates, fmt.Sprintf("%s.workstations[%d].outputs", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.OnContinue != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.OnContinue, workStates, fmt.Sprintf("%s.workstations[%d].onContinue", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.OnFailure != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.OnFailure, workStates, fmt.Sprintf("%s.workstations[%d].onFailure", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.OnRejection != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.OnRejection, workStates, fmt.Sprintf("%s.workstations[%d].onRejection", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.Resources != nil {
			for resourceIndex, resource := range *workstation.Resources {
				if strings.TrimSpace(resource.Name) == "" || !resources[resource.Name] {
					targets = append(targets, editableFactoryErrorTarget("edge", workstation.Name+"->"+resource.Name, fmt.Sprintf("%s.workstations[%d].resources[%d].name", canonicalFactoryValidationRoot, workstationIndex, resourceIndex)))
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

func typeCountCollisionTargets(factory factoryapi.Factory) []factoryapi.ErrorTarget {
	if factory.Workstations == nil {
		return nil
	}
	var targets []factoryapi.ErrorTarget
	for workstationIndex, workstation := range *factory.Workstations {
		inputCounts := ioWorkTypeCounts(workstation.Inputs)
		if len(inputCounts) == 0 {
			continue
		}
		if workstation.Outputs != nil {
			targets = append(targets, typeCountRouteCollisionTargets(workstation.Name, inputCounts, *workstation.Outputs, fmt.Sprintf("%s.workstations[%d].outputs", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.OnContinue != nil {
			targets = append(targets, typeCountRouteCollisionTargets(workstation.Name, inputCounts, *workstation.OnContinue, fmt.Sprintf("%s.workstations[%d].onContinue", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.OnRejection != nil {
			targets = append(targets, typeCountRouteCollisionTargets(workstation.Name, inputCounts, *workstation.OnRejection, fmt.Sprintf("%s.workstations[%d].onRejection", canonicalFactoryValidationRoot, workstationIndex))...)
		}
		if workstation.OnFailure != nil {
			targets = append(targets, typeCountRouteCollisionTargets(workstation.Name, inputCounts, *workstation.OnFailure, fmt.Sprintf("%s.workstations[%d].onFailure", canonicalFactoryValidationRoot, workstationIndex))...)
		}
	}
	return targets
}

func ioWorkTypeCounts(ios []factoryapi.WorkstationIO) map[string]int {
	counts := make(map[string]int)
	for _, io := range ios {
		if strings.TrimSpace(io.WorkType) == "" {
			continue
		}
		counts[io.WorkType]++
	}
	return counts
}

func typeCountRouteCollisionTargets(
	workstation string,
	inputCounts map[string]int,
	routes []factoryapi.WorkstationIO,
	fieldPrefix string,
) []factoryapi.ErrorTarget {
	routeCounts := ioWorkTypeCounts(routes)
	var targets []factoryapi.ErrorTarget
	for workType, inputCount := range inputCounts {
		routeCount := routeCounts[workType]
		if routeCount == 0 || routeCount == inputCount {
			continue
		}
		for routeIndex, route := range routes {
			if route.WorkType != workType {
				continue
			}
			targets = append(targets, editableFactoryErrorTarget("edge", workstation+"->"+route.WorkType+":"+route.State, fmt.Sprintf("%s[%d]", fieldPrefix, routeIndex)))
		}
	}
	return targets
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
			return factoryapi.Factory{}, fmt.Errorf("%w: %w", ErrInvalidNamedFactory, err)
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
			return factoryapi.Factory{}, ErrCurrentFactoryNotFound
		}
		return factoryapi.Factory{}, fmt.Errorf("read current factory pointer: %w", err)
	}
	factoryDir, err := factoryconfig.ResolveNamedFactoryDir(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("resolve current factory %q: %w", name, err)
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.Factory{}, fmt.Errorf("load current factory %q: %w", name, err)
	}

	return fs.serializeNamedFactory(factoryapi.FactoryName(name), current, true)
}

func (fs *FactoryService) currentFactoryDefinitionVersionAtRoot(rootDir string, name factoryapi.FactoryName) (factoryapi.HybridLogicalTimestamp, error) {
	factoryDir := rootDir
	if name != apisurface.DefaultCurrentFactoryName {
		resolved, err := factoryconfig.ResolveNamedFactoryDir(rootDir, string(name))
		if err != nil {
			return factoryapi.HybridLogicalTimestamp{}, err
		}
		factoryDir = resolved
	}
	var workstationLoader factoryconfig.WorkstationLoader
	if fs.cfg != nil {
		workstationLoader = fs.cfg.WorkstationLoader
	}
	current, err := factoryconfig.LoadRuntimeConfig(factoryDir, workstationLoader)
	if err != nil {
		return factoryapi.HybridLogicalTimestamp{}, fmt.Errorf("load current factory definition: %w", err)
	}
	if current.FactoryConfig().Version != nil {
		version := current.FactoryConfig().Version
		return factoryapi.HybridLogicalTimestamp{
			Logical:  apitypes.Int64String(version.Logical),
			Physical: version.Physical.UTC(),
		}, nil
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
		Logical:  apitypes.Int64String(logical),
		Physical: modified,
	}, nil
}

func (fs *FactoryService) withCurrentFactoryVersion(
	rootDir string,
	name factoryapi.FactoryName,
	serialized factoryapi.Factory,
) (factoryapi.Factory, error) {
	version, err := fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	serialized.Version = &version
	return serialized, nil
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
		if err := factoryconfig.ApplySharedFactoryStarterWork(current.FactoryDir(), clonedFactoryCfg); err != nil {
			return factoryapi.Factory{}, fmt.Errorf("inline shared factory starter work: %w", err)
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
		return factoryapi.Factory{}, fmt.Errorf("serialize current factory: %w", err)
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

const factorySessionValidationTargetKind = "factory-session-validation"

const (
	factorySessionValidationReasonRequired       = "required"
	factorySessionValidationReasonMissing        = "missing"
	factorySessionValidationReasonNotDirectory   = "not_directory"
	factorySessionValidationReasonNotRunnable    = "not_runnable"
	factorySessionValidationReasonTargetNotFound = "target_not_found"
	factorySessionValidationReasonUnreadable     = "unreadable"
)

type factorySessionValidationError struct {
	reason string
	field  string
	err    error
}

func (e *factorySessionValidationError) Error() string {
	if e == nil || e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e *factorySessionValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

func (e *factorySessionValidationError) ErrorTargets() []factoryapi.ErrorTarget {
	if e == nil {
		return nil
	}
	return []factoryapi.ErrorTarget{factorySessionValidationErrorTarget(e.reason, e.field)}
}

func newFactorySessionValidationError(reason string, field string, err error) error {
	if err == nil {
		return nil
	}
	return &factorySessionValidationError{
		reason: reason,
		field:  field,
		err:    err,
	}
}

func factorySessionValidationErrorTarget(reason string, field string) factoryapi.ErrorTarget {
	target := factoryapi.ErrorTarget{Kind: factorySessionValidationTargetKind}
	if reason != "" {
		target.Id = &reason
	}
	if field != "" {
		target.Field = &field
	}
	return target
}
