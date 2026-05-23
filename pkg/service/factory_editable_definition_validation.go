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
)

func (fs *FactoryService) prepareEditableFactoryDefinitionSave(
	sessionRootDir string,
	current factoryapi.Factory,
	request factoryapi.SaveEditableFactoryDefinitionRequest,
) (string, []byte, error) {
	if current.Name == apisurface.DefaultCurrentFactoryName {
		return "", nil, ErrCurrentNamedFactoryNotFound
	}
	if request.FactoryDefinition.Name != current.Name {
		return "", nil, fmt.Errorf("%w: editable save must preserve current factory name %q", ErrInvalidNamedFactoryName, current.Name)
	}
	if err := apisurface.ValidateWritableNamedFactoryName(request.FactoryDefinition.Name); err != nil {
		return "", nil, err
	}
	if err := validateEditableFactoryTopology(request.FactoryDefinition); err != nil {
		return "", nil, err
	}
	payload, err := json.Marshal(request.FactoryDefinition)
	if err != nil {
		return "", nil, fmt.Errorf("marshal editable factory payload: %w", err)
	}
	return sessionRootDir, payload, nil
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
		if workstation.Outputs != nil {
			targets = append(targets, danglingIOTargets(workstation.Name, *workstation.Outputs, workStates, fmt.Sprintf("factoryDefinition.workstations[%d].outputs", workstationIndex))...)
		}
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
