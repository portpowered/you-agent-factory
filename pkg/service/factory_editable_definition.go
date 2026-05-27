package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
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
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return "", factoryapi.Factory{}, ErrCurrentFactoryNotFound
	}
	if request.Name != current.Name {
		return "", factoryapi.Factory{}, fmt.Errorf("%w: editable save must preserve current factory name %q", ErrInvalidNamedFactoryName, current.Name)
	}
	if err := apisurface.ValidateWritableNamedFactoryName(request.Name); err != nil {
		return "", factoryapi.Factory{}, err
	}
	sanitized := request
	sanitized.Version = nil
	if err := validateEditableFactoryTopology(sanitized); err != nil {
		return "", factoryapi.Factory{}, err
	}
	return sessionRootDir, sanitized, nil
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
		return nil
	}
	currentVersion, err := fs.currentFactoryDefinitionVersionAtRoot(rootDir, name)
	if err != nil {
		return err
	}
	if compareEditableFactoryVersions(*baseVersion, currentVersion) < 0 {
		return fmt.Errorf("%w: base version logical=%d physical=%s current logical=%d physical=%s",
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
		logical = current.Logical + 1
		if !physical.After(current.Physical.UTC()) {
			physical = current.Physical.UTC().Add(time.Nanosecond)
		}
	}
	return factoryapi.HybridLogicalTimestamp{
		Logical:  logical,
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
