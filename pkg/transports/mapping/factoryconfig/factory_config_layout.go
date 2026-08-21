// Factory portable-layout validation and bidirectional layout mapping helpers.
package factoryconfig

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func validatePortableLayoutBoundaryJSON(data []byte) error {
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("decode layout validation payload: %w", err)
	}

	layoutValue, ok := root["layout"]
	if !ok || layoutValue == nil {
		return nil
	}

	layout, ok := layoutValue.(map[string]any)
	if !ok {
		return fmt.Errorf("layout must be an object")
	}
	if err := requireNumber(layout, "schemaVersion", "layout"); err != nil {
		return err
	}
	totalImageBytes := 0
	if err := validateLayoutNodeArray(layout, "nodes", "layout", &totalImageBytes); err != nil {
		return err
	}
	if err := validateLayoutEdgeArray(layout, "edges", "layout"); err != nil {
		return err
	}
	if err := validateLayoutGroupArray(layout, "groups", "layout"); err != nil {
		return err
	}
	if err := validateLayoutAnnotationArray(layout, "annotations", "layout", &totalImageBytes); err != nil {
		return err
	}
	if err := validateOptionalPointObject(layout, "viewport", "layout", true, "zoom"); err != nil {
		return err
	}
	if err := validateLayoutPreferences(layout, "preferences", "layout"); err != nil {
		return err
	}
	return nil
}

func validateLayoutNodeArray(parent map[string]any, key string, path string, totalImageBytes *int) error {
	values, ok, err := optionalObjectArray(parent, key, path)
	if !ok || err != nil {
		return err
	}
	seenEmptyStateNodeIDs := make(map[string]struct{}, len(values))
	for index, node := range values {
		nodePath := fmt.Sprintf("%s.%s[%d]", path, key, index)
		if err := requireNonBlankString(node, "id", nodePath); err != nil {
			return err
		}
		if err := validateOptionalPointObject(node, "position", nodePath, true); err != nil {
			return err
		}
		if err := validateOptionalSizeObject(node, "size", nodePath, false); err != nil {
			return err
		}
		imageBytes, err := validateLayoutNodeEmptyState(node, nodePath)
		if err != nil {
			return err
		}
		if imageBytes > 0 || nodeHasTextEmptyState(node) {
			nodeID := node["id"].(string)
			if _, duplicate := seenEmptyStateNodeIDs[nodeID]; duplicate {
				return fmt.Errorf("%s.emptyState duplicates an empty state for canonical node %q", nodePath, nodeID)
			}
			seenEmptyStateNodeIDs[nodeID] = struct{}{}
		}
		*totalImageBytes += imageBytes
		if *totalImageBytes > factoryLayoutEmbeddedImageTotalMaxBytes {
			return fmt.Errorf("%s.emptyState.image.source.data exceeds the %d-byte Factory embedded-image budget", nodePath, factoryLayoutEmbeddedImageTotalMaxBytes)
		}
	}
	return nil
}

func validateLayoutEdgeArray(parent map[string]any, key string, path string) error {
	values, ok, err := optionalObjectArray(parent, key, path)
	if !ok || err != nil {
		return err
	}
	for index, edge := range values {
		edgePath := fmt.Sprintf("%s.%s[%d]", path, key, index)
		if err := requireString(edge, "id", edgePath); err != nil {
			return err
		}
		if err := validateOptionalPointArray(edge, "waypoints", edgePath); err != nil {
			return err
		}
		if err := validateOptionalPointObject(edge, "labelPosition", edgePath, false); err != nil {
			return err
		}
	}
	return nil
}

func validateLayoutGroupArray(parent map[string]any, key string, path string) error {
	values, ok, err := optionalObjectArray(parent, key, path)
	if !ok || err != nil {
		return err
	}
	for index, group := range values {
		groupPath := fmt.Sprintf("%s.%s[%d]", path, key, index)
		if err := requireString(group, "id", groupPath); err != nil {
			return err
		}
		if err := validateOptionalBoundsObject(group, "bounds", groupPath, true); err != nil {
			return err
		}
		if err := requireStringArray(group, "nodeIds", groupPath); err != nil {
			return err
		}
	}
	return nil
}

func validateLayoutPreferences(parent map[string]any, key string, path string) error {
	value, ok := parent[key]
	if !ok || value == nil {
		return nil
	}
	record, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.%s must be an object", path, key)
	}
	direction, ok := record["direction"]
	if !ok || direction == nil {
		return nil
	}
	directionValue, ok := direction.(string)
	if !ok {
		return fmt.Errorf("%s.%s.direction must be a string", path, key)
	}
	switch directionValue {
	case "UP", "DOWN", "LEFT", "RIGHT":
		return nil
	default:
		return fmt.Errorf("%s.%s.direction must be one of UP, DOWN, LEFT, RIGHT", path, key)
	}
}

func validateOptionalPointArray(parent map[string]any, key string, path string) error {
	value, ok := parent[key]
	if !ok || value == nil {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s.%s must be an array", path, key)
	}
	for index, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.%s[%d] must be an object", path, key, index)
		}
		itemPath := fmt.Sprintf("%s.%s[%d]", path, key, index)
		if err := requireNumber(record, "x", itemPath); err != nil {
			return err
		}
		if err := requireNumber(record, "y", itemPath); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalPointObject(parent map[string]any, key string, path string, required bool, extraRequiredKeys ...string) error {
	value, ok := parent[key]
	if !ok || value == nil {
		if required {
			return fmt.Errorf("%s.%s is required", path, key)
		}
		return nil
	}
	record, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.%s must be an object", path, key)
	}
	pointPath := fmt.Sprintf("%s.%s", path, key)
	if err := requireNumber(record, "x", pointPath); err != nil {
		return err
	}
	if err := requireNumber(record, "y", pointPath); err != nil {
		return err
	}
	for _, extraKey := range extraRequiredKeys {
		if err := requireNumber(record, extraKey, pointPath); err != nil {
			return err
		}
	}
	return nil
}

func validateOptionalSizeObject(parent map[string]any, key string, path string, required bool) error {
	value, ok := parent[key]
	if !ok || value == nil {
		if required {
			return fmt.Errorf("%s.%s is required", path, key)
		}
		return nil
	}
	record, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.%s must be an object", path, key)
	}
	sizePath := fmt.Sprintf("%s.%s", path, key)
	if err := requireNumber(record, "width", sizePath); err != nil {
		return err
	}
	if err := requireNumber(record, "height", sizePath); err != nil {
		return err
	}
	return nil
}

func validateOptionalBoundsObject(parent map[string]any, key string, path string, required bool) error {
	value, ok := parent[key]
	if !ok || value == nil {
		if required {
			return fmt.Errorf("%s.%s is required", path, key)
		}
		return nil
	}
	record, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s.%s must be an object", path, key)
	}
	boundsPath := fmt.Sprintf("%s.%s", path, key)
	if err := requireNumber(record, "x", boundsPath); err != nil {
		return err
	}
	if err := requireNumber(record, "y", boundsPath); err != nil {
		return err
	}
	if err := requireNumber(record, "width", boundsPath); err != nil {
		return err
	}
	if err := requireNumber(record, "height", boundsPath); err != nil {
		return err
	}
	return nil
}

func optionalObjectArray(parent map[string]any, key string, path string) ([]map[string]any, bool, error) {
	value, ok := parent[key]
	if !ok || value == nil {
		return nil, false, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, true, fmt.Errorf("%s.%s must be an array", path, key)
	}
	result := make([]map[string]any, len(items))
	for index, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			return nil, true, fmt.Errorf("%s.%s[%d] must be an object", path, key, index)
		}
		result[index] = record
	}
	return result, true, nil
}

func requireString(parent map[string]any, key string, path string) error {
	value, ok := parent[key]
	if !ok || value == nil {
		return fmt.Errorf("%s.%s is required", path, key)
	}
	if _, ok := value.(string); !ok {
		return fmt.Errorf("%s.%s must be a string", path, key)
	}
	return nil
}

func requireStringArray(parent map[string]any, key string, path string) error {
	value, ok := parent[key]
	if !ok || value == nil {
		return fmt.Errorf("%s.%s is required", path, key)
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s.%s must be an array", path, key)
	}
	for index, item := range items {
		if _, ok := item.(string); !ok {
			return fmt.Errorf("%s.%s[%d] must be a string", path, key, index)
		}
	}
	return nil
}

func requireNumber(parent map[string]any, key string, path string) error {
	value, ok := parent[key]
	if !ok || value == nil {
		return fmt.Errorf("%s.%s is required", path, key)
	}
	if _, ok := value.(float64); !ok {
		return fmt.Errorf("%s.%s must be a number", path, key)
	}
	return nil
}

func factoryLayoutAPIFromInternal(layout *interfaces.FactoryLayoutConfig) *factoryapi.FactoryLayout {
	if layout == nil {
		return nil
	}

	apiLayout := &factoryapi.FactoryLayout{
		SchemaVersion: int32(layout.SchemaVersion),
		Nodes:         factoryLayoutNodesAPIFromInternal(layout.Nodes),
		Edges:         factoryLayoutEdgesAPIFromInternal(layout.Edges),
		Groups:        factoryLayoutGroupsAPIFromInternal(layout.Groups),
		Annotations:   factoryLayoutAnnotationsAPIFromInternal(layout.Annotations),
		Viewport:      factoryLayoutViewportAPIFromInternal(layout.Viewport),
		Preferences:   factoryLayoutPreferencesAPIFromInternal(layout.Preferences),
	}
	return apiLayout
}

func factoryLayoutNodesAPIFromInternal(nodes []interfaces.FactoryLayoutNodeConfig) *[]factoryapi.FactoryLayoutNode {
	if len(nodes) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryLayoutNode, len(nodes))
	for i, node := range nodes {
		values[i] = factoryapi.FactoryLayoutNode{
			Id:         node.ID,
			Position:   factoryLayoutPointAPIFromInternal(node.Position),
			Size:       factoryLayoutSizeAPIFromInternal(node.Size),
			Locked:     node.Locked,
			EmptyState: factoryLayoutEmptyStateAPIFromInternal(node.EmptyState),
		}
	}
	return &values
}

func factoryLayoutEdgesAPIFromInternal(edges []interfaces.FactoryLayoutEdgeConfig) *[]factoryapi.FactoryLayoutEdge {
	if len(edges) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryLayoutEdge, len(edges))
	for i, edge := range edges {
		values[i] = factoryapi.FactoryLayoutEdge{
			Id:            edge.ID,
			Waypoints:     factoryLayoutPointsAPIFromInternal(edge.Waypoints),
			LabelPosition: factoryLayoutPointPtrAPIFromInternal(edge.LabelPosition),
		}
	}
	return &values
}

func factoryLayoutGroupsAPIFromInternal(groups []interfaces.FactoryLayoutGroupConfig) *[]factoryapi.FactoryLayoutGroup {
	if len(groups) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryLayoutGroup, len(groups))
	for i, group := range groups {
		values[i] = factoryapi.FactoryLayoutGroup{
			Id:            group.ID,
			Label:         stringPtrIfNotEmpty(group.Label),
			Bounds:        factoryLayoutBoundsAPIFromInternal(group.Bounds),
			NodeIds:       append([]string(nil), group.NodeIDs...),
			ParentGroupId: group.ParentGroupID,
			Color:         stringPtrIfNotEmpty(group.Color),
			Locked:        group.Locked,
		}
	}
	return &values
}

func factoryLayoutViewportAPIFromInternal(viewport *interfaces.FactoryLayoutViewportConfig) *factoryapi.FactoryLayoutViewport {
	if viewport == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutViewport{
		X:    float32(viewport.X),
		Y:    float32(viewport.Y),
		Zoom: float32(viewport.Zoom),
	}
}

func factoryLayoutPreferencesAPIFromInternal(preferences *interfaces.FactoryLayoutPreferencesConfig) *factoryapi.FactoryLayoutPreferences {
	if preferences == nil {
		return nil
	}
	result := &factoryapi.FactoryLayoutPreferences{}
	if strings.TrimSpace(preferences.Direction) != "" {
		direction := factoryapi.FactoryLayoutPreferencesDirection(preferences.Direction)
		result.Direction = &direction
	}
	return result
}

func factoryLayoutPointAPIFromInternal(point interfaces.FactoryLayoutPointConfig) factoryapi.FactoryLayoutPoint {
	return factoryapi.FactoryLayoutPoint{
		X: float32(point.X),
		Y: float32(point.Y),
	}
}

func factoryLayoutPointPtrAPIFromInternal(point *interfaces.FactoryLayoutPointConfig) *factoryapi.FactoryLayoutPoint {
	if point == nil {
		return nil
	}
	value := factoryLayoutPointAPIFromInternal(*point)
	return &value
}

func factoryLayoutPointsAPIFromInternal(points []interfaces.FactoryLayoutPointConfig) *[]factoryapi.FactoryLayoutPoint {
	if len(points) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryLayoutPoint, len(points))
	for i, point := range points {
		values[i] = factoryLayoutPointAPIFromInternal(point)
	}
	return &values
}

func factoryLayoutSizeAPIFromInternal(size *interfaces.FactoryLayoutSizeConfig) *factoryapi.FactoryLayoutSize {
	if size == nil {
		return nil
	}
	return &factoryapi.FactoryLayoutSize{
		Width:  float32(size.Width),
		Height: float32(size.Height),
	}
}

func factoryLayoutBoundsAPIFromInternal(bounds interfaces.FactoryLayoutBoundsConfig) factoryapi.FactoryLayoutBounds {
	return factoryapi.FactoryLayoutBounds{
		X:      float32(bounds.X),
		Y:      float32(bounds.Y),
		Width:  float32(bounds.Width),
		Height: float32(bounds.Height),
	}
}

func factoryLayoutInternalFromAPI(layout *factoryapi.FactoryLayout) *interfaces.FactoryLayoutConfig {
	if layout == nil {
		return nil
	}
	return &interfaces.FactoryLayoutConfig{
		SchemaVersion: int(layout.SchemaVersion),
		Nodes:         factoryLayoutNodesInternalFromAPI(layout.Nodes),
		Edges:         factoryLayoutEdgesInternalFromAPI(layout.Edges),
		Groups:        factoryLayoutGroupsInternalFromAPI(layout.Groups),
		Annotations:   factoryLayoutAnnotationsInternalFromAPI(layout.Annotations),
		Viewport:      factoryLayoutViewportInternalFromAPI(layout.Viewport),
		Preferences:   factoryLayoutPreferencesInternalFromAPI(layout.Preferences),
	}
}

func factoryLayoutAnnotationsInternalFromAPI(annotations *[]factoryapi.FactoryLayoutAnnotation) []interfaces.FactoryLayoutAnnotationConfig {
	if annotations == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutAnnotationConfig, len(*annotations))
	for i, annotation := range *annotations {
		values[i] = interfaces.FactoryLayoutAnnotationConfig{
			ID:       annotation.Id,
			Kind:     string(annotation.Kind),
			Position: factoryLayoutAnnotationPositionInternalFromAPI(annotation.Position),
			Size:     factoryLayoutAnnotationSizeInternalFromAPI(annotation.Size),
			Note:     factoryLayoutNoteInternalFromAPI(annotation.Note),
			Image:    factoryLayoutImageInternalFromAPI(annotation.Image),
		}
	}
	return values
}

func factoryLayoutAnnotationPositionInternalFromAPI(position factoryapi.FactoryLayoutAnnotationPosition) interfaces.FactoryLayoutPointConfig {
	return interfaces.FactoryLayoutPointConfig{X: float64(position.X), Y: float64(position.Y)}
}

func factoryLayoutAnnotationSizeInternalFromAPI(size *factoryapi.FactoryLayoutAnnotationSize) *interfaces.FactoryLayoutSizeConfig {
	if size == nil {
		return nil
	}
	return &interfaces.FactoryLayoutSizeConfig{Width: float64(size.Width), Height: float64(size.Height)}
}

func factoryLayoutNoteInternalFromAPI(note *factoryapi.FactoryLayoutNote) *interfaces.FactoryLayoutNoteConfig {
	if note == nil {
		return nil
	}
	return &interfaces.FactoryLayoutNoteConfig{
		Title: stringValue(note.Title),
		Body:  note.Body,
		Tone:  string(note.Tone),
	}
}

func factoryLayoutImageInternalFromAPI(image *factoryapi.FactoryLayoutImage) *interfaces.FactoryLayoutImageConfig {
	if image == nil {
		return nil
	}
	return &interfaces.FactoryLayoutImageConfig{
		Source: interfaces.FactoryLayoutImageSourceConfig{
			Kind:      string(image.Source.Kind),
			MediaType: string(image.Source.MediaType),
			Data:      base64.StdEncoding.EncodeToString(image.Source.Data),
		},
		AlternativeText: image.AlternativeText,
	}
}

func factoryLayoutNodesInternalFromAPI(nodes *[]factoryapi.FactoryLayoutNode) []interfaces.FactoryLayoutNodeConfig {
	if nodes == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutNodeConfig, len(*nodes))
	for i, node := range *nodes {
		values[i] = interfaces.FactoryLayoutNodeConfig{
			ID:         node.Id,
			Position:   factoryLayoutPointInternalFromAPI(node.Position),
			Size:       factoryLayoutSizeInternalFromAPI(node.Size),
			Locked:     node.Locked,
			EmptyState: factoryLayoutEmptyStateInternalFromAPI(node.EmptyState),
		}
	}
	return values
}

func factoryLayoutEmptyStateInternalFromAPI(emptyState *factoryapi.FactoryLayoutEmptyState) *interfaces.FactoryLayoutEmptyStateConfig {
	if emptyState == nil {
		return nil
	}
	return &interfaces.FactoryLayoutEmptyStateConfig{
		Text:  stringValue(emptyState.Text),
		Image: factoryLayoutImageInternalFromAPI(emptyState.Image),
	}
}

func factoryLayoutEdgesInternalFromAPI(edges *[]factoryapi.FactoryLayoutEdge) []interfaces.FactoryLayoutEdgeConfig {
	if edges == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutEdgeConfig, len(*edges))
	for i, edge := range *edges {
		values[i] = interfaces.FactoryLayoutEdgeConfig{
			ID:            edge.Id,
			Waypoints:     factoryLayoutPointsInternalFromAPI(edge.Waypoints),
			LabelPosition: factoryLayoutPointPtrInternalFromAPI(edge.LabelPosition),
		}
	}
	return values
}

func factoryLayoutGroupsInternalFromAPI(groups *[]factoryapi.FactoryLayoutGroup) []interfaces.FactoryLayoutGroupConfig {
	if groups == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutGroupConfig, len(*groups))
	for i, group := range *groups {
		values[i] = interfaces.FactoryLayoutGroupConfig{
			ID:            group.Id,
			Label:         stringValue(group.Label),
			Bounds:        factoryLayoutBoundsInternalFromAPI(group.Bounds),
			NodeIDs:       append([]string(nil), group.NodeIds...),
			ParentGroupID: group.ParentGroupId,
			Color:         stringValue(group.Color),
			Locked:        group.Locked,
		}
	}
	return values
}

func factoryLayoutViewportInternalFromAPI(viewport *factoryapi.FactoryLayoutViewport) *interfaces.FactoryLayoutViewportConfig {
	if viewport == nil {
		return nil
	}
	return &interfaces.FactoryLayoutViewportConfig{
		X:    float64(viewport.X),
		Y:    float64(viewport.Y),
		Zoom: float64(viewport.Zoom),
	}
}

func factoryLayoutPreferencesInternalFromAPI(preferences *factoryapi.FactoryLayoutPreferences) *interfaces.FactoryLayoutPreferencesConfig {
	if preferences == nil {
		return nil
	}
	return &interfaces.FactoryLayoutPreferencesConfig{
		Direction: enumStringValue(preferences.Direction),
	}
}

func factoryLayoutPointInternalFromAPI(point factoryapi.FactoryLayoutPoint) interfaces.FactoryLayoutPointConfig {
	return interfaces.FactoryLayoutPointConfig{
		X: float64(point.X),
		Y: float64(point.Y),
	}
}

func factoryLayoutPointPtrInternalFromAPI(point *factoryapi.FactoryLayoutPoint) *interfaces.FactoryLayoutPointConfig {
	if point == nil {
		return nil
	}
	value := factoryLayoutPointInternalFromAPI(*point)
	return &value
}

func factoryLayoutPointsInternalFromAPI(points *[]factoryapi.FactoryLayoutPoint) []interfaces.FactoryLayoutPointConfig {
	if points == nil {
		return nil
	}
	values := make([]interfaces.FactoryLayoutPointConfig, len(*points))
	for i, point := range *points {
		values[i] = factoryLayoutPointInternalFromAPI(point)
	}
	return values
}

func factoryLayoutSizeInternalFromAPI(size *factoryapi.FactoryLayoutSize) *interfaces.FactoryLayoutSizeConfig {
	if size == nil {
		return nil
	}
	return &interfaces.FactoryLayoutSizeConfig{
		Width:  float64(size.Width),
		Height: float64(size.Height),
	}
}

func factoryLayoutBoundsInternalFromAPI(bounds factoryapi.FactoryLayoutBounds) interfaces.FactoryLayoutBoundsConfig {
	return interfaces.FactoryLayoutBoundsConfig{
		X:      float64(bounds.X),
		Y:      float64(bounds.Y),
		Width:  float64(bounds.Width),
		Height: float64(bounds.Height),
	}
}
