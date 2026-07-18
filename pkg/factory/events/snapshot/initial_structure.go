// Package snapshot constructs detached Factory snapshots from canonical event
// topology without depending on generated transport contracts.
package snapshot

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryresource "github.com/portpowered/infinite-you/pkg/factory/resource"
	"github.com/portpowered/infinite-you/pkg/factory/state"
)

// initialStructureFactoryDocument is the Factory-owned serialized definition
// captured by canonical initial-structure events. It intentionally models only
// fields projected by InitialStructurePayload; transport adapters may decode
// the resulting snapshot into their generated response contracts.
type initialStructureFactoryDocument struct {
	Layout          *initialStructureLayout           `json:"layout,omitempty"`
	Name            string                            `json:"name"`
	Resources       []initialStructureResource        `json:"resources"`
	SupportingFiles *initialStructureResourceManifest `json:"supportingFiles,omitempty"`
	Version         *initialStructureVersion          `json:"version,omitempty"`
	Workers         []initialStructureWorker          `json:"workers"`
	Workstations    []initialStructureWorkstation     `json:"workstations"`
	WorkTypes       []initialStructureWorkType        `json:"workTypes"`
}

type initialStructureVersion struct {
	Logical  string    `json:"logical"`
	Physical time.Time `json:"physical"`
}

type initialStructureResource struct {
	ID       *string `json:"id,omitempty"`
	Capacity int     `json:"capacity"`
	Name     string  `json:"name"`
}

type initialStructureWorkType struct {
	ID     *string                     `json:"id,omitempty"`
	Name   string                      `json:"name"`
	States []initialStructureWorkState `json:"states"`
}

type initialStructureWorkState struct {
	ID   *string `json:"id,omitempty"`
	Name string  `json:"name"`
	Type string  `json:"type"`
}

type initialStructureWorker struct {
	ID               *string                               `json:"id,omitempty"`
	ExecutorProvider *string                               `json:"executorProvider,omitempty"`
	Model            *string                               `json:"model,omitempty"`
	ModelProvider    *string                               `json:"modelProvider,omitempty"`
	Name             string                                `json:"name"`
	Resources        []initialStructureResourceRequirement `json:"resources,omitempty"`
	Type             *string                               `json:"type,omitempty"`
}

type initialStructureWorkstation struct {
	Behavior    *string                               `json:"behavior,omitempty"`
	ID          *string                               `json:"id,omitempty"`
	Inputs      []initialStructureWorkstationIO       `json:"inputs"`
	Name        string                                `json:"name"`
	OnContinue  *[]initialStructureWorkstationIO      `json:"onContinue,omitempty"`
	OnFailure   *[]initialStructureWorkstationIO      `json:"onFailure,omitempty"`
	OnRejection *[]initialStructureWorkstationIO      `json:"onRejection,omitempty"`
	Outputs     *[]initialStructureWorkstationIO      `json:"outputs,omitempty"`
	Resources   []initialStructureResourceRequirement `json:"resources,omitempty"`
	Type        *string                               `json:"type,omitempty"`
	Worker      string                                `json:"worker"`
}

type initialStructureResourceRequirement struct {
	Capacity int    `json:"capacity"`
	Name     string `json:"name"`
}

type initialStructureWorkstationIO struct {
	State    string `json:"state"`
	WorkType string `json:"workType"`
}

type initialStructureResourceManifest struct {
	BundledFiles  *[]initialStructureBundledFile  `json:"bundledFiles,omitempty"`
	RequiredTools *[]initialStructureRequiredTool `json:"requiredTools,omitempty"`
}

type initialStructureRequiredTool struct {
	Command     string    `json:"command"`
	Name        string    `json:"name"`
	Purpose     *string   `json:"purpose,omitempty"`
	VersionArgs *[]string `json:"versionArgs,omitempty"`
}

type initialStructureBundledFile struct {
	Content    initialStructureBundledFileContent `json:"content"`
	ID         *string                            `json:"id,omitempty"`
	TargetPath string                             `json:"targetPath"`
	Type       string                             `json:"type"`
}

type initialStructureBundledFileContent struct {
	Encoding string `json:"encoding"`
	Inline   string `json:"inline"`
}

type initialStructureLayout struct {
	Edges         *[]initialStructureLayoutEdge      `json:"edges,omitempty"`
	Groups        *[]initialStructureLayoutGroup     `json:"groups,omitempty"`
	Nodes         *[]initialStructureLayoutNode      `json:"nodes,omitempty"`
	Preferences   *initialStructureLayoutPreferences `json:"preferences,omitempty"`
	SchemaVersion int32                              `json:"schemaVersion"`
	Viewport      *initialStructureLayoutViewport    `json:"viewport,omitempty"`
}

type initialStructureLayoutNode struct {
	ID       string                      `json:"id"`
	Locked   *bool                       `json:"locked,omitempty"`
	Position initialStructureLayoutPoint `json:"position"`
	Size     *initialStructureLayoutSize `json:"size,omitempty"`
}

type initialStructureLayoutEdge struct {
	ID            string                         `json:"id"`
	LabelPosition *initialStructureLayoutPoint   `json:"labelPosition,omitempty"`
	Waypoints     *[]initialStructureLayoutPoint `json:"waypoints,omitempty"`
}

type initialStructureLayoutGroup struct {
	Bounds        initialStructureLayoutBounds `json:"bounds"`
	Color         *string                      `json:"color,omitempty"`
	ID            string                       `json:"id"`
	Label         *string                      `json:"label,omitempty"`
	Locked        *bool                        `json:"locked,omitempty"`
	NodeIDs       []string                     `json:"nodeIds"`
	ParentGroupID *string                      `json:"parentGroupId,omitempty"`
}

type initialStructureLayoutPoint struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
}

type initialStructureLayoutSize struct {
	Height float32 `json:"height"`
	Width  float32 `json:"width"`
}

type initialStructureLayoutBounds struct {
	Height float32 `json:"height"`
	Width  float32 `json:"width"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
}

type initialStructureLayoutViewport struct {
	X    float32 `json:"x"`
	Y    float32 `json:"y"`
	Zoom float32 `json:"zoom"`
}

type initialStructureLayoutPreferences struct {
	Direction *string `json:"direction,omitempty"`
}

// FromInitialStructure captures the Factory definition projected into the
// canonical INITIAL_STRUCTURE event payload.
func FromInitialStructure(payload interfaces.InitialStructurePayload) *interfaces.FactorySnapshot {
	snapshot, err := interfaces.NewFactorySnapshot(initialStructureFactory(payload))
	if err != nil {
		panic(fmt.Sprintf("capture initial Factory snapshot: %v", err))
	}
	return snapshot
}

func initialStructureFactory(payload interfaces.InitialStructurePayload) initialStructureFactoryDocument {
	return initialStructureFactoryDocument{
		Name:            defaultFactoryName(payload.Name),
		Version:         initialStructureFactoryVersion(payload.Version),
		Layout:          initialStructureFactoryLayout(payload.Layout),
		Resources:       initialStructureResources(payload.Resources),
		SupportingFiles: initialStructureFactoryResourceManifest(payload.ResourceManifest),
		WorkTypes:       initialStructureWorkTypes(payload.WorkTypes),
		Workers:         initialStructureWorkers(payload.Workers),
		Workstations:    initialStructureWorkstations(payload.Workstations, payload.Places),
	}
}

func defaultFactoryName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "factory"
	}
	return name
}

func initialStructureFactoryVersion(version *interfaces.FactoryVersion) *initialStructureVersion {
	if version == nil {
		return nil
	}
	return &initialStructureVersion{Logical: strconv.FormatInt(version.Logical, 10), Physical: interfaces.CanonicalEventTime(version.Physical)}
}

func initialStructureResources(resources []interfaces.FactoryResource) []initialStructureResource {
	out := make([]initialStructureResource, 0, len(resources))
	for _, resource := range resources {
		name := resource.Name
		if name == "" {
			name = resource.ID
		}
		out = append(out, initialStructureResource{ID: snapshotEntityIDPtr(resource.ID, name), Name: name, Capacity: resource.Capacity})
	}
	return out
}

func initialStructureWorkTypes(workTypes []interfaces.FactoryWorkType) []initialStructureWorkType {
	out := make([]initialStructureWorkType, 0, len(workTypes))
	for _, workType := range workTypes {
		name := workType.Name
		if name == "" {
			name = workType.ID
		}
		states := make([]initialStructureWorkState, 0, len(workType.States))
		for _, stateDef := range workType.States {
			states = append(states, initialStructureWorkState{ID: snapshotEntityIDPtr(stateDef.ID, stateDef.Value), Name: stateDef.Value, Type: initialStructureWorkStateType(stateDef.Category)})
		}
		out = append(out, initialStructureWorkType{ID: snapshotEntityIDPtr(workType.ID, name), Name: name, States: states})
	}
	return out
}

func initialStructureWorkStateType(category string) string {
	switch state.StateCategory(category) {
	case state.StateCategoryInitial:
		return "INITIAL"
	case state.StateCategoryTerminal:
		return "TERMINAL"
	case state.StateCategoryFailed:
		return "FAILED"
	default:
		return "PROCESSING"
	}
}

func initialStructureWorkers(workers []interfaces.FactoryWorker) []initialStructureWorker {
	out := make([]initialStructureWorker, 0, len(workers))
	for _, worker := range workers {
		name := worker.Name
		if name == "" {
			name = worker.ID
		}
		out = append(out, initialStructureWorker{
			ID:               snapshotEntityIDPtr(worker.ID, name),
			Name:             name,
			ExecutorProvider: snapshotStringPtr(interfaces.PublicWorkerProviderFromInternalRuntime(worker.Provider)),
			ModelProvider:    snapshotStringPtr(interfaces.PublicWorkerModelProviderFromInternalRuntime(worker.ModelProvider)),
			Model:            snapshotStringPtr(worker.Model),
			Type:             snapshotStringPtr(interfaces.PublicWorkerTypeFromInternalRuntime(worker.Config["type"])),
			Resources:        initialStructureResourceRequirements(worker.Resources),
		})
	}
	return out
}

func initialStructureWorkstations(workstations []interfaces.FactoryWorkstation, places []interfaces.FactoryPlace) []initialStructureWorkstation {
	placesByID := make(map[string]interfaces.FactoryPlace, len(places))
	for _, place := range places {
		placesByID[place.ID] = place
	}
	out := make([]initialStructureWorkstation, 0, len(workstations))
	for _, workstation := range workstations {
		name := workstation.Name
		if name == "" {
			name = workstation.ID
		}
		out = append(out, initialStructureWorkstation{
			ID: snapshotStringPtr(workstation.ID), Name: name, Worker: workstation.WorkerID,
			Type:        snapshotStringPtr(interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Config["type"], "", "")),
			Behavior:    snapshotStringPtr(string(interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKind(workstation.Kind)))),
			Inputs:      initialStructureWorkstationIOs(workstation.InputPlaceIDs, placesByID),
			Outputs:     initialStructureWorkstationIOsPtr(workstation.OutputPlaceIDs, placesByID),
			OnContinue:  initialStructureWorkstationIOsPtr(workstation.ContinuePlaceIDs, placesByID),
			OnRejection: initialStructureWorkstationIOsPtr(workstation.RejectionPlaceIDs, placesByID),
			OnFailure:   initialStructureWorkstationIOsPtr(workstation.FailurePlaceIDs, placesByID),
			Resources:   initialStructureResourceRequirements(workstation.Resources),
		})
	}
	return out
}

func initialStructureResourceRequirements(resources []factoryresource.Config) []initialStructureResourceRequirement {
	out := make([]initialStructureResourceRequirement, len(resources))
	for i, resource := range resources {
		out[i] = initialStructureResourceRequirement{Name: resource.Name, Capacity: resource.Capacity}
	}
	return out
}

func initialStructureWorkstationIOs(placeIDs []string, places map[string]interfaces.FactoryPlace) []initialStructureWorkstationIO {
	out := make([]initialStructureWorkstationIO, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		place, ok := places[placeID]
		if !ok {
			place.TypeID, place.State = splitPlaceID(placeID)
		}
		out = append(out, initialStructureWorkstationIO{WorkType: place.TypeID, State: place.State})
	}
	return out
}

func initialStructureWorkstationIOsPtr(placeIDs []string, places map[string]interfaces.FactoryPlace) *[]initialStructureWorkstationIO {
	values := initialStructureWorkstationIOs(placeIDs, places)
	if len(values) == 0 {
		return nil
	}
	return &values
}

func initialStructureFactoryResourceManifest(manifest *interfaces.PortableResourceManifestConfig) *initialStructureResourceManifest {
	if manifest == nil {
		return nil
	}
	return &initialStructureResourceManifest{RequiredTools: initialStructureRequiredTools(manifest.RequiredTools), BundledFiles: initialStructureBundledFiles(manifest.BundledFiles)}
}

func initialStructureRequiredTools(tools []interfaces.RequiredToolConfig) *[]initialStructureRequiredTool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]initialStructureRequiredTool, len(tools))
	for i, tool := range tools {
		out[i] = initialStructureRequiredTool{Name: tool.Name, Command: tool.Command, Purpose: snapshotStringPtr(tool.Purpose), VersionArgs: snapshotStringSlicePtr(tool.VersionArgs)}
	}
	return &out
}

func initialStructureBundledFiles(files []interfaces.BundledFileConfig) *[]initialStructureBundledFile {
	if len(files) == 0 {
		return nil
	}
	sorted := append([]interfaces.BundledFileConfig(nil), files...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].TargetPath < sorted[j].TargetPath })
	out := make([]initialStructureBundledFile, len(sorted))
	for i, file := range sorted {
		out[i] = initialStructureBundledFile{ID: snapshotStringPtr(interfaces.CanonicalBundledFileID(file.ID, file.TargetPath)), Type: file.Type, TargetPath: file.TargetPath, Content: initialStructureBundledFileContent{Encoding: file.Content.Encoding, Inline: file.Content.Inline}}
	}
	return &out
}

func initialStructureFactoryLayout(layout *interfaces.FactoryLayoutConfig) *initialStructureLayout {
	if layout == nil {
		return nil
	}
	return &initialStructureLayout{SchemaVersion: int32(layout.SchemaVersion), Nodes: initialStructureLayoutNodes(layout.Nodes), Edges: initialStructureLayoutEdges(layout.Edges), Groups: initialStructureLayoutGroups(layout.Groups), Viewport: projectInitialStructureLayoutViewport(layout.Viewport), Preferences: initialStructureLayoutPrefs(layout.Preferences)}
}

func initialStructureLayoutNodes(nodes []interfaces.FactoryLayoutNodeConfig) *[]initialStructureLayoutNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]initialStructureLayoutNode, len(nodes))
	for i, node := range nodes {
		out[i] = initialStructureLayoutNode{ID: node.ID, Position: initialStructurePoint(node.Position), Size: initialStructureSize(node.Size), Locked: node.Locked}
	}
	return &out
}

func initialStructureLayoutEdges(edges []interfaces.FactoryLayoutEdgeConfig) *[]initialStructureLayoutEdge {
	if len(edges) == 0 {
		return nil
	}
	out := make([]initialStructureLayoutEdge, len(edges))
	for i, edge := range edges {
		out[i] = initialStructureLayoutEdge{ID: edge.ID, Waypoints: initialStructurePoints(edge.Waypoints), LabelPosition: initialStructurePointPtr(edge.LabelPosition)}
	}
	return &out
}

func initialStructureLayoutGroups(groups []interfaces.FactoryLayoutGroupConfig) *[]initialStructureLayoutGroup {
	if len(groups) == 0 {
		return nil
	}
	out := make([]initialStructureLayoutGroup, len(groups))
	for i, group := range groups {
		out[i] = initialStructureLayoutGroup{ID: group.ID, Label: snapshotStringPtr(group.Label), Bounds: initialStructureBounds(group.Bounds), NodeIDs: append([]string(nil), group.NodeIDs...), ParentGroupID: group.ParentGroupID, Color: snapshotStringPtr(group.Color), Locked: group.Locked}
	}
	return &out
}

func projectInitialStructureLayoutViewport(viewport *interfaces.FactoryLayoutViewportConfig) *initialStructureLayoutViewport {
	if viewport == nil {
		return nil
	}
	return &initialStructureLayoutViewport{X: float32(viewport.X), Y: float32(viewport.Y), Zoom: float32(viewport.Zoom)}
}

func initialStructureLayoutPrefs(prefs *interfaces.FactoryLayoutPreferencesConfig) *initialStructureLayoutPreferences {
	if prefs == nil {
		return nil
	}
	return &initialStructureLayoutPreferences{Direction: snapshotStringPtr(prefs.Direction)}
}

func initialStructurePoint(point interfaces.FactoryLayoutPointConfig) initialStructureLayoutPoint {
	return initialStructureLayoutPoint{X: float32(point.X), Y: float32(point.Y)}
}
func initialStructurePointPtr(point *interfaces.FactoryLayoutPointConfig) *initialStructureLayoutPoint {
	if point == nil {
		return nil
	}
	value := initialStructurePoint(*point)
	return &value
}
func initialStructurePoints(points []interfaces.FactoryLayoutPointConfig) *[]initialStructureLayoutPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]initialStructureLayoutPoint, len(points))
	for i, point := range points {
		out[i] = initialStructurePoint(point)
	}
	return &out
}
func initialStructureSize(size *interfaces.FactoryLayoutSizeConfig) *initialStructureLayoutSize {
	if size == nil {
		return nil
	}
	return &initialStructureLayoutSize{Width: float32(size.Width), Height: float32(size.Height)}
}
func initialStructureBounds(bounds interfaces.FactoryLayoutBoundsConfig) initialStructureLayoutBounds {
	return initialStructureLayoutBounds{X: float32(bounds.X), Y: float32(bounds.Y), Width: float32(bounds.Width), Height: float32(bounds.Height)}
}
func snapshotStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func snapshotEntityIDPtr(id, fallbackName string) *string {
	if strings.TrimSpace(id) == "" || id == fallbackName {
		return nil
	}
	return &id
}

func snapshotStringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]string(nil), values...)
	return &cloned
}

func splitPlaceID(placeID string) (string, string) {
	before, after, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID, ""
	}
	return before, after
}
