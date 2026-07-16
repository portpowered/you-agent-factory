package events

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/work"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func cloneFactoryEvents(events []interfaces.FactoryEvent) []interfaces.FactoryEvent {
	clones := make([]interfaces.FactoryEvent, len(events))
	for index, event := range events {
		clones[index] = event.Clone()
	}
	return clones
}

// RecordInferenceEvent appends worker-owned provider facts to canonical
// history while Factory owns the envelope, vocabulary, and ordering.
func (h *FactoryEventHistory) RecordInferenceEvent(event workerexecution.InferenceEvent) {
	if h == nil || strings.TrimSpace(event.ID) == "" || strings.TrimSpace(event.DispatchID) == "" {
		return
	}
	eventType, payload := inferenceFactoryEventPayload(event)
	if eventType == "" || payload == nil {
		return
	}
	h.appendEvent(domainFactoryEvent(
		eventType,
		event.ID,
		interfaces.FactoryEventContext{
			Tick:       event.Tick,
			EventTime:  interfaces.CanonicalEventTime(event.EventTime),
			DispatchID: stringPtrIfNotEmpty(event.DispatchID),
			RequestID:  stringPtrIfNotEmpty(event.RequestID),
			TraceIDs:   stringSlicePtr(event.TraceIDs),
			WorkIDs:    stringSlicePtr(event.WorkIDs),
		},
		payload,
	))
}

func inferenceFactoryEventPayload(event workerexecution.InferenceEvent) (interfaces.FactoryEventType, any) {
	switch event.Kind {
	case workerexecution.InferenceEventKindRequest:
		if event.Request != nil && event.Response == nil {
			return interfaces.FactoryEventTypeInferenceRequest, *event.Request
		}
	case workerexecution.InferenceEventKindResponse:
		if event.Response != nil && event.Request == nil {
			return interfaces.FactoryEventTypeInferenceResponse, *event.Response
		}
	}
	return "", nil
}

func generatedFactory(payload interfaces.InitialStructurePayload) factoryapi.Factory {
	resources := generatedResources(payload.Resources)
	workTypes := generatedWorkTypes(payload.WorkTypes)
	workers := generatedWorkers(payload.Workers)
	workstations := generatedWorkstations(payload.Workstations, payload.Places)

	return factoryapi.Factory{
		Name:            generatedFactoryName(payload.Name),
		Version:         generatedFactoryVersion(payload.Version),
		Layout:          generatedFactoryLayout(payload.Layout),
		Resources:       slicePtr(resources),
		SupportingFiles: generatedFactoryResourceManifest(payload.ResourceManifest),
		WorkTypes:       slicePtr(workTypes),
		Workers:         slicePtr(workers),
		Workstations:    slicePtr(workstations),
	}
}

func factorySnapshotFromInitialStructure(payload interfaces.InitialStructurePayload) *interfaces.FactorySnapshot {
	snapshot, err := interfaces.NewFactorySnapshot(generatedFactory(payload))
	if err != nil {
		panic(fmt.Sprintf("capture initial Factory snapshot: %v", err))
	}
	return snapshot
}

func generatedFactoryName(name string) factoryapi.FactoryName {
	if strings.TrimSpace(name) == "" {
		return "factory"
	}
	return factoryapi.FactoryName(name)
}

func generatedFactoryVersion(version *interfaces.FactoryVersion) *factoryapi.HybridLogicalTimestamp {
	if version == nil {
		return nil
	}
	return &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(version.Logical),
		Physical: interfaces.CanonicalEventTime(version.Physical),
	}
}

func generatedFactoryResourceManifest(manifest *interfaces.PortableResourceManifestConfig) *factoryapi.ResourceManifest {
	if manifest == nil {
		return nil
	}
	return &factoryapi.ResourceManifest{
		RequiredTools: generatedFactoryRequiredTools(manifest.RequiredTools),
		BundledFiles:  generatedFactoryBundledFiles(manifest.BundledFiles),
	}
}

func generatedFactoryRequiredTools(requiredTools []interfaces.RequiredToolConfig) *[]factoryapi.RequiredTool {
	if len(requiredTools) == 0 {
		return nil
	}
	out := make([]factoryapi.RequiredTool, len(requiredTools))
	for i, tool := range requiredTools {
		out[i] = factoryapi.RequiredTool{
			Name:        tool.Name,
			Command:     tool.Command,
			Purpose:     stringPtrIfNotEmpty(tool.Purpose),
			VersionArgs: stringSlicePtr(tool.VersionArgs),
		}
	}
	return &out
}

func generatedFactoryBundledFiles(bundledFiles []interfaces.BundledFileConfig) *[]factoryapi.BundledFile {
	if len(bundledFiles) == 0 {
		return nil
	}
	sorted := append([]interfaces.BundledFileConfig(nil), bundledFiles...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TargetPath < sorted[j].TargetPath
	})
	out := make([]factoryapi.BundledFile, len(sorted))
	for i, file := range sorted {
		out[i] = factoryapi.BundledFile{
			Id:         stringPtrIfNotEmpty(interfaces.CanonicalBundledFileID(file.ID, file.TargetPath)),
			Type:       factoryapi.BundledFileType(file.Type),
			TargetPath: file.TargetPath,
			Content: factoryapi.BundledFileContent{
				Encoding: factoryapi.BundledFileContentEncoding(file.Content.Encoding),
				Inline:   file.Content.Inline,
			},
		}
	}
	return &out
}

func generatedFactoryLayout(layout *interfaces.FactoryLayoutConfig) *factoryapi.FactoryLayout {
	if layout == nil {
		return nil
	}
	return &factoryapi.FactoryLayout{
		SchemaVersion: int32(layout.SchemaVersion),
		Nodes:         generatedFactoryLayoutNodes(layout.Nodes),
		Edges:         generatedFactoryLayoutEdges(layout.Edges),
		Groups:        generatedFactoryLayoutGroups(layout.Groups),
		Viewport:      generatedFactoryLayoutViewport(layout.Viewport),
		Preferences:   generatedFactoryLayoutPreferences(layout.Preferences),
	}
}

func generatedFactoryLayoutNodes(nodes []interfaces.FactoryLayoutNodeConfig) *[]factoryapi.FactoryLayoutNode {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryLayoutNode, len(nodes))
	for i, node := range nodes {
		out[i] = factoryapi.FactoryLayoutNode{
			Id:       node.ID,
			Position: generatedFactoryLayoutPoint(node.Position),
			Size:     generatedFactoryLayoutSize(node.Size),
			Locked:   node.Locked,
		}
	}
	return &out
}

func generatedFactoryLayoutEdges(edges []interfaces.FactoryLayoutEdgeConfig) *[]factoryapi.FactoryLayoutEdge {
	if len(edges) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryLayoutEdge, len(edges))
	for i, edge := range edges {
		out[i] = factoryapi.FactoryLayoutEdge{
			Id:            edge.ID,
			Waypoints:     generatedFactoryLayoutPoints(edge.Waypoints),
			LabelPosition: generatedFactoryLayoutPointPtr(edge.LabelPosition),
		}
	}
	return &out
}

func generatedFactoryLayoutGroups(groups []interfaces.FactoryLayoutGroupConfig) *[]factoryapi.FactoryLayoutGroup {
	if len(groups) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryLayoutGroup, len(groups))
	for i, group := range groups {
		out[i] = factoryapi.FactoryLayoutGroup{
			Id:            group.ID,
			Label:         stringPtrIfNotEmpty(group.Label),
			Bounds:        generatedFactoryLayoutBounds(group.Bounds),
			NodeIds:       append([]string(nil), group.NodeIDs...),
			ParentGroupId: group.ParentGroupID,
			Color:         stringPtrIfNotEmpty(group.Color),
			Locked:        group.Locked,
		}
	}
	return &out
}

func generatedFactoryLayoutViewport(viewport *interfaces.FactoryLayoutViewportConfig) *factoryapi.FactoryLayoutViewport {
	if viewport == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutViewport{
		X:    float32(viewport.X),
		Y:    float32(viewport.Y),
		Zoom: float32(viewport.Zoom),
	}
}

func generatedFactoryLayoutPreferences(preferences *interfaces.FactoryLayoutPreferencesConfig) *factoryapi.FactoryLayoutPreferences {
	if preferences == nil {
		return nil
	}
	out := &factoryapi.FactoryLayoutPreferences{}
	if strings.TrimSpace(preferences.Direction) != "" {
		direction := factoryapi.FactoryLayoutPreferencesDirection(preferences.Direction)
		out.Direction = &direction
	}
	return out
}

func generatedFactoryLayoutPoint(point interfaces.FactoryLayoutPointConfig) factoryapi.FactoryLayoutPoint {
	return factoryapi.FactoryLayoutPoint{
		X: float32(point.X),
		Y: float32(point.Y),
	}
}

func generatedFactoryLayoutPointPtr(point *interfaces.FactoryLayoutPointConfig) *factoryapi.FactoryLayoutPoint {
	if point == nil {
		return nil
	}
	out := generatedFactoryLayoutPoint(*point)
	return &out
}

func generatedFactoryLayoutPoints(points []interfaces.FactoryLayoutPointConfig) *[]factoryapi.FactoryLayoutPoint {
	if len(points) == 0 {
		return nil
	}
	out := make([]factoryapi.FactoryLayoutPoint, len(points))
	for i, point := range points {
		out[i] = generatedFactoryLayoutPoint(point)
	}
	return &out
}

func generatedFactoryLayoutSize(size *interfaces.FactoryLayoutSizeConfig) *factoryapi.FactoryLayoutSize {
	if size == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutSize{
		Width:  float32(size.Width),
		Height: float32(size.Height),
	}
}

func generatedFactoryLayoutBounds(bounds interfaces.FactoryLayoutBoundsConfig) factoryapi.FactoryLayoutBounds {
	return factoryapi.FactoryLayoutBounds{
		X:      float32(bounds.X),
		Y:      float32(bounds.Y),
		Width:  float32(bounds.Width),
		Height: float32(bounds.Height),
	}
}

func generatedResources(resources []interfaces.FactoryResource) []factoryapi.Resource {
	out := make([]factoryapi.Resource, 0, len(resources))
	for _, resource := range resources {
		name := resource.Name
		if name == "" {
			name = resource.ID
		}
		out = append(out, factoryapi.Resource{Name: name, Capacity: resource.Capacity})
	}
	return out
}

func generatedWorkTypes(workTypes []interfaces.FactoryWorkType) []factoryapi.WorkType {
	out := make([]factoryapi.WorkType, 0, len(workTypes))
	for _, workType := range workTypes {
		name := workType.Name
		if name == "" {
			name = workType.ID
		}
		states := make([]factoryapi.WorkState, 0, len(workType.States))
		for _, stateDef := range workType.States {
			states = append(states, factoryapi.WorkState{
				Name: stateDef.Value,
				Type: generatedWorkStateType(stateDef.Category),
			})
		}
		out = append(out, factoryapi.WorkType{Name: name, States: states})
	}
	return out
}

func generatedWorkStateType(category string) factoryapi.WorkStateType {
	switch state.StateCategory(category) {
	case state.StateCategoryInitial:
		return factoryapi.WorkStateTypeINITIAL
	case state.StateCategoryTerminal:
		return factoryapi.WorkStateTypeTERMINAL
	case state.StateCategoryFailed:
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func inferredGeneratedWorkStateType(name string) factoryapi.WorkStateType {
	switch name {
	case "init":
		return factoryapi.WorkStateTypeINITIAL
	case "complete", "done":
		return factoryapi.WorkStateTypeTERMINAL
	case "failed":
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func generatedWorkers(workers []interfaces.FactoryWorker) []factoryapi.Worker {
	out := make([]factoryapi.Worker, 0, len(workers))
	for _, worker := range workers {
		name := worker.Name
		if name == "" {
			name = worker.ID
		}
		out = append(out, factoryapi.Worker{
			Name: name,
			ExecutorProvider: generatedEnumPtr(worker.Provider, func(value string) factoryapi.WorkerProvider {
				return factoryapi.WorkerProvider(interfaces.PublicWorkerProviderFromInternalRuntime(value))
			}),
			ModelProvider: generatedEnumPtr(worker.ModelProvider, func(value string) factoryapi.WorkerModelProvider {
				return factoryapi.WorkerModelProvider(interfaces.PublicWorkerModelProviderFromInternalRuntime(value))
			}),
			Model: stringPtrIfNotEmpty(worker.Model),
			Type: generatedEnumPtr(worker.Config["type"], func(value string) factoryapi.WorkerType {
				return factoryapi.WorkerType(interfaces.PublicWorkerTypeFromInternalRuntime(value))
			}),
		})
	}
	return out
}

func generatedWorkstations(workstations []interfaces.FactoryWorkstation, places []interfaces.FactoryPlace) []factoryapi.Workstation {
	placesByID := make(map[string]interfaces.FactoryPlace, len(places))
	for _, place := range places {
		placesByID[place.ID] = place
	}
	out := make([]factoryapi.Workstation, 0, len(workstations))
	for _, workstation := range workstations {
		name := workstation.Name
		if name == "" {
			name = workstation.ID
		}
		converted := factoryapi.Workstation{
			Id:     stringPtrIfNotEmpty(workstation.ID),
			Name:   name,
			Worker: workstation.WorkerID,
			Type: generatedEnumPtr(workstation.Config["type"], func(value string) factoryapi.WorkstationType {
				return factoryapi.WorkstationType(interfaces.PublicWorkstationTypeFromInternalRuntime(value, "", ""))
			}),
			Inputs:      generatedWorkstationIOs(workstation.InputPlaceIDs, placesByID),
			Outputs:     generatedWorkstationIOsPtr(workstation.OutputPlaceIDs, placesByID),
			OnContinue:  generatedWorkstationIOsPtr(workstation.ContinuePlaceIDs, placesByID),
			OnRejection: generatedWorkstationIOsPtr(workstation.RejectionPlaceIDs, placesByID),
			OnFailure:   generatedWorkstationIOsPtr(workstation.FailurePlaceIDs, placesByID),
		}
		if workstation.Kind != "" {
			converted.Behavior = generatedEnumPtr(workstation.Kind, func(value string) factoryapi.WorkstationKind {
				return factoryapi.WorkstationKind(interfaces.CanonicalPublicWorkstationKind(interfaces.WorkstationKind(value)))
			})
		}
		out = append(out, converted)
	}
	return out
}

func generatedWorkstationIOs(placeIDs []string, places map[string]interfaces.FactoryPlace) []factoryapi.WorkstationIO {
	out := make([]factoryapi.WorkstationIO, 0, len(placeIDs))
	for _, placeID := range placeIDs {
		place, ok := places[placeID]
		if !ok {
			workType, stateValue := splitPlaceID(placeID)
			place = interfaces.FactoryPlace{TypeID: workType, State: stateValue}
		}
		out = append(out, factoryapi.WorkstationIO{WorkType: place.TypeID, State: place.State})
	}
	return out
}

func generatedWorkstationIOsPtr(placeIDs []string, places map[string]interfaces.FactoryPlace) *[]factoryapi.WorkstationIO {
	ios := generatedWorkstationIOs(placeIDs, places)
	if len(ios) == 0 {
		return nil
	}
	return &ios
}

func splitPlaceID(placeID string) (string, string) {
	before, after, ok := strings.Cut(placeID, ":")
	if !ok {
		return placeID, ""
	}
	return before, after
}

func dispatchConsumedWorkRefsFromTokens(tokens []factorytoken.Token) []interfaces.DispatchConsumedWorkRef {
	out := make([]interfaces.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		workID := token.Color.WorkID
		if workID == "" {
			workID = token.ID
		}
		if workID == "" {
			continue
		}
		out = append(out, interfaces.DispatchConsumedWorkRef{WorkID: workID})
	}
	return out
}

func dispatchRequestEventMetadataPtr(replayKey string, selection workerexecution.ResolvedRunnerSelection) *interfaces.DispatchRequestEventMetadata {
	if replayKey == "" && selection.RunnerID == "" && selection.Source == "" {
		return nil
	}
	runnerID := stringPtrIfNotEmpty(selection.RunnerID)
	var source *workerexecution.RunnerSelectionSource
	if selection.Source != "" {
		value := selection.Source
		source = &value
	}
	return &interfaces.DispatchRequestEventMetadata{
		ReplayKey:             stringPtrIfNotEmpty(replayKey),
		RunnerID:              runnerID,
		RunnerSelectionSource: source,
	}
}

func eventRelations(relations []work.FactoryRelation) []work.WorkRequestEventRelation {
	out := make([]work.WorkRequestEventRelation, 0, len(relations))
	for _, relation := range relations {
		out = append(out, eventRelation(relation))
	}
	return out
}

func eventRelation(relation work.FactoryRelation) work.WorkRequestEventRelation {
	targetName := relation.TargetWorkName
	if targetName == "" {
		targetName = relation.TargetWorkID
	}
	return work.WorkRequestEventRelation{
		Type:           work.WorkRelationType(relation.Type),
		SourceWorkName: relation.SourceWorkName,
		TargetWorkName: targetName,
		TargetWorkID:   relation.TargetWorkID,
		RequiredState:  relation.RequiredState,
	}
}

func (h *FactoryEventHistory) dispatchResourcesPtr(tokens []factorytoken.Token) *[]interfaces.DispatchResourceRef {
	resources := make([]interfaces.DispatchResourceRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		resources = append(resources, h.dispatchResource(token.Color.WorkTypeID))
	}
	return slicePtr(resources)
}

func (h *FactoryEventHistory) dispatchOutputResourcesPtr(mutations []interfaces.TokenMutationRecord) *[]workerexecution.DispatchResourceEventRef {
	resources := make([]workerexecution.DispatchResourceEventRef, 0, len(mutations))
	for _, mutation := range mutations {
		if mutation.Token == nil || mutation.Token.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		resource := h.dispatchResource(mutation.Token.Color.WorkTypeID)
		resources = append(resources, workerexecution.DispatchResourceEventRef(resource))
	}
	return &resources
}

func (h *FactoryEventHistory) dispatchResource(resourceID string) interfaces.DispatchResourceRef {
	resource := interfaces.DispatchResourceRef{Name: resourceID}
	if h.net != nil && h.net.Resources != nil {
		if def := h.net.Resources[resourceID]; def != nil {
			resource.Name = def.Name
			if resource.Name == "" {
				resource.Name = def.ID
			}
			resource.Capacity = def.Capacity
		}
	}
	return resource
}

func eventWorks(items []work.FactoryWorkItem) []work.WorkRequestEventWork {
	out := make([]work.WorkRequestEventWork, 0, len(items))
	for _, item := range items {
		name := item.DisplayName
		if name == "" {
			name = item.ID
		}
		currentChainingTraceID := item.CurrentChainingTraceID
		if currentChainingTraceID == "" {
			currentChainingTraceID = item.TraceID
		}
		var state *work.WorkEventState
		if item.State != "" {
			state = &work.WorkEventState{Name: item.State, Type: string(inferredGeneratedWorkStateType(item.State))}
		}
		out = append(out, work.WorkRequestEventWork{
			Name:                     name,
			WorkID:                   item.ID,
			WorkTypeID:               item.WorkTypeID,
			State:                    state,
			ChainingTraceDepth:       item.ChainingTraceDepth,
			CurrentChainingTraceID:   currentChainingTraceID,
			PreviousChainingTraceIDs: append([]string(nil), item.PreviousChainingTraceIDs...),
			TraceID:                  item.TraceID,
			Content:                  work.CloneWorkContentParts(item.Content),
			Tags:                     cloneStringMap(item.Tags),
		})
	}
	return out
}

func requestEventWorks(items []work.FactoryWorkItem) []work.WorkRequestEventWork {
	out := eventWorks(items)
	for index := range out {
		out[index].Content = requestEventContent(out[index].Content)
	}
	return out
}

func requestEventContent(parts []work.WorkContentPart) []work.WorkContentPart {
	out := make([]work.WorkContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type.Normalized() {
		case work.WorkContentPartTypeText, work.WorkContentPartTypeImage,
			work.WorkContentPartTypeAudio, work.WorkContentPartTypeBinary:
			out = append(out, part)
		case work.WorkContentPartTypeJSON:
			if len(part.JSON) == 0 || json.Valid(part.JSON) {
				out = append(out, part)
			}
		}
	}
	return work.CloneWorkContentParts(out)
}

func eventWorksPtr(items []work.FactoryWorkItem) *[]work.WorkRequestEventWork {
	out := eventWorks(items)
	return &out
}

func generatedEnumPtr[T ~string](value string, convert func(string) T) *T {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := convert(value)
	return &converted
}
