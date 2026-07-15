package projections

import (
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func initialStructureFromGenerated(payload factoryapi.InitialStructureRequestEventPayload) interfaces.InitialStructurePayload {
	factoryPayload := payload.Factory
	resources, resourcePlaces := resourcesAndPlacesFromGenerated(factoryPayload.Resources)
	workTypes, workTypePlaces := workTypesAndPlacesFromGenerated(factoryPayload.WorkTypes)

	places := make([]interfaces.FactoryPlace, 0, len(resourcePlaces)+len(workTypePlaces))
	places = append(places, resourcePlaces...)
	places = append(places, workTypePlaces...)

	return interfaces.InitialStructurePayload{
		Name:         string(factoryPayload.Name),
		Resources:    resources,
		Layout:       layoutFromGenerated(factoryPayload.Layout),
		Workers:      workersFromGenerated(factoryPayload.Workers),
		WorkTypes:    workTypes,
		Workstations: workstationsFromGenerated(factoryPayload.Workstations),
		Places:       places,
	}
}

func layoutFromGenerated(layout *factoryapi.FactoryLayout) *interfaces.FactoryLayoutConfig {
	if layout == nil {
		return nil
	}
	return &interfaces.FactoryLayoutConfig{
		SchemaVersion: int(layout.SchemaVersion),
		Nodes:         layoutNodesFromGenerated(layout.Nodes),
		Edges:         layoutEdgesFromGenerated(layout.Edges),
		Groups:        layoutGroupsFromGenerated(layout.Groups),
		Viewport:      layoutViewportFromGenerated(layout.Viewport),
		Preferences:   layoutPreferencesFromGenerated(layout.Preferences),
	}
}

func layoutNodesFromGenerated(nodes *[]factoryapi.FactoryLayoutNode) []interfaces.FactoryLayoutNodeConfig {
	if nodes == nil {
		return nil
	}
	out := make([]interfaces.FactoryLayoutNodeConfig, len(*nodes))
	for i, node := range *nodes {
		out[i] = interfaces.FactoryLayoutNodeConfig{
			ID:       node.Id,
			Position: layoutPointFromGenerated(node.Position),
			Size:     layoutSizeFromGenerated(node.Size),
			Locked:   node.Locked,
		}
	}
	return out
}

func layoutEdgesFromGenerated(edges *[]factoryapi.FactoryLayoutEdge) []interfaces.FactoryLayoutEdgeConfig {
	if edges == nil {
		return nil
	}
	out := make([]interfaces.FactoryLayoutEdgeConfig, len(*edges))
	for i, edge := range *edges {
		out[i] = interfaces.FactoryLayoutEdgeConfig{
			ID:            edge.Id,
			Waypoints:     layoutPointsFromGenerated(edge.Waypoints),
			LabelPosition: layoutPointPtrFromGenerated(edge.LabelPosition),
		}
	}
	return out
}

func layoutGroupsFromGenerated(groups *[]factoryapi.FactoryLayoutGroup) []interfaces.FactoryLayoutGroupConfig {
	if groups == nil {
		return nil
	}
	out := make([]interfaces.FactoryLayoutGroupConfig, len(*groups))
	for i, group := range *groups {
		out[i] = interfaces.FactoryLayoutGroupConfig{
			ID:            group.Id,
			Label:         stringValue(group.Label),
			Bounds:        layoutBoundsFromGenerated(group.Bounds),
			NodeIDs:       append([]string(nil), group.NodeIds...),
			ParentGroupID: group.ParentGroupId,
			Color:         stringValue(group.Color),
			Locked:        group.Locked,
		}
	}
	return out
}

func layoutViewportFromGenerated(viewport *factoryapi.FactoryLayoutViewport) *interfaces.FactoryLayoutViewportConfig {
	if viewport == nil {
		return nil
	}
	return &interfaces.FactoryLayoutViewportConfig{
		X:    float64(viewport.X),
		Y:    float64(viewport.Y),
		Zoom: float64(viewport.Zoom),
	}
}

func layoutPreferencesFromGenerated(preferences *factoryapi.FactoryLayoutPreferences) *interfaces.FactoryLayoutPreferencesConfig {
	if preferences == nil {
		return nil
	}
	return &interfaces.FactoryLayoutPreferencesConfig{
		Direction: enumStringValue(preferences.Direction),
	}
}

func layoutPointFromGenerated(point factoryapi.FactoryLayoutPoint) interfaces.FactoryLayoutPointConfig {
	return interfaces.FactoryLayoutPointConfig{
		X: float64(point.X),
		Y: float64(point.Y),
	}
}

func layoutPointPtrFromGenerated(point *factoryapi.FactoryLayoutPoint) *interfaces.FactoryLayoutPointConfig {
	if point == nil {
		return nil
	}
	out := layoutPointFromGenerated(*point)
	return &out
}

func layoutPointsFromGenerated(points *[]factoryapi.FactoryLayoutPoint) []interfaces.FactoryLayoutPointConfig {
	if points == nil {
		return nil
	}
	out := make([]interfaces.FactoryLayoutPointConfig, len(*points))
	for i, point := range *points {
		out[i] = layoutPointFromGenerated(point)
	}
	return out
}

func layoutSizeFromGenerated(size *factoryapi.FactoryLayoutSize) *interfaces.FactoryLayoutSizeConfig {
	if size == nil {
		return nil
	}
	return &interfaces.FactoryLayoutSizeConfig{
		Width:  float64(size.Width),
		Height: float64(size.Height),
	}
}

func layoutBoundsFromGenerated(bounds factoryapi.FactoryLayoutBounds) interfaces.FactoryLayoutBoundsConfig {
	return interfaces.FactoryLayoutBoundsConfig{
		X:      float64(bounds.X),
		Y:      float64(bounds.Y),
		Width:  float64(bounds.Width),
		Height: float64(bounds.Height),
	}
}

func resourcesAndPlacesFromGenerated(resources *[]factoryapi.Resource) ([]interfaces.FactoryResource, []interfaces.FactoryPlace) {
	generated := sliceValue(resources)
	convertedResources := make([]interfaces.FactoryResource, 0, len(generated))
	convertedPlaces := make([]interfaces.FactoryPlace, 0, len(generated))
	for _, resource := range generated {
		convertedResources = append(convertedResources, interfaces.FactoryResource{
			ID:       resource.Name,
			Name:     resource.Name,
			Capacity: resource.Capacity,
		})
		convertedPlaces = append(convertedPlaces, interfaces.FactoryPlace{
			ID:       generatedPlaceID(resource.Name, "available"),
			TypeID:   resource.Name,
			State:    "available",
			Category: "PROCESSING",
		})
	}
	return convertedResources, convertedPlaces
}

func workTypesAndPlacesFromGenerated(workTypes *[]factoryapi.WorkType) ([]interfaces.FactoryWorkType, []interfaces.FactoryPlace) {
	generated := sliceValue(workTypes)
	convertedWorkTypes := make([]interfaces.FactoryWorkType, 0, len(generated))
	convertedPlaces := make([]interfaces.FactoryPlace, 0)
	for _, workType := range generated {
		converted := interfaces.FactoryWorkType{
			ID:   workType.Name,
			Name: workType.Name,
		}
		for _, stateDef := range workType.States {
			category := string(stateDef.Type)
			converted.States = append(converted.States, interfaces.FactoryStateDefinition{
				Value:    stateDef.Name,
				Category: category,
			})
			convertedPlaces = append(convertedPlaces, interfaces.FactoryPlace{
				ID:       generatedPlaceID(workType.Name, stateDef.Name),
				TypeID:   workType.Name,
				State:    stateDef.Name,
				Category: category,
			})
		}
		convertedWorkTypes = append(convertedWorkTypes, converted)
	}
	return convertedWorkTypes, convertedPlaces
}
