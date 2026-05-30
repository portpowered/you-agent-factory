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
	if err := normalizeFactoryEnumObjectFieldWithNormalizer(root, "runner", "runner", interfaces.StrictPublicFactoryRunnerID); err != nil {
		return nil, err
	}
	if err := normalizeFactoryGuardEntries(root); err != nil {
		return nil, err
	}
	if err := normalizeFactoryInputTypeEntries(root); err != nil {
		return nil, err
	}
	if err := normalizeFactoryWorkTypeEntries(root); err != nil {
		return nil, err
	}
	if err := normalizeFactoryResourceEntries(root); err != nil {
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

func normalizeFactoryResourceEntries(root map[string]any) error {
	resources, ok := root["resources"].([]any)
	if !ok {
		return nil
	}
	for i, item := range resources {
		resource, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(resource, "type", fmt.Sprintf("resources[%d].type", i), interfaces.StrictPublicFactoryResourceType); err != nil {
			return err
		}
	}
	return nil
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

func normalizeFactoryWorkTypeEntries(root map[string]any) error {
	workTypes, ok := root["workTypes"].([]any)
	if !ok {
		return nil
	}
	for workTypeIndex, item := range workTypes {
		workType, ok := item.(map[string]any)
		if !ok {
			continue
		}
		behaviors, ok := workType["handlingBehavior"].([]any)
		if !ok {
			continue
		}
		for behaviorIndex, behaviorAny := range behaviors {
			behavior, ok := behaviorAny.(string)
			if !ok {
				continue
			}
			canonical := interfaces.StrictPublicWorkTypeHandlingBehavior(behavior)
			if canonical == "" {
				return fmt.Errorf("workTypes[%d].handlingBehavior[%d]: unsupported value %q", workTypeIndex, behaviorIndex, behavior)
			}
			behaviors[behaviorIndex] = canonical
		}
		workType["handlingBehavior"] = behaviors
	}
	return nil
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
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(worker, "provider", fmt.Sprintf("workers[%d].provider", i), interfaces.StrictPublicFactoryHostedWorkerProvider); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(worker, "modelLocality", fmt.Sprintf("workers[%d].modelLocality", i), interfaces.StrictPublicFactoryWorkerModelLocality); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(worker, "executorProvider", fmt.Sprintf("workers[%d].executorProvider", i), interfaces.StrictPublicFactoryWorkerProvider); err != nil {
			return err
		}
		if err := normalizeFactoryWorkerOperationEntries(worker, i); err != nil {
			return err
		}
		if err := rejectUnsupportedHostedWorkerBoundaryFields(worker, fmt.Sprintf("workers[%d]", i)); err != nil {
			return err
		}
		normalizeRuntimeResourceRequirements(worker, "resources")
	}
	return nil
}

func rejectUnsupportedHostedWorkerBoundaryFields(worker map[string]any, path string) error {
	auth, _ := worker["auth"].(map[string]any)
	if len(auth) == 0 {
		return nil
	}
	return rejectRetiredBoundaryFields(auth, path+".auth", []retiredBoundaryField{
		{key: "apiKey", replacement: "v1 hosted workers accept only auth.secretRef"},
		{key: "api_key", replacement: "v1 hosted workers accept only auth.secretRef"},
		{key: "token", replacement: "v1 hosted workers accept only auth.secretRef"},
		{key: "accessToken", replacement: "v1 hosted workers accept only auth.secretRef"},
		{key: "access_token", replacement: "v1 hosted workers accept only auth.secretRef"},
		{key: "clientId", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "client_id", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "clientSecret", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "client_secret", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "refreshToken", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "refresh_token", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "tokenUrl", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
		{key: "token_url", replacement: "v1 hosted workers do not support OAuth; use auth.secretRef"},
	})
}

func normalizeFactoryWorkerOperationEntries(worker map[string]any, workerIndex int) error {
	operations, ok := worker["operations"].([]any)
	if !ok {
		return nil
	}
	for operationIndex, operationAny := range operations {
		operation, ok := operationAny.(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFactoryModelOperationName(operation, "name", fmt.Sprintf("workers[%d].operations[%d].name", workerIndex, operationIndex)); err != nil {
			return err
		}
		if err := normalizeFactoryModelOperationSlots(operation, "inputs", fmt.Sprintf("workers[%d].operations[%d].inputs", workerIndex, operationIndex)); err != nil {
			return err
		}
		if err := normalizeFactoryModelOperationSlots(operation, "outputs", fmt.Sprintf("workers[%d].operations[%d].outputs", workerIndex, operationIndex)); err != nil {
			return err
		}
	}
	return nil
}

func normalizeFactoryModelOperationSlots(operation map[string]any, key string, fieldPath string) error {
	slots, ok := operation[key].([]any)
	if !ok {
		return nil
	}
	for slotIndex, slotAny := range slots {
		slot, ok := slotAny.(map[string]any)
		if !ok {
			continue
		}
		contentTypes, ok := slot["contentTypes"].([]any)
		if !ok {
			continue
		}
		for contentTypeIndex, contentTypeAny := range contentTypes {
			contentType, ok := contentTypeAny.(string)
			if !ok {
				continue
			}
			canonical := interfaces.StrictPublicFactoryWorkerModelOperationContentType(contentType)
			if canonical == "" {
				return fmt.Errorf("%s[%d].contentTypes[%d]: unsupported value %q", fieldPath, slotIndex, contentTypeIndex, contentType)
			}
			contentTypes[contentTypeIndex] = canonical
		}
		slot["contentTypes"] = contentTypes
	}
	return nil
}

func normalizeFactoryModelOperationName(container map[string]any, key string, fieldPath string) error {
	raw, ok := container[key]
	if !ok {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		container[key] = ""
		return nil
	}
	if !isUppercaseOperationIdentifier(trimmed) {
		return fmt.Errorf("%s: unsupported value %q", fieldPath, value)
	}
	container[key] = trimmed
	return nil
}

func isUppercaseOperationIdentifier(value string) bool {
	for i, r := range value {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if i > 0 && r >= '0' && r <= '9' {
			continue
		}
		if i > 0 && r == '_' {
			continue
		}
		return false
	}
	return value != ""
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
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(workstation, "behavior", fmt.Sprintf("workstations[%d].behavior", i), func(value string) string {
			return interfaces.StrictPublicWorkstationKind(value)
		}); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(workstation, "runner", fmt.Sprintf("workstations[%d].runner", i), interfaces.StrictPublicFactoryRunnerID); err != nil {
			return err
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(workstation, "type", fmt.Sprintf("workstations[%d].type", i), interfaces.StrictPublicFactoryWorkstationType); err != nil {
			return err
		}
		if err := normalizeFactoryModelOperationName(workstation, "operation", fmt.Sprintf("workstations[%d].operation", i)); err != nil {
			return err
		}
		if err := normalizeFactoryWorkstationOperationBindings(workstation, i); err != nil {
			return err
		}
		if err := normalizeFactoryWorkstationGuardEntries(workstation, i); err != nil {
			return err
		}
		if err := normalizeFactoryWorkstationInputGuardEntries(workstation, i); err != nil {
			return err
		}
		normalizeRuntimeResourceRequirements(workstation, "resources")
	}
	return nil
}

func normalizeFactoryWorkstationOperationBindings(workstation map[string]any, workstationIndex int) error {
	bindings, ok := workstation["operationBindings"].([]any)
	if !ok {
		return nil
	}
	for bindingIndex, bindingAny := range bindings {
		binding, ok := bindingAny.(map[string]any)
		if !ok {
			continue
		}
		selector, ok := binding["selector"].(map[string]any)
		if !ok {
			continue
		}
		if err := normalizeFactoryEnumObjectFieldWithNormalizer(
			selector,
			"type",
			fmt.Sprintf("workstations[%d].operationBindings[%d].selector.type", workstationIndex, bindingIndex),
			interfaces.StrictPublicFactoryWorkerModelOperationContentType,
		); err != nil {
			return err
		}
	}
	return nil
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

type retiredBoundaryField struct {
	key         string
	replacement string
}

var retiredFactoryBoundaryFields = []retiredBoundaryField{
	{key: "project", replacement: "use id"},
	{key: "factoryDir", replacement: "use factoryDirectory"},
	{key: "factory_dir", replacement: "use factoryDirectory"},
	{key: "resourceManifest", replacement: "use supportingFiles"},
	{key: "resource_manifest", replacement: "use supportingFiles"},
	{key: "workflowId", replacement: "remove workflowId"},
	{key: "workflow_id", replacement: "remove workflowId"},
}

var retiredWorkerBoundaryFields = []retiredBoundaryField{
	{key: "model_provider", replacement: "use modelProvider"},
	{key: "sessionId", replacement: "remove sessionId; provider sessions are runtime-owned"},
	{key: "session_id", replacement: "remove sessionId; provider sessions are runtime-owned"},
	{key: "concurrency", replacement: "remove concurrency; use resources to limit concurrent work"},
}

var retiredWorkstationBoundaryFields = []retiredBoundaryField{
	{key: "kind", replacement: "use behavior"},
	{key: "runtimeType", replacement: "use type"},
	{key: "runtime_type", replacement: "use type"},
	{key: "resourceUsage", replacement: "use resources"},
	{key: "resource_usage", replacement: "use resources"},
	{key: "resource-usage", replacement: "use resources"},
	{key: "stopToken", replacement: "use stopWords"},
	{key: "stop_token", replacement: "use stopWords"},
	{key: "runtimeStopWords", replacement: "use stopWords"},
	{key: "runtime_stop_words", replacement: "use stopWords"},
	{key: "timeout", replacement: "use limits.maxExecutionTime"},
}

func rejectRetiredFanInField(data []byte) error {
	var payload struct {
		Workstations []map[string]json.RawMessage `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	for index, workstation := range payload.Workstations {
		if _, ok := workstation["join"]; ok {
			return fmt.Errorf("workstations[%d].join is not supported; use per-input guards", index)
		}
	}
	return nil
}

func rejectRetiredExhaustionRulesField(data []byte) error {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if _, ok := payload["exhaustionRules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	if _, ok := payload["exhaustion_rules"]; ok {
		return fmt.Errorf("exhaustion_rules is retired; use a guarded LOGICAL_MOVE workstation with a visit_count guard instead")
	}
	return nil
}

func rejectRetiredCronIntervalField(data []byte) error {
	var payload struct {
		Workstations []struct {
			Cron *interfaces.CronConfig `json:"cron"`
		} `json:"workstations"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	for index, workstation := range payload.Workstations {
		if workstation.Cron != nil && workstation.Cron.HasUnsupportedInterval() {
			return fmt.Errorf("workstations[%d].cron.interval is not supported; use cron.schedule", index)
		}
	}
	return nil
}

func rejectRetiredGeneratedBoundaryAliases(data []byte) error {
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil
	}
	if err := rejectRetiredBoundaryFields(payload, "factory", retiredFactoryBoundaryFields); err != nil {
		return err
	}
	if err := rejectRetiredWorkerBoundaryAliases(payload); err != nil {
		return err
	}
	if err := rejectRetiredWorkstationBoundaryAliases(payload); err != nil {
		return err
	}
	return nil
}

func rejectRetiredWorkerBoundaryAliases(root map[string]any) error {
	workers, ok := root["workers"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workers {
		worker, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectRetiredWorkerBoundaryObject(worker, fmt.Sprintf("workers[%d]", index), true); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredWorkerBoundaryObject(worker map[string]any, path string, includeDefinition bool) error {
	if err := rejectRetiredHostedProviderAlias(worker, path); err != nil {
		return err
	}
	if err := rejectRetiredBoundaryFields(worker, path, retiredWorkerBoundaryFields); err != nil {
		return err
	}
	if !includeDefinition {
		return nil
	}
	definition, ok := worker["definition"].(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredWorkerBoundaryObject(definition, path+".definition", false)
}

func rejectRetiredHostedProviderAlias(worker map[string]any, path string) error {
	rawProvider, hasProvider := worker["provider"]
	if !hasProvider {
		return nil
	}
	provider, _ := rawProvider.(string)
	workerType, _ := worker["type"].(string)
	if interfaces.StrictPublicFactoryWorkerType(workerType) == interfaces.WorkerTypeHosted &&
		interfaces.StrictPublicFactoryHostedWorkerProvider(provider) != "" {
		return nil
	}
	return fmt.Errorf("%s.provider is not supported; use executorProvider", path)
}

func rejectRetiredWorkstationBoundaryAliases(root map[string]any) error {
	workstations, ok := root["workstations"].([]any)
	if !ok {
		return nil
	}
	for index, item := range workstations {
		workstation, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if err := rejectRetiredWorkstationBoundaryObject(workstation, fmt.Sprintf("workstations[%d]", index), true); err != nil {
			return err
		}
	}
	return nil
}

func rejectRetiredWorkstationBoundaryObject(workstation map[string]any, path string, includeDefinition bool) error {
	if err := rejectRetiredBoundaryFields(workstation, path, retiredWorkstationBoundaryFields); err != nil {
		return err
	}
	if err := rejectRetiredCronBoundaryAliases(workstation["cron"], path+".cron"); err != nil {
		return err
	}
	if !includeDefinition {
		return nil
	}
	definition, ok := workstation["definition"].(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredWorkstationBoundaryObject(definition, path+".definition", false)
}

func rejectRetiredCronBoundaryAliases(raw any, path string) error {
	cron, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	return rejectRetiredBoundaryFields(cron, path, []retiredBoundaryField{
		{key: "trigger_at_start", replacement: "use triggerAtStart"},
		{key: "expiry_window", replacement: "use expiryWindow"},
	})
}

func rejectRetiredBoundaryFields(container map[string]any, path string, fields []retiredBoundaryField) error {
	for _, field := range fields {
		if _, ok := container[field.key]; ok {
			return fmt.Errorf("%s.%s is not supported; %s", path, field.key, field.replacement)
		}
	}
	return nil
}

// LoadFromCanonicalJSON normalizes canonical factory JSON, expands it to a
// FactoryConfig, and runs the same blocking load validation used for inline
// factory directories. It does not require an on-disk factory directory; split
// worker/workstation AGENTS.md layouts still need LoadFromFactoryDir.
func LoadFromCanonicalJSON(payload []byte, workstationLoader WorkstationLoader) (*LoadedFactoryConfig, error) {
	factoryCfg, _, err := normalizeNamedFactoryPayload("factory", payload)
	if err != nil {
		return nil, err
	}
	if err := validatePortableBundledFilesForExpandOnPath("", factoryCfg); err != nil {
		return nil, err
	}
	if err := validateBlockingFactoryLoad(factoryCfg); err != nil {
		return nil, err
	}

	inlineDefinitionsRequired := hasInlineRuntimeDefinitions(factoryCfg)
	runtimeDefs, err := loadRuntimeDefinitionLookupMapsFromFactoryConfig("", factoryCfg, InlineRuntimeDefinitionOptions{
		RequireSplitDefinitions: inlineDefinitionsRequired,
		WorkstationLoader:       workstationLoader,
	})
	if err != nil {
		return nil, err
	}

	return NewLoadedFactoryConfig("", factoryCfg, runtimeDefs)
}
