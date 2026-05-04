package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"unicode"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var defaultFactoryConfigMapper = NewFactoryConfigMapper()

type rawOpenAPIFactory struct {
	Workstations []rawOpenAPIWorkstation `json:"workstations"`
}

type rawOpenAPIWorkstation struct {
	ID   string                 `json:"id"`
	Name string                 `json:"name"`
	Cron *interfaces.CronConfig `json:"cron"`
}

// GeneratedFactoryFromOpenAPIJSON converts an OpenAPI-compatible factory JSON
// payload into the generated Factory boundary model.
func GeneratedFactoryFromOpenAPIJSON(data []byte) (factoryapi.Factory, error) {
	boundary, err := decodeGeneratedFactoryBoundaryJSON(data)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return boundary.generated, nil
}

// FactoryConfigFromOpenAPIJSON converts an OpenAPI-compatible factory JSON payload
// into the internal config representation used by runtime mappers and tests.
func FactoryConfigFromOpenAPIJSON(data []byte) (*interfaces.FactoryConfig, error) {
	generated, err := GeneratedFactoryFromOpenAPIJSON(data)
	if err != nil {
		return nil, err
	}
	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// MarshalCanonicalFactoryConfig serializes factory config using camelCase keys across
// factory/workstation/work structures for deterministic canonical output.
func MarshalCanonicalFactoryConfig(cfg *interfaces.FactoryConfig) ([]byte, error) {
	return defaultFactoryConfigMapper.Flatten(cfg)
}

func normalizeFactoryInputJSON(data []byte) ([]byte, error) {
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("decode factory input payload: %w", err)
	}
	normalized, err := normalizeCanonicalFactoryInputFields(decoded)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("normalize factory input payload: %w", err)
	}
	return raw, nil
}

func normalizeFactoryOutputJSONKeys(v any) any {
	return normalizeFactoryOutputJSONKeysForField(v, "")
}

func normalizeFactoryOutputJSONKeysForField(v any, fieldName string) any {
	switch typed := v.(type) {
	case map[string]any:
		if preservesObjectKeys(fieldName) {
			return normalizeFactoryJSONValuesPreservingKeys(typed)
		}
		return normalizeFactoryConfigObjectKeys(typed, normalizeFactoryOutputJSONKeysForField)
	case []any:
		values := make([]any, len(typed))
		for i, value := range typed {
			values[i] = normalizeFactoryOutputJSONKeysForField(value, fieldName)
		}
		return values
	default:
		return v
	}
}

func normalizeFactoryConfigObjectKeys(values map[string]any, normalizeValue func(any, string) any) map[string]any {
	keys := sortedFactoryConfigKeys(values)
	normalized := make(map[string]any, len(values))

	for _, key := range keys {
		normalizedKey := canonicalFactoryConfigKey(key)
		if key != normalizedKey {
			continue
		}
		normalized[normalizedKey] = normalizeValue(values[key], normalizedKey)
	}
	for _, key := range keys {
		normalizedKey := canonicalFactoryConfigKey(key)
		if key == normalizedKey {
			continue
		}
		if _, exists := normalized[normalizedKey]; exists {
			continue
		}
		normalized[normalizedKey] = normalizeValue(values[key], normalizedKey)
	}

	return normalized
}

func sortedFactoryConfigKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func normalizeFactoryJSONValuesPreservingKeys(values map[string]any) map[string]any {
	normalized := make(map[string]any, len(values))
	for key, value := range values {
		normalized[key] = normalizeFactoryJSONValuePreservingKeys(value)
	}
	return normalized
}

func normalizeFactoryJSONValuePreservingKeys(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return normalizeFactoryJSONValuesPreservingKeys(typed)
	case []any:
		values := make([]any, len(typed))
		for i, item := range typed {
			values[i] = normalizeFactoryJSONValuePreservingKeys(item)
		}
		return values
	default:
		return value
	}
}

func preservesObjectKeys(fieldName string) bool {
	return fieldName == "env" || fieldName == "metadata"
}

func normalizeCanonicalFactoryInputFields(v any) (any, error) {
	root, ok := v.(map[string]any)
	if !ok {
		return v, nil
	}
	if err := normalizeFactoryGuardEntries(root); err != nil {
		return nil, err
	}
	if err := normalizeFactoryInputTypeEntries(root); err != nil {
		return nil, err
	}
	if err := normalizeFactoryWorkerEntries(root); err != nil {
		return nil, err
	}
	if err := normalizeFactoryWorkstationEntries(root); err != nil {
		return nil, err
	}
	return v, nil
}

func normalizeFactoryGuardEntries(root map[string]any) error {
	guards, ok := root["guards"].([]any)
	if !ok {
		return nil
	}
	for i, item := range guards {
		guard, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFactoryEnumObjectField(guard, "type", fmt.Sprintf("guards[%d].type", i), publicFactoryRootGuardTypeAliases); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(guard, "modelProvider", fmt.Sprintf("guards[%d].modelProvider", i), interfaces.StrictPublicFactoryWorkerModelProvider); err != nil {
			return err
		}
		if err := rejectUnsupportedFactoryGuardBoundaryFields(guard, fmt.Sprintf("guards[%d]", i)); err != nil {
			return err
		}
	}
	return nil
}

func rejectUnsupportedFactoryGuardBoundaryFields(guard map[string]any, path string) error {
	return rejectRetiredBoundaryFields(guard, path, []retiredBoundaryField{
		{key: "workstation", replacement: "factory guards support modelProvider, optional model, and refreshWindow"},
		{key: "maxVisits", replacement: "factory guards support modelProvider, optional model, and refreshWindow"},
		{key: "matchConfig", replacement: "factory guards support modelProvider, optional model, and refreshWindow"},
		{key: "matchInput", replacement: "factory guards support modelProvider, optional model, and refreshWindow"},
		{key: "parentInput", replacement: "factory guards support modelProvider, optional model, and refreshWindow"},
		{key: "spawnedBy", replacement: "factory guards support modelProvider, optional model, and refreshWindow"},
	})
}

func normalizeFactoryInputTypeEntries(root map[string]any) error {
	inputTypes, ok := root["inputTypes"].([]any)
	if !ok {
		return nil
	}
	for i, item := range inputTypes {
		inputType, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFactoryEnumObjectField(inputType, "type", fmt.Sprintf("inputTypes[%d].type", i), publicFactoryInputKindAliases); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFactoryWorkerEntries(root map[string]any) error {
	workers, ok := root["workers"].([]any)
	if !ok {
		return nil
	}
	for i, item := range workers {
		worker, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mergeInlineDefinitionFields(worker)
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(worker, "type", fmt.Sprintf("workers[%d].type", i), interfaces.StrictPublicFactoryWorkerType); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(worker, "modelProvider", fmt.Sprintf("workers[%d].modelProvider", i), interfaces.StrictPublicFactoryWorkerModelProvider); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(worker, "executorProvider", fmt.Sprintf("workers[%d].executorProvider", i), interfaces.StrictPublicFactoryWorkerProvider); err != nil {
			return err
		}
		normalizeRuntimeResourceRequirements(worker, "resources")
	}
	return nil
}

func normalizeFactoryWorkstationEntries(root map[string]any) error {
	workstations, ok := root["workstations"].([]any)
	if !ok {
		return nil
	}
	for i, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mergeInlineDefinitionFields(workstation)
		if err := normalizeFactoryEnumObjectField(workstation, "behavior", fmt.Sprintf("workstations[%d].behavior", i), publicFactoryWorkstationKindAliases); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(workstation, "type", fmt.Sprintf("workstations[%d].type", i), interfaces.StrictPublicFactoryWorkstationType); err != nil {
			return err
		}
		if err := normalizeFactoryWorkstationGuardEntries(workstation, i); err != nil {
			return err
		}
		if err := normalizeFactoryWorkstationInputGuardEntries(workstation, i); err != nil {
			return err
		}
		normalizeWorkstationIORouteField(workstation, "onContinue")
		normalizeWorkstationIORouteField(workstation, "onRejection")
		normalizeWorkstationIORouteField(workstation, "onFailure")
		normalizeRuntimeResourceRequirements(workstation, "resources")
	}
	return nil
}

func normalizeWorkstationIORouteField(workstation map[string]any, key string) {
	value, ok := workstation[key]
	if !ok {
		return
	}
	switch typed := value.(type) {
	case nil, []any:
		return
	case map[string]any:
		workstation[key] = []any{typed}
	default:
		workstation[key] = value
	}
}

func normalizeFactoryWorkstationGuardEntries(workstation map[string]any, workstationIndex int) error {
	guards, ok := workstation["guards"].([]any)
	if !ok {
		return nil
	}
	for guardIndex, guardAny := range guards {
		guard, ok := guardAny.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFactoryEnumObjectField(guard, "type", fmt.Sprintf("workstations[%d].guards[%d].type", workstationIndex, guardIndex), publicFactoryWorkstationGuardTypeAliases); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFactoryWorkstationInputGuardEntries(workstation map[string]any, workstationIndex int) error {
	inputs, ok := workstation["inputs"].([]any)
	if !ok {
		return nil
	}
	for inputIndex, inputAny := range inputs {
		input, ok := inputAny.(map[string]any)
		if !ok {
			continue
		}
		guards, ok := input["guards"].([]any)
		if !ok {
			continue
		}
		for guardIndex, guardAny := range guards {
			guard, ok := guardAny.(map[string]any)
			if !ok {
				continue
			}
			if err := normalizeFactoryEnumObjectField(guard, "type", fmt.Sprintf("workstations[%d].inputs[%d].guards[%d].type", workstationIndex, inputIndex, guardIndex), publicFactoryInputGuardTypeAliases); err != nil {
				return err
			}
		}
	}
	return nil
}

func normalizeFactoryEnumObjectField(container map[string]any, key string, fieldPath string, aliases map[string]string) error {
	if err := normalizePublicFactoryEnumValueInObject(container, key, aliases); err != nil {
		return fmt.Errorf("%s: %w", fieldPath, err)
	}
	return nil
}

func normalizeFactoryEnumObjectFieldWithNormalizer(container map[string]any, key string, fieldPath string, normalize func(string) string) error {
	if err := normalizePublicFactoryEnumValueInObjectWith(container, key, normalize); err != nil {
		return fmt.Errorf("%s: %w", fieldPath, err)
	}
	return nil
}

func normalizeRuntimeResourceRequirements(container map[string]any, key string) {
	value, ok := container[key]
	if !ok {
		return
	}
	container[key] = runtimeResourceRequirementsFromBoundaryValue(value)
}

func runtimeResourceRequirementsFromBoundaryValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []any:
		resources := make([]any, 0, len(typed))
		for _, item := range typed {
			resource, ok := runtimeResourceRequirementFromBoundaryItem(item)
			if !ok {
				continue
			}
			resources = append(resources, resource)
		}
		return resources
	default:
		if resource, ok := runtimeResourceRequirementFromBoundaryItem(value); ok {
			return []any{resource}
		}
		return value
	}
}

func runtimeResourceRequirementFromBoundaryItem(value any) (map[string]any, bool) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil, false
		}
		return map[string]any{
			"name":     typed,
			"capacity": 1,
		}, true
	case map[string]any:
		return typed, true
	default:
		return nil, false
	}
}

func mergeInlineDefinitionFields(container map[string]any) {
	definition, ok := container["definition"].(map[string]any)
	if !ok {
		return
	}
	for key, value := range definition {
		if _, exists := container[key]; exists {
			continue
		}
		container[key] = value
	}
	delete(container, "definition")
}

func applyOpenAPICronCompatibility(cfg *interfaces.FactoryConfig, raw []byte) {
	if cfg == nil {
		return
	}
	rawCronByWorkstation := buildRawOpenAPIWorkstationCronIndex(raw)
	for wi := range cfg.Workstations {
		ws := &cfg.Workstations[wi]
		cron := rawCronByWorkstation[ws.Name]
		if cron == nil && ws.ID != "" {
			cron = rawCronByWorkstation[ws.ID]
		}
		if cron == nil {
			continue
		}
		copied := *cron
		ws.Cron = &copied
	}
}

func buildRawOpenAPIWorkstationCronIndex(raw []byte) map[string]*interfaces.CronConfig {
	var cfg rawOpenAPIFactory
	_ = json.Unmarshal(raw, &cfg)

	index := make(map[string]*interfaces.CronConfig)
	for _, ws := range cfg.Workstations {
		if ws.Cron == nil {
			continue
		}
		index[ws.Name] = ws.Cron
		if ws.ID != "" {
			index[ws.ID] = ws.Cron
		}
	}
	return index
}

func canonicalFactoryConfigKey(key string) string {
	if strings.ContainsAny(key, "_-") {
		return toCamelCase(key)
	}
	return key
}

func toCamelCase(key string) string {
	parts := strings.FieldsFunc(key, func(r rune) bool {
		return r == '_' || r == '-'
	})
	if len(parts) == 0 {
		return key
	}

	var builder strings.Builder
	for i, part := range parts {
		if part == "" {
			continue
		}
		lower := strings.ToLower(part)
		if i == 0 {
			builder.WriteString(lower)
			continue
		}
		runes := []rune(lower)
		runes[0] = unicode.ToUpper(runes[0])
		builder.WriteString(string(runes))
	}
	return builder.String()
}
