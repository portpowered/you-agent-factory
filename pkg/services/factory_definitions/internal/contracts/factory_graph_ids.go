package factorycontracts

import (
	"strings"
)

// CanonicalFactoryGraphEntityID resolves the durable public identifier used by
// the graph contract, preferring explicit ids while preserving legacy name-keyed
// compatibility for factories that have not materialized ids yet.
func CanonicalFactoryGraphEntityID(explicitID, fallbackName string) string {
	normalizedID := strings.TrimSpace(explicitID)
	if normalizedID != "" {
		return normalizedID
	}
	return fallbackName
}

func CanonicalFactoryGraphResourceID(resource ResourceConfig) string {
	return CanonicalFactoryGraphEntityID(resource.ID, resource.Name)
}

func CanonicalFactoryGraphWorkerID(worker Config) string {
	return CanonicalFactoryGraphEntityID(worker.ID, worker.Name)
}

func CanonicalFactoryGraphWorkTypeID(workType WorkTypeConfig) string {
	return CanonicalFactoryGraphEntityID(workType.ID, workType.Name)
}

func CanonicalFactoryGraphWorkstationID(workstation FactoryWorkstationConfig) string {
	return CanonicalFactoryGraphEntityID(workstation.ID, workstation.Name)
}

func CanonicalFactoryGraphWorkStateID(workType WorkTypeConfig, state StateConfig) string {
	return CanonicalFactoryGraphEntityID(workType.ID, workType.Name) +
		":" +
		CanonicalFactoryGraphEntityID(state.ID, state.Name)
}

func CanonicalFactoryGraphNodeID(kind, subjectID string) string {
	return kind + ":" + subjectID
}

func CanonicalFactoryGraphEdgeID(kind, sourceNodeID, targetNodeID string) string {
	return kind + ":" + sourceNodeID + "->" + targetNodeID
}

func CanonicalBundledFileID(explicitID, targetPath string) string {
	normalizedID := strings.TrimSpace(explicitID)
	if normalizedID != "" {
		return normalizedID
	}
	return targetPath
}

func CanonicalBundledFileGraphNodeKind(fileType string) string {
	switch strings.TrimSpace(fileType) {
	case BundledFileTypeDoc:
		return "doc"
	case BundledFileTypeScript:
		return "script"
	case BundledFileTypeInput:
		return "input"
	case BundledFileTypeRootHelper:
		return "root-helper"
	default:
		return ""
	}
}

func CanonicalBundledFileGraphNodeID(file BundledFileConfig) string {
	kind := CanonicalBundledFileGraphNodeKind(file.Type)
	if kind == "" {
		return ""
	}
	id := CanonicalBundledFileID(file.ID, file.TargetPath)
	if strings.TrimSpace(id) == "" {
		return ""
	}
	return CanonicalFactoryGraphNodeID(kind, id)
}

func IsBundledFileGraphNodeID(nodeID string) bool {
	switch {
	case strings.HasPrefix(nodeID, "doc:"):
		return true
	case strings.HasPrefix(nodeID, "script:"):
		return true
	case strings.HasPrefix(nodeID, "input:"):
		return true
	case strings.HasPrefix(nodeID, "root-helper:"):
		return true
	default:
		return false
	}
}

// SupportedFactoryLayoutSchemaVersion is the only portable layout contract version
// validated against pending factory graph topology.
const SupportedFactoryLayoutSchemaVersion = 1

// PendingFactoryGraphTopology indexes canonical graph node and edge ids derived
// from a pending factory definition. Layout validation uses these indexes as its
// source of truth for recoverable reference checks.
type PendingFactoryGraphTopology struct {
	NodeIDs map[string]struct{}
	EdgeIDs map[string]struct{}
}

// BuildPendingFactoryGraphTopology derives pending graph node and edge membership
// from factory topology using the same canonical id rules as the graph editor.
func BuildPendingFactoryGraphTopology(cfg *FactoryConfig) PendingFactoryGraphTopology {
	topology := PendingFactoryGraphTopology{
		NodeIDs: make(map[string]struct{}),
		EdgeIDs: make(map[string]struct{}),
	}
	if cfg == nil {
		return topology
	}

	index := buildFactoryGraphEntityIndex(cfg)
	if cfg.ResourceManifest != nil {
		for _, bundledFile := range cfg.ResourceManifest.BundledFiles {
			addPendingBundledFileGraphNodes(&topology, bundledFile)
		}
	}
	for _, resource := range cfg.Resources {
		addPendingFactoryGraphNode(&topology, "resource", CanonicalFactoryGraphResourceID(resource))
	}
	for _, worker := range cfg.Workers {
		workerNodeID := addPendingFactoryGraphNode(&topology, "worker", CanonicalFactoryGraphWorkerID(worker))
		for _, resource := range worker.Resources {
			resourceID := resource.Name
			if explicitID, ok := index.resourceIDsByName[resource.Name]; ok {
				resourceID = explicitID
			}
			resourceNodeID := addPendingFactoryGraphNode(&topology, "resource", resourceID)
			addPendingFactoryGraphEdge(&topology, "worker-resource", resourceNodeID, workerNodeID)
		}
	}
	for _, workType := range cfg.WorkTypes {
		workTypeNodeID := addPendingFactoryGraphNode(&topology, "work-type", CanonicalFactoryGraphWorkTypeID(workType))
		for _, state := range workType.States {
			workStateNodeID := addPendingFactoryGraphNode(
				&topology,
				"work-state",
				CanonicalFactoryGraphWorkStateID(workType, state),
			)
			addPendingFactoryGraphEdge(&topology, "work-type-state", workTypeNodeID, workStateNodeID)
		}
	}
	for _, workstation := range cfg.Workstations {
		workstationNodeID := addPendingFactoryGraphNode(
			&topology,
			"workstation",
			CanonicalFactoryGraphWorkstationID(workstation),
		)
		appendPendingWorkstationWorkerEdge(&topology, index, workstation, workstationNodeID)
		appendPendingWorkstationResourceEdges(&topology, workstation, workstationNodeID)
		appendPendingWorkstationIOEdges(&topology, cfg.WorkTypes, "workstation-input", workstation.Inputs, workstationNodeID, true)
		appendPendingWorkstationIOEdges(&topology, cfg.WorkTypes, "workstation-output", workstation.Outputs, workstationNodeID, false)
		appendPendingWorkstationIOEdges(&topology, cfg.WorkTypes, "workstation-on-continue", workstation.OnContinue, workstationNodeID, false)
		appendPendingWorkstationIOEdges(&topology, cfg.WorkTypes, "workstation-on-failure", workstation.OnFailure, workstationNodeID, false)
		appendPendingWorkstationIOEdges(&topology, cfg.WorkTypes, "workstation-on-rejection", workstation.OnRejection, workstationNodeID, false)
	}
	return topology
}

func addPendingBundledFileGraphNodes(topology *PendingFactoryGraphTopology, bundledFile BundledFileConfig) {
	if nodeID := CanonicalBundledFileGraphNodeID(bundledFile); nodeID != "" {
		topology.NodeIDs[nodeID] = struct{}{}
	}

	// Older graph-editor drafts represented every bundled file as a doc node.
	// Keep accepting those persisted layout references while canonical callers
	// migrate SCRIPT, INPUT, and ROOT_HELPER files to their typed node kinds.
	targetPath := strings.TrimSpace(bundledFile.TargetPath)
	if bundledFile.Type == BundledFileTypeDoc || targetPath == "" {
		return
	}
	topology.NodeIDs[CanonicalFactoryGraphNodeID("doc", targetPath)] = struct{}{}
}

type factoryGraphEntityIndex struct {
	resourceIDsByName          map[string]string
	workerIDsByName            map[string]string
	workTypeIDsByName          map[string]string
	workStateIDsByWorkTypeName map[string]map[string]string
}

func buildFactoryGraphEntityIndex(cfg *FactoryConfig) factoryGraphEntityIndex {
	index := factoryGraphEntityIndex{
		resourceIDsByName:          make(map[string]string),
		workerIDsByName:            make(map[string]string),
		workTypeIDsByName:          make(map[string]string),
		workStateIDsByWorkTypeName: make(map[string]map[string]string),
	}
	for _, resource := range cfg.Resources {
		if id := CanonicalFactoryGraphEntityID(resource.ID, resource.Name); id != resource.Name {
			index.resourceIDsByName[resource.Name] = id
		}
	}
	for _, worker := range cfg.Workers {
		if id := CanonicalFactoryGraphEntityID(worker.ID, worker.Name); id != worker.Name {
			index.workerIDsByName[worker.Name] = id
		}
	}
	for _, workType := range cfg.WorkTypes {
		if id := CanonicalFactoryGraphEntityID(workType.ID, workType.Name); id != workType.Name {
			index.workTypeIDsByName[workType.Name] = id
		}
		stateIDsByName := make(map[string]string)
		for _, state := range workType.States {
			if id := CanonicalFactoryGraphEntityID(state.ID, state.Name); id != state.Name {
				stateIDsByName[state.Name] = id
			}
		}
		index.workStateIDsByWorkTypeName[workType.Name] = stateIDsByName
	}
	return index
}

func addPendingFactoryGraphNode(topology *PendingFactoryGraphTopology, kind, entityID string) string {
	nodeID := CanonicalFactoryGraphNodeID(kind, entityID)
	topology.NodeIDs[nodeID] = struct{}{}
	return nodeID
}

func addPendingFactoryGraphEdge(topology *PendingFactoryGraphTopology, kind, sourceNodeID, targetNodeID string) {
	edgeID := CanonicalFactoryGraphEdgeID(kind, sourceNodeID, targetNodeID)
	topology.EdgeIDs[edgeID] = struct{}{}
}

func appendPendingWorkstationWorkerEdge(
	topology *PendingFactoryGraphTopology,
	index factoryGraphEntityIndex,
	workstation FactoryWorkstationConfig,
	workstationNodeID string,
) {
	workerName := workstation.WorkerTypeName
	if workerName == "" {
		return
	}
	workerID := workerName
	if explicitID, ok := index.workerIDsByName[workerName]; ok {
		workerID = explicitID
	}
	workerNodeID := addPendingFactoryGraphNode(topology, "worker", workerID)
	addPendingFactoryGraphEdge(topology, "worker-assignment", workerNodeID, workstationNodeID)
}

func appendPendingWorkstationResourceEdges(
	topology *PendingFactoryGraphTopology,
	workstation FactoryWorkstationConfig,
	workstationNodeID string,
) {
	for _, resource := range workstation.Resources {
		resourceNodeID := addPendingFactoryGraphNode(topology, "resource", CanonicalFactoryGraphResourceID(resource))
		addPendingFactoryGraphEdge(topology, "workstation-resource", resourceNodeID, workstationNodeID)
	}
}

func appendPendingWorkstationIOEdges(
	topology *PendingFactoryGraphTopology,
	workTypes []WorkTypeConfig,
	kind string,
	ios []IOConfig,
	workstationNodeID string,
	input bool,
) {
	for _, io := range ios {
		workType := findWorkTypeByName(workTypes, io.WorkTypeName)
		if workType == nil {
			continue
		}
		state := findWorkStateByName(*workType, io.StateName)
		if state == nil {
			continue
		}
		workStateNodeID := addPendingFactoryGraphNode(
			topology,
			"work-state",
			CanonicalFactoryGraphWorkStateID(*workType, *state),
		)
		if input {
			addPendingFactoryGraphEdge(topology, kind, workStateNodeID, workstationNodeID)
			continue
		}
		addPendingFactoryGraphEdge(topology, kind, workstationNodeID, workStateNodeID)
	}
}

func findWorkTypeByName(workTypes []WorkTypeConfig, name string) *WorkTypeConfig {
	for i := range workTypes {
		if workTypes[i].Name == name {
			return &workTypes[i]
		}
	}
	return nil
}

func findWorkStateByName(workType WorkTypeConfig, name string) *StateConfig {
	for i := range workType.States {
		if workType.States[i].Name == name {
			return &workType.States[i]
		}
	}
	return nil
}
