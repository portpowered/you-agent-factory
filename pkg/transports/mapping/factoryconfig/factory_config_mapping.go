// backendsizecheck:ignore-file canonical public factory contract mapping remains consolidated until dedicated mapper seams are extracted from pkg/config.
// pkgmaintcheck:ignore-file-lines canonical public factory contract mapping remains consolidated until dedicated mapper seams are extracted from pkg/config.
package factoryconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/http/apitypes"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/optional"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workercompatibility "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/transports/mapping/factorydefinition/retiredboundary"
	contentcontract "github.com/portpowered/infinite-you/pkg/transports/mapping/workcontent"
)

// FactoryConfigMapper maps between on-disk factory configuration payloads and
// canonical in-memory config structures.
type FactoryConfigMapper struct{}

// NewFactoryConfigMapper returns the canonical mapper used across config loading
// and serialization paths.
func NewFactoryConfigMapper() *FactoryConfigMapper {
	return &FactoryConfigMapper{}
}

type generatedFactoryBoundary struct {
	generated factoryapi.Factory
}

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

// Expand parses and normalizes a user-provided factory payload into the internal
// canonical configuration representation.
func (m *FactoryConfigMapper) Expand(data []byte) (*interfaces.FactoryConfig, error) {
	boundary, err := decodeGeneratedFactoryBoundaryJSON(data)
	if err != nil {
		return nil, err
	}

	cfg, err := FactoryConfigFromOpenAPI(boundary.generated)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func decodeGeneratedFactoryBoundaryJSON(data []byte) (generatedFactoryBoundary, error) {
	if err := retiredboundary.RejectGeneratedBoundaryAliases(data); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	normalizedData, err := normalizeFactoryInputJSON(data)
	if err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := retiredboundary.RejectExhaustionRulesField(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := retiredboundary.RejectFanInField(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := retiredboundary.RejectCronIntervalField(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if err := validatePortableLayoutBoundaryJSON(normalizedData); err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}

	apiCfg, err := decodeGeneratedFactoryBoundary(normalizedData)
	if err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	return generatedFactoryBoundary{
		generated: apiCfg,
	}, nil
}

func decodeGeneratedFactoryBoundary(data []byte) (factoryapi.Factory, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var apiCfg factoryapi.Factory
	if err := decoder.Decode(&apiCfg); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("unmarshal factory api model: %w", err)
	}
	if err := ensureFactoryBoundaryEOF(decoder); err != nil {
		return factoryapi.Factory{}, err
	}
	if err := validateGeneratedFactoryBoundary(apiCfg); err != nil {
		return factoryapi.Factory{}, err
	}
	return apiCfg, nil
}

func ensureFactoryBoundaryEOF(decoder *json.Decoder) error {
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("unmarshal factory api model: %w", err)
	}
	return fmt.Errorf("unmarshal factory api model: unexpected trailing JSON value")
}

func validateGeneratedFactoryBoundary(apiCfg factoryapi.Factory) error {
	if strings.TrimSpace(string(apiCfg.Name)) == "" {
		return fmt.Errorf("factory.name is required")
	}
	return nil
}

// Flatten serializes an internal factory configuration into canonical JSON that is
// stable for persisted output and downstream tooling.
func (m *FactoryConfigMapper) Flatten(cfg *interfaces.FactoryConfig) ([]byte, error) {
	apiCfg := factoryAPIFromInternalConfig(cfg)
	if isDefaultPetriOrchestratorAPI(apiCfg.Orchestrator) {
		apiCfg.Orchestrator = nil
	}

	raw, err := json.Marshal(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("marshal factory api model: %w", err)
	}

	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, fmt.Errorf("decode factory api payload: %w", err)
	}
	canonical := normalizeFactoryOutputJSONKeys(decoded)
	dropSupportedPortableBundledInlineContent(canonical)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, fmt.Errorf("normalize factory config keys: %w", err)
	}
	return encoded, nil
}

func factoryAPIFromInternalConfig(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	if cfg == nil {
		return factoryapi.Factory{}
	}

	return factoryapi.Factory{
		Name:                factoryReferenceName(cfg),
		Id:                  stringPtrIfNotEmpty(cfg.Project),
		Version:             hybridLogicalTimestampPtr(cfg.Version),
		Guards:              factoryGuardsAPIFromInternal(cfg.Guards),
		InputTypes:          inputTypesAPIFromInternal(cfg.InputTypes),
		InvocationReturn:    invocationReturnAPIFromInternal(cfg.InvocationReturn),
		InvocationSignature: invocationSignatureAPIFromInternal(cfg.InvocationSignature),
		Orchestrator:        orchestratorAPIFromInternal(cfg),
		WorkTypes:           workTypesAPIFromInternal(cfg.WorkTypes),
		Resources:           resourcesAPIFromInternal(cfg.Resources),
		SupportingFiles:     resourceManifestAPIFromInternal(cfg.ResourceManifest),
		Layout:              factoryLayoutAPIFromInternal(cfg.Layout),
		Workers:             workersAPIFromInternal(cfg.Workers, cfg.Workstations),
		Workstations:        workstationsAPIFromInternal(cfg.Workstations, workerTypesByName(cfg.Workers)),
	}
}

func invocationSignatureAPIFromInternal(value *interfaces.InvocationSignatureConfig) *factoryapi.FactoryInvocationSignature {
	if value == nil {
		return nil
	}
	return &factoryapi.FactoryInvocationSignature{
		Parameters:                 invocationParametersAPIFromInternal(value.Parameters),
		UnknownNamedArgumentPolicy: invocationUnknownNamedArgumentPolicyPtr(value.UnknownNamedArgumentPolicy),
		OutputContract:             invocationOutputContractAPIFromInternal(value.OutputContract),
		Examples:                   invocationExamplesAPIFromInternal(value.Examples),
	}
}

func invocationParametersAPIFromInternal(parameters []interfaces.InvocationParameterConfig) *[]factoryapi.FactoryInvocationParameter {
	if len(parameters) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryInvocationParameter, len(parameters))
	for i, parameter := range parameters {
		values[i] = factoryapi.FactoryInvocationParameter{
			Name:          parameter.Name,
			Description:   stringPtrIfNotEmpty(parameter.Description),
			ExternalName:  stringPtrIfNotEmpty(parameter.ExternalName),
			Aliases:       stringSlicePtr(parameter.Aliases),
			TypeHint:      invocationParameterTypeHintPtr(parameter.TypeHint),
			ValueMode:     invocationParameterValueModePtr(parameter.ValueMode),
			Required:      boolPtrIfTrue(parameter.Required),
			Sensitive:     boolPtrIfTrue(parameter.Sensitive),
			Choices:       stringSlicePtr(parameter.Choices),
			DefaultValue:  stringPtrIfNotEmpty(parameter.DefaultValue),
			DefaultValues: stringSlicePtr(parameter.DefaultValues),
			Bindings:      invocationParameterBindingsAPIFromInternal(parameter.Bindings),
		}
	}
	return &values
}

func invocationParameterBindingsAPIFromInternal(bindings []interfaces.InvocationParameterBindingConfig) *[]factoryapi.FactoryInvocationParameterBinding {
	if len(bindings) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryInvocationParameterBinding, len(bindings))
	for i, binding := range bindings {
		values[i] = factoryapi.FactoryInvocationParameterBinding{
			Kind:     factoryapi.FactoryInvocationParameterBindingKind(binding.Kind),
			Position: intPtrIfNonZero(binding.Position),
		}
	}
	return &values
}

func invocationUnknownNamedArgumentPolicyPtr(value string) *factoryapi.FactoryInvocationUnknownNamedArgumentPolicy {
	if value == "" {
		return nil
	}
	policy := factoryapi.FactoryInvocationUnknownNamedArgumentPolicy(value)
	return &policy
}

func invocationParameterTypeHintPtr(value string) *factoryapi.FactoryInvocationParameterTypeHint {
	if value == "" {
		return nil
	}
	typeHint := factoryapi.FactoryInvocationParameterTypeHint(value)
	return &typeHint
}

func invocationParameterValueModePtr(value string) *factoryapi.FactoryInvocationParameterValueMode {
	if value == "" {
		return nil
	}
	valueMode := factoryapi.FactoryInvocationParameterValueMode(value)
	return &valueMode
}

func invocationOutputContractAPIFromInternal(value *interfaces.InvocationOutputContractConfig) *factoryapi.FactoryInvocationOutputContract {
	if value == nil {
		return nil
	}
	return &factoryapi.FactoryInvocationOutputContract{
		Mode:          invocationOutputContractModePtr(value.Mode),
		PathParameter: stringPtrIfNotEmpty(value.PathParameter),
		ContentType:   stringPtrIfNotEmpty(value.ContentType),
		FileExtension: stringPtrIfNotEmpty(value.FileExtension),
		Description:   stringPtrIfNotEmpty(value.Description),
	}
}

func invocationOutputContractModePtr(value string) *factoryapi.FactoryInvocationOutputContractMode {
	if value == "" {
		return nil
	}
	mode := factoryapi.FactoryInvocationOutputContractMode(value)
	return &mode
}

func invocationExamplesAPIFromInternal(examples []interfaces.InvocationExampleConfig) *[]factoryapi.FactoryInvocationExample {
	if len(examples) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryInvocationExample, len(examples))
	for i, example := range examples {
		values[i] = factoryapi.FactoryInvocationExample{
			Name:        example.Name,
			Description: stringPtrIfNotEmpty(example.Description),
			Argv:        stringSlicePtr(example.Argv),
			Stdin:       stringPtrIfNotEmpty(example.Stdin),
		}
	}
	return &values
}

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
	if err := validateLayoutNodeArray(layout, "nodes", "layout"); err != nil {
		return err
	}
	if err := validateLayoutEdgeArray(layout, "edges", "layout"); err != nil {
		return err
	}
	if err := validateLayoutGroupArray(layout, "groups", "layout"); err != nil {
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

func validateLayoutNodeArray(parent map[string]any, key string, path string) error {
	values, ok, err := optionalObjectArray(parent, key, path)
	if !ok || err != nil {
		return err
	}
	for index, node := range values {
		nodePath := fmt.Sprintf("%s.%s[%d]", path, key, index)
		if err := requireString(node, "id", nodePath); err != nil {
			return err
		}
		if err := validateOptionalPointObject(node, "position", nodePath, true); err != nil {
			return err
		}
		if err := validateOptionalSizeObject(node, "size", nodePath, false); err != nil {
			return err
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
			Id:       node.ID,
			Position: factoryLayoutPointAPIFromInternal(node.Position),
			Size:     factoryLayoutSizeAPIFromInternal(node.Size),
			Locked:   node.Locked,
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

func inputTypesAPIFromInternal(inputTypes []interfaces.InputTypeConfig) *[]factoryapi.InputType {
	if len(inputTypes) == 0 {
		return nil
	}
	result := make([]factoryapi.InputType, len(inputTypes))
	for i, inputType := range inputTypes {
		result[i] = factoryapi.InputType{
			Name: inputType.Name,
			Type: publicFactoryInputKindFromInternal(inputType.Type),
		}
	}
	return &result
}

func invocationReturnAPIFromInternal(value *interfaces.InvocationReturnConfig) *factoryapi.InvocationReturn {
	if value == nil {
		return nil
	}
	result := &factoryapi.InvocationReturn{
		Policy: factoryapi.InvocationReturnPolicy(value.Policy),
	}
	if strings.TrimSpace(value.WorkTypeName) != "" {
		result.WorkTypeName = stringPtrIfNotEmpty(value.WorkTypeName)
	}
	if strings.TrimSpace(value.TerminalState) != "" {
		result.TerminalState = stringPtrIfNotEmpty(value.TerminalState)
	}
	if strings.TrimSpace(value.WorkName) != "" {
		result.WorkName = stringPtrIfNotEmpty(value.WorkName)
	}
	return result
}

func workTypeHandlingBehaviorAPIFromInternal(behaviors []string) *[]factoryapi.WorkTypeHandlingBehavior {
	if len(behaviors) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkTypeHandlingBehavior, 0, len(behaviors))
	for _, behavior := range behaviors {
		canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(behavior)
		if canonical == "" {
			continue
		}
		values = append(values, factoryapi.WorkTypeHandlingBehavior(canonical))
	}
	if len(values) == 0 {
		return nil
	}
	return &values
}

func workTypesAPIFromInternal(workTypes []interfaces.WorkTypeConfig) *[]factoryapi.WorkType {
	if len(workTypes) == 0 {
		return nil
	}
	result := make([]factoryapi.WorkType, len(workTypes))
	for i, workType := range workTypes {
		states := make([]factoryapi.WorkState, len(workType.States))
		for stateIndex, state := range workType.States {
			states[stateIndex] = factoryapi.WorkState{
				Id:   stringPtrIfNotEmpty(state.ID),
				Name: state.Name,
				Type: factoryapi.WorkStateType(state.Type),
			}
		}
		result[i] = factoryapi.WorkType{
			Id:               stringPtrIfNotEmpty(workType.ID),
			Name:             workType.Name,
			States:           states,
			HandlingBehavior: workTypeHandlingBehaviorAPIFromInternal(workType.HandlingBehavior),
		}
	}
	return &result
}

func resourcesAPIFromInternal(resources []interfaces.ResourceConfig) *[]factoryapi.Resource {
	if len(resources) == 0 {
		return nil
	}
	result := make([]factoryapi.Resource, len(resources))
	for i, resource := range resources {
		result[i] = factoryapi.Resource{
			Id:         stringPtrIfNotEmpty(resource.ID),
			Name:       resource.Name,
			Type:       resourceTypePtrIfNotEmpty(resource.Type),
			Capacity:   resource.Capacity,
			Model:      stringPtrIfNotEmpty(resource.Model),
			Backend:    stringPtrIfNotEmpty(resource.Backend),
			LoadPolicy: stringPtrIfNotEmpty(resource.LoadPolicy),
			Provider:   stringPtrIfNotEmpty(resource.Provider),
		}
	}
	return &result
}

func workersAPIFromInternal(workers []interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) *[]factoryapi.Worker {
	if len(workers) == 0 {
		return nil
	}
	result := make([]factoryapi.Worker, len(workers))
	for i, worker := range workers {
		result[i] = *workerDefinitionAPIFromInternalWithUsage(&worker, workstations)
	}
	return &result
}

func workerTypesByName(workers []interfaces.FactoryWorkerConfig) map[string]string {
	if len(workers) == 0 {
		return nil
	}
	workerTypes := make(map[string]string, len(workers))
	for _, worker := range workers {
		workerTypes[worker.Name] = worker.Type
	}
	return workerTypes
}

func workstationsAPIFromInternal(workstations []interfaces.FactoryWorkstationConfig, workerTypes map[string]string) *[]factoryapi.Workstation {
	if len(workstations) == 0 {
		return nil
	}
	result := make([]factoryapi.Workstation, 0, len(workstations))
	for _, workstation := range workstations {
		result = append(result, workstationAPIFromInternal(workstation, workerTypes[workstation.WorkerTypeName]))
	}
	return &result
}

func factoryReferenceName(cfg *interfaces.FactoryConfig) factoryapi.FactoryName {
	if cfg != nil && strings.TrimSpace(cfg.Name) != "" {
		return factoryapi.FactoryName(cfg.Name)
	}
	if cfg != nil && strings.TrimSpace(cfg.Project) != "" {
		return factoryapi.FactoryName(cfg.Project)
	}
	return factoryapi.FactoryName("factory")
}

// FactoryConfigToOpenAPI converts the internal factory config into the generated
// OpenAPI model without passing through normalized on-disk JSON.
func FactoryConfigToOpenAPI(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	return factoryAPIFromInternalConfig(cfg)
}

func hybridLogicalTimestampPtr(version *interfaces.FactoryVersion) *factoryapi.HybridLogicalTimestamp {
	if version == nil {
		return nil
	}
	return &factoryapi.HybridLogicalTimestamp{
		Logical:  apitypes.Int64String(version.Logical),
		Physical: version.Physical.UTC(),
	}
}

func workstationAPIFromInternal(workstation interfaces.FactoryWorkstationConfig, workerType string) factoryapi.Workstation {
	normalized := interfaces.CloneWorkstationConfig(workstation)
	NormalizeWorkstationExecutionLimit(&normalized)
	promptBody := normalized.PromptTemplate
	if promptBody == "" {
		promptBody = normalized.Body
	}

	apiWorkstation := factoryapi.Workstation{
		Name:                  normalized.Name,
		Worker:                normalized.WorkerTypeName,
		Inputs:                workstationIOsAPIFromInternal(normalized.Inputs),
		Outputs:               optionalWorkstationIOsAPIFromInternal(normalized.Outputs),
		ClassificationRoutes:  classificationRoutesAPIFromInternal(normalized.ClassificationRoutes),
		Cron:                  workstationCronAPIFromInternal(normalized.Cron),
		OnContinue:            optionalWorkstationIOsAPIFromInternal(normalized.OnContinue),
		OnRejection:           optionalWorkstationIOsAPIFromInternal(normalized.OnRejection),
		OnFailure:             optionalWorkstationIOsAPIFromInternal(normalized.OnFailure),
		Resources:             resourceRequirementsAPIFromInternal(normalized.Resources),
		CopyReferencedScripts: boolPtrIfTrue(normalized.CopyReferencedScripts),
		Guards:                workstationGuardsAPIFromInternal(normalized.Guards),
		StopWords:             stringSlicePtr(mergeStopWords(normalized.StopWords, normalized.RuntimeStopWords)),
		Env:                   stringMapPtr(normalized.Env),
		Body:                  stringPtrIfNotEmpty(promptBody),
		Limits:                workstationLimitsAPIFromInternal(normalized.Limits),
		WorkPropagation:       workPropagationAPIFromInternal(normalized.WorkPropagation),
		OutputSchema:          stringPtrIfNotEmpty(normalized.OutputSchema),
		OutcomeFormat:         workstationOutcomeFormatPtrIfNotEmpty(normalized.OutcomeFormat),
		Operation:             stringPtrIfNotEmpty(normalized.Operation),
		OperationBindings:     workstationOperationBindingsAPIFromInternal(normalized.OperationBindings),
		PromptFile:            stringPtrIfNotEmpty(normalized.PromptFile),
		Type:                  workstationTypePtrIfNotEmpty(normalized, workerType),
	}
	if normalized.ID != "" {
		apiWorkstation.Id = stringPtr(normalized.ID)
	}
	if normalized.Kind != "" {
		behavior := publicFactoryWorkstationKindFromInternal(normalized.Kind)
		apiWorkstation.Behavior = &behavior
	}
	if normalized.WorkingDirectory != "" {
		apiWorkstation.WorkingDirectory = stringPtr(normalized.WorkingDirectory)
	}
	if normalized.Worktree != "" {
		apiWorkstation.Worktree = stringPtr(normalized.Worktree)
	}
	if normalized.OpenCodeAgent != "" {
		apiWorkstation.OpenCodeAgent = stringPtr(normalized.OpenCodeAgent)
	}
	return apiWorkstation
}

func resourceManifestAPIFromInternal(manifest *interfaces.PortableResourceManifestConfig) *factoryapi.ResourceManifest {
	if manifest == nil {
		return nil
	}
	return &factoryapi.ResourceManifest{
		RequiredTools: requiredToolsAPIFromInternal(manifest.RequiredTools),
		BundledFiles:  bundledFilesAPIFromInternal(manifest.BundledFiles),
	}
}

func requiredToolsAPIFromInternal(requiredTools []interfaces.RequiredToolConfig) *[]factoryapi.RequiredTool {
	if len(requiredTools) == 0 {
		return nil
	}
	values := make([]factoryapi.RequiredTool, len(requiredTools))
	for i, tool := range requiredTools {
		values[i] = factoryapi.RequiredTool{
			Name:        tool.Name,
			Command:     tool.Command,
			Purpose:     stringPtrIfNotEmpty(tool.Purpose),
			VersionArgs: stringSlicePtr(tool.VersionArgs),
		}
	}
	return &values
}

func bundledFilesAPIFromInternal(bundledFiles []interfaces.BundledFileConfig) *[]factoryapi.BundledFile {
	if len(bundledFiles) == 0 {
		return nil
	}
	sorted := append([]interfaces.BundledFileConfig(nil), bundledFiles...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].TargetPath < sorted[j].TargetPath
	})
	values := make([]factoryapi.BundledFile, len(sorted))
	for i, file := range sorted {
		values[i] = factoryapi.BundledFile{
			Id:         stringPtrIfNotEmpty(interfaces.CanonicalBundledFileID(file.ID, file.TargetPath)),
			Type:       factoryapi.BundledFileType(file.Type),
			TargetPath: file.TargetPath,
			Content:    bundledFileContentAPIFromInternal(file),
		}
	}
	return &values
}

func bundledFileContentAPIFromInternal(file interfaces.BundledFileConfig) factoryapi.BundledFileContent {
	return factoryapi.BundledFileContent{
		Encoding: factoryapi.BundledFileContentEncoding(file.Content.Encoding),
		Inline:   file.Content.Inline,
	}
}

func classificationRoutesAPIFromInternal(routes []interfaces.ClassificationRouteConfig) *[]factoryapi.ClassificationRoute {
	if len(routes) == 0 {
		return nil
	}
	values := make([]factoryapi.ClassificationRoute, len(routes))
	for i, route := range routes {
		values[i] = factoryapi.ClassificationRoute{
			Label:   route.Label,
			Outputs: workstationIOsAPIFromInternal(route.Outputs),
		}
	}
	return &values
}

func dropSupportedPortableBundledInlineContent(payload any) {
	root, ok := payload.(map[string]any)
	if !ok {
		return
	}
	supportingFiles, ok := root["supportingFiles"].(map[string]any)
	if !ok {
		return
	}
	bundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok {
		return
	}
	for _, entry := range bundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		fileType, _ := bundledFile["type"].(string)
		targetPath, _ := bundledFile["targetPath"].(string)
		if !interfaces.ShouldOmitSupportedPortableBundledInline(interfaces.BundledFileConfig{
			Type:       fileType,
			TargetPath: targetPath,
		}) {
			continue
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			continue
		}
		inline, _ := content["inline"].(string)
		if strings.TrimSpace(inline) != "" {
			continue
		}
		delete(content, "inline")
	}
}

// WorkstationConfigToOpenAPI converts an internal workstation config into the
// generated OpenAPI model.
func WorkstationConfigToOpenAPI(workstation interfaces.FactoryWorkstationConfig) factoryapi.Workstation {
	return WorkstationConfigToOpenAPIWithWorkerType(workstation, "")
}

// WorkstationConfigToOpenAPIWithWorkerType converts an internal workstation
// config into the generated OpenAPI model using worker-type context for
// behavior-aware workstation taxonomy projection.
func WorkstationConfigToOpenAPIWithWorkerType(workstation interfaces.FactoryWorkstationConfig, workerType string) factoryapi.Workstation {
	return workstationAPIFromInternal(workstation, workerType)
}

func workerDefinitionAPIFromInternalWithUsage(def *interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) *factoryapi.Worker {
	if def == nil {
		return nil
	}
	return &factoryapi.Worker{
		Id:               stringPtrIfNotEmpty(def.ID),
		Type:             workerTypePtrForFactoryUsage(def, workstations),
		Provider:         hostedWorkerProviderPtrIfNotEmpty(def.Provider),
		Name:             def.Name,
		Args:             stringSlicePtr(def.Args),
		Auth:             hostedWorkerAuthAPIFromInternal(def.Auth),
		Body:             stringPtrIfNotEmpty(def.Body),
		Command:          stringPtrIfNotEmpty(def.Command),
		Linear:           hostedLinearWorkerAPIFromInternal(def.Linear),
		AgentTools:       agentWorkerToolsAPIFromInternal(def.AgentTools),
		Model:            stringPtrIfNotEmpty(def.Model),
		ModelProvider:    workerModelProviderPtrIfNotEmpty(def.ModelProvider),
		ModelLocality:    workerModelLocalityPtrIfNotEmpty(def.ModelLocality),
		ExecutorProvider: workerProviderPtrIfNotEmpty(def.ExecutorProvider),
		Operations:       modelOperationsAPIFromInternal(def.Operations),
		Resources:        resourceRequirementsAPIFromInternal(def.Resources),
		SkipPermissions:  boolPtrIfTrue(def.SkipPermissions),
		OpenCodeAgent:    stringPtrIfNotEmpty(def.OpenCodeAgent),
		StopToken:        stringPtrIfNotEmpty(def.StopToken),
		Timeout:          stringPtrIfNotEmpty(def.Timeout),
	}
}

func hostedWorkerAuthAPIFromInternal(auth *interfaces.HostedWorkerAuthConfig) *factoryapi.HostedWorkerAuth {
	if auth == nil {
		return nil
	}
	return &factoryapi.HostedWorkerAuth{
		SecretRef: stringPtrIfNotEmpty(auth.SecretRef),
	}
}

func hostedLinearWorkerAPIFromInternal(cfg *interfaces.HostedLinearWorkerConfig) *factoryapi.HostedLinearWorkerConfig {
	if cfg == nil {
		return nil
	}
	return &factoryapi.HostedLinearWorkerConfig{
		PollInterval: stringPtrIfNotEmpty(cfg.PollInterval),
		TeamIds:      stringSlicePtr(cfg.TeamIDs),
		StateIds:     stringSlicePtr(cfg.StateIDs),
		Mapping:      hostedLinearWorkerMappingAPIFromInternal(cfg.Mapping),
		Claim:        hostedLinearWorkerClaimAPIFromInternal(cfg.Claim),
	}
}

func hostedLinearWorkerMappingAPIFromInternal(mapping interfaces.HostedLinearWorkerMappingConfig) *factoryapi.HostedLinearWorkerMapping {
	return &factoryapi.HostedLinearWorkerMapping{
		WorkType: stringPtrIfNotEmpty(mapping.WorkType),
		State:    stringPtrIfNotEmpty(mapping.State),
	}
}

func hostedLinearWorkerClaimAPIFromInternal(claim *interfaces.HostedLinearWorkerClaimConfig) *factoryapi.HostedLinearWorkerClaim {
	if claim == nil {
		return nil
	}
	return &factoryapi.HostedLinearWorkerClaim{
		AssigneeField: stringPtrIfNotEmpty(claim.AssigneeField),
	}
}

func agentWorkerToolsAPIFromInternal(cfg *interfaces.AgentToolsConfig) *factoryapi.AgentWorkerToolsConfig {
	if cfg == nil {
		return nil
	}
	policy := interfaces.NormalizeAgentToolPolicy(cfg.Policy)
	if policy == interfaces.AgentToolPolicyDisabled {
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicyDISABLED,
		}
	}
	switch policy {
	case interfaces.AgentToolPolicyReadOnly:
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicyREADONLY,
		}
	case interfaces.AgentToolPolicyEnabled:
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicyENABLED,
		}
	default:
		return &factoryapi.AgentWorkerToolsConfig{
			Policy: factoryapi.AgentWorkerToolPolicy(policy),
		}
	}
}

// WorkerConfigToOpenAPI converts an internal worker config into the generated
// OpenAPI worker model.
func WorkerConfigToOpenAPI(worker interfaces.FactoryWorkerConfig) factoryapi.Worker {
	return WorkerConfigToOpenAPIWithFactoryUsage(worker, nil)
}

// WorkerConfigToOpenAPIWithFactoryUsage converts an internal worker config into
// the generated OpenAPI worker model using workstation references for
// behavior-aware worker taxonomy projection.
func WorkerConfigToOpenAPIWithFactoryUsage(worker interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) factoryapi.Worker {
	return *workerDefinitionAPIFromInternalWithUsage(&worker, workstations)
}

func modelOperationsAPIFromInternal(operations []interfaces.ModelOperation) *[]factoryapi.ModelOperation {
	if len(operations) == 0 {
		return nil
	}
	values := make([]factoryapi.ModelOperation, len(operations))
	for i, operation := range operations {
		values[i] = factoryapi.ModelOperation{
			Name:    operation.Name,
			Inputs:  modelOperationSlotsAPIFromInternal(operation.Inputs),
			Outputs: modelOperationSlotsAPIFromInternal(operation.Outputs),
		}
	}
	return &values
}

func modelOperationSlotsAPIFromInternal(slots []interfaces.ModelOperationSlot) *[]factoryapi.ModelOperationSlot {
	if len(slots) == 0 {
		return nil
	}
	values := make([]factoryapi.ModelOperationSlot, len(slots))
	for i, slot := range slots {
		values[i] = factoryapi.ModelOperationSlot{
			Name:         slot.Name,
			ContentTypes: modelOperationContentTypesAPIFromInternal(slot.ContentTypes),
			Required:     boolPtrIfTrue(slot.Required),
		}
	}
	return &values
}

func modelOperationContentTypesAPIFromInternal(contentTypes []string) []factoryapi.ModelOperationContentType {
	if len(contentTypes) == 0 {
		return nil
	}
	values := make([]factoryapi.ModelOperationContentType, len(contentTypes))
	for i, contentType := range contentTypes {
		values[i] = publicFactoryModelOperationContentTypeFromInternal(contentType)
	}
	return values
}

func workstationLimitsAPIFromInternal(limits interfaces.WorkstationLimits) *factoryapi.WorkstationLimits {
	if limits.MaxRetries == 0 && limits.MaxExecutionTime == "" {
		return nil
	}
	return &factoryapi.WorkstationLimits{
		MaxExecutionTime: stringPtrIfNotEmpty(limits.MaxExecutionTime),
		MaxRetries:       intPtrIfNonZero(limits.MaxRetries),
	}
}

func workPropagationAPIFromInternal(value *interfaces.WorkPropagationConfig) *factoryapi.WorkPropagation {
	if value == nil {
		return nil
	}
	return &factoryapi.WorkPropagation{
		Mode: factoryapi.WorkPropagationMode(value.Mode),
	}
}

func workstationOperationBindingsAPIFromInternal(bindings []interfaces.ModelOperationBinding) *[]factoryapi.WorkstationOperationBinding {
	if len(bindings) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkstationOperationBinding, len(bindings))
	for i, binding := range bindings {
		values[i] = factoryapi.WorkstationOperationBinding{
			Slot:           binding.Slot,
			Config:         contentcontract.GeneratedPtrFromParts(binding.Config),
			DefaultContent: contentcontract.GeneratedPtrFromParts(binding.DefaultContent),
			Selector:       workstationOperationBindingSelectorAPIFromInternal(binding.Selector),
		}
	}
	return &values
}

func workstationOperationBindingSelectorAPIFromInternal(selector *interfaces.ModelOperationBindingSelector) *factoryapi.WorkstationOperationBindingSelector {
	if selector == nil {
		return nil
	}
	return &factoryapi.WorkstationOperationBindingSelector{
		Slot:  stringPtrIfNotEmpty(selector.Slot),
		Label: stringPtrIfNotEmpty(selector.Label),
		Type:  modelOperationContentTypePtrIfNotEmpty(selector.Type),
		Role:  stringPtrIfNotEmpty(selector.Role),
	}
}

func workstationCronAPIFromInternal(cron *interfaces.CronConfig) *factoryapi.WorkstationCron {
	if cron == nil {
		return nil
	}
	return &factoryapi.WorkstationCron{
		ExpiryWindow:   stringPtrIfNotEmpty(cron.ExpiryWindow),
		Jitter:         stringPtrIfNotEmpty(cron.Jitter),
		Schedule:       cron.Schedule,
		TriggerAtStart: boolPtrIfTrue(cron.TriggerAtStart),
	}
}

func workstationIOsAPIFromInternal(configs []interfaces.IOConfig) []factoryapi.WorkstationIO {
	values := make([]factoryapi.WorkstationIO, len(configs))
	for i, cfg := range configs {
		values[i] = workstationIOAPIFromInternal(cfg)
	}
	return values
}

func optionalWorkstationIOsAPIFromInternal(configs []interfaces.IOConfig) *[]factoryapi.WorkstationIO {
	if len(configs) == 0 {
		return nil
	}
	values := workstationIOsAPIFromInternal(configs)
	return &values
}

func workstationIOAPIFromInternal(cfg interfaces.IOConfig) factoryapi.WorkstationIO {
	apiIO := factoryapi.WorkstationIO{
		State:    cfg.StateName,
		WorkType: cfg.WorkTypeName,
	}
	if cfg.Guard != nil {
		guards := []factoryapi.InputGuard{inputGuardAPIFromInternal(*cfg.Guard)}
		apiIO.Guards = &guards
	}
	return apiIO
}

func inputGuardAPIFromInternal(guard interfaces.InputGuardConfig) factoryapi.InputGuard {
	apiGuard := factoryapi.InputGuard{
		Type: publicInputGuardTypeFromInternal(guard.Type),
	}
	if guard.MatchInput != "" {
		apiGuard.MatchInput = stringPtr(guard.MatchInput)
	}
	if guard.ParentInput != "" {
		apiGuard.ParentInput = stringPtr(guard.ParentInput)
	}
	if guard.SpawnedBy != "" {
		apiGuard.SpawnedBy = stringPtr(guard.SpawnedBy)
	}
	return apiGuard
}

func resourceRequirementsAPIFromInternal(resources []interfaces.ResourceConfig) *[]factoryapi.ResourceRequirement {
	if len(resources) == 0 {
		return nil
	}
	values := make([]factoryapi.ResourceRequirement, len(resources))
	for i, resource := range resources {
		values[i] = factoryapi.ResourceRequirement{
			Name:     resource.Name,
			Capacity: resource.Capacity,
		}
	}
	return &values
}

func resourceTypePtrIfNotEmpty(value string) *factoryapi.ResourceType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	canonical := factoryapi.ResourceType(publicFactoryResourceTypeFromInternal(value))
	return &canonical
}

func workstationGuardsAPIFromInternal(guards []interfaces.GuardConfig) *[]factoryapi.WorkstationGuard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.WorkstationGuard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.WorkstationGuard{
			Type:        publicWorkstationGuardTypeFromInternal(guard.Type),
			Workstation: stringPtrIfNotEmpty(guard.Workstation),
			MaxVisits:   intPtrIfNonZero(guard.MaxVisits),
			MatchConfig: guardMatchConfigAPIFromInternal(guard.MatchConfig),
		}
	}
	return &values
}

func factoryGuardsAPIFromInternal(guards []interfaces.FactoryGuardConfig) *[]factoryapi.FactoryGuard {
	if len(guards) == 0 {
		return nil
	}
	values := make([]factoryapi.FactoryGuard, len(guards))
	for i, guard := range guards {
		values[i] = factoryapi.FactoryGuard{
			Type:          publicFactoryRootGuardTypeFromInternal(guard.Type),
			ModelProvider: publicFactoryWorkerModelProviderFromInternal(guard.ModelProvider),
			Model:         stringPtrIfNotEmpty(guard.Model),
			RefreshWindow: guard.RefreshWindow,
		}
	}
	return &values
}

func guardMatchConfigAPIFromInternal(matchConfig *interfaces.GuardMatchConfig) *factoryapi.GuardMatchConfig {
	if matchConfig == nil {
		return nil
	}
	return &factoryapi.GuardMatchConfig{
		InputKey: matchConfig.InputKey,
	}
}

func workerTypePtrForFactoryUsage(def *interfaces.FactoryWorkerConfig, workstations []interfaces.FactoryWorkstationConfig) *factoryapi.WorkerType {
	if def == nil || strings.TrimSpace(def.Type) == "" {
		return nil
	}
	publicType := workercompatibility.PublicWorkerTypeForFactoryUsage(*def, compatibilityWorkstations(workstations))
	enumValue := factoryapi.WorkerType(publicType)
	return &enumValue
}

func compatibilityWorkstations(values []interfaces.FactoryWorkstationConfig) []workercompatibility.Workstation {
	if len(values) == 0 {
		return nil
	}
	workstations := make([]workercompatibility.Workstation, len(values))
	for i, value := range values {
		workstations[i] = workercompatibility.Workstation{
			Name: value.Name, Type: value.Type, Kind: value.Kind, WorkerTypeName: value.WorkerTypeName,
		}
	}
	return workstations
}

func workerModelProviderPtrIfNotEmpty(value string) *factoryapi.WorkerModelProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerModelProviderFromInternal(value)
	return &enumValue
}

func workerModelLocalityPtrIfNotEmpty(value string) *factoryapi.WorkerModelLocality {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerModelLocalityFromInternal(value)
	return &enumValue
}

func workerProviderPtrIfNotEmpty(value string) *factoryapi.WorkerProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryWorkerProviderFromInternal(value)
	return &enumValue
}

func hostedWorkerProviderPtrIfNotEmpty(value string) *factoryapi.HostedWorkerProvider {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := factoryapi.HostedWorkerProvider(interfaces.PermissivePublicFactoryHostedWorkerProvider(value))
	return &converted
}

func workstationOutcomeFormatPtrIfNotEmpty(value string) *factoryapi.WorkstationOutcomeFormat {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	converted := factoryapi.WorkstationOutcomeFormat(interfaces.PermissivePublicFactoryWorkstationOutcomeFormat(value))
	return &converted
}

func workstationTypePtrIfNotEmpty(workstation interfaces.FactoryWorkstationConfig, workerType string) *factoryapi.WorkstationType {
	publicType := interfaces.PublicWorkstationTypeFromInternalRuntime(workstation.Type, workerType, workstation.Kind)
	if strings.TrimSpace(publicType) == "" {
		return nil
	}
	enumValue := publicFactoryWorkstationTypeFromInternal(workstation, workerType)
	return &enumValue
}

func modelOperationContentTypePtrIfNotEmpty(value string) *factoryapi.ModelOperationContentType {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	enumValue := publicFactoryModelOperationContentTypeFromInternal(value)
	return &enumValue
}

func valueOrEmpty[T ~string](value *T) T {
	if value == nil {
		return ""
	}
	return *value
}

func stringPtr(value string) *string {
	return &value
}

func stringPtrIfNotEmpty(value string) *string {
	return optional.NonEmptyStringPtr(value)
}

func stringSlicePtr(values []string) *[]string {
	return optional.CopiedStringsPtr(values)
}

func stringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	copied := factoryapi.StringMap(cloneStringMap(values))
	return &copied
}

func intPtrIfNonZero(value int) *int {
	return optional.NonZeroIntPtr(value)
}

func boolPtrIfTrue(value bool) *bool {
	return optional.TrueBoolPtr(value)
}

func stringValue(value *string) string {
	return optional.StringValue(value)
}

func stringSliceValue(values *[]string) []string {
	return optional.StringsValue(values)
}

func stringMapValue(values *factoryapi.StringMap) map[string]string {
	return optional.StringMapValue(values)
}

func intValue(value *int) int {
	return optional.IntValue(value)
}

func boolValue(value *bool) bool {
	return optional.BoolValue(value)
}
