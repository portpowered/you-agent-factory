// Package globalconfig owns the generated GlobalConfig codec for Operator
// Settings. It keeps generated-contract decoding, representation mapping, and
// canonical document encoding at the Operator Settings transport boundary.
package globalconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Decode decodes one generated GlobalConfig document and maps it to normalized
// Operator Settings values. Unknown object fields are ignored; use
// DecodeWithDiagnostics when the owning caller needs their safe JSON paths.
func Decode(data []byte) (operatorsettings.Config, error) {
	config, _, err := DecodeWithDiagnostics(data)
	return config, err
}

// DecodeWithDiagnostics decodes one generated GlobalConfig document, maps it
// to normalized Operator Settings values, and reports sorted unique paths for
// unknown object fields. Known-field validation and exactly-one-document
// enforcement remain strict.
func DecodeWithDiagnostics(
	data []byte,
) (operatorsettings.Config, operatorsettings.ConfigDecodeDiagnostics, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, fmt.Errorf("decode generated global config: %w", err)
	}
	normalized, err := canonicalizeKnownJSONFieldNames(raw)
	if err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, fmt.Errorf("decode generated global config: %w", err)
	}

	var generated *factoryapi.GlobalConfig
	if err := json.Unmarshal(normalized, &generated); err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, fmt.Errorf("decode generated global config: %w", err)
	}
	if generated == nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, fmt.Errorf("decode generated global config: expected a JSON object")
	}
	if err := requireEOF(decoder); err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, err
	}

	config, err := mapConfig(*generated)
	if err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, err
	}
	config, err = config.Normalize()
	if err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, err
	}
	paths, err := collectUnknownJSONPaths(data)
	if err != nil {
		return operatorsettings.Config{}, operatorsettings.ConfigDecodeDiagnostics{}, fmt.Errorf("decode generated global config: %w", err)
	}
	return config, operatorsettings.ConfigDecodeDiagnostics{IgnoredJSONPaths: paths}, nil
}

func requireEOF(decoder *json.Decoder) error {
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("decode generated global config: %w", err)
	}
	return fmt.Errorf("decode generated global config: unexpected trailing JSON")
}

var globalConfigRawMessageType = reflect.TypeOf(json.RawMessage{})

type jsonField struct {
	name string
	typ  reflect.Type
}

// canonicalizeKnownJSONFieldNames restores encoding/json's case-insensitive
// struct-field matching before handing the value to generated models whose
// AdditionalProperties codecs use exact map keys. This keeps legacy customer
// spellings such as backendScopeId equivalent to backendScopeID while leaving
// unknown keys untouched for diagnostics and generated-model tolerance.
func canonicalizeKnownJSONFieldNames(raw json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	value = canonicalizeKnownJSONFieldNamesForType(value, reflect.TypeOf(factoryapi.GlobalConfig{}))
	return json.Marshal(value)
}

func canonicalizeKnownJSONFieldNamesForType(value any, typ reflect.Type) any {
	if value == nil || typ == nil {
		return value
	}
	typ = dereferenceJSONType(typ)
	if typ == globalConfigRawMessageType {
		return value
	}

	switch typ.Kind() {
	case reflect.Map:
		return canonicalizeKnownJSONFieldNamesForMap(value, typ)
	case reflect.Slice, reflect.Array:
		return canonicalizeKnownJSONFieldNamesForSequence(value, typ)
	case reflect.Struct:
		return canonicalizeKnownJSONFieldNamesForStruct(value, typ)
	default:
		return value
	}
}

func canonicalizeKnownJSONFieldNamesForMap(value any, typ reflect.Type) any {
	object, ok := value.(map[string]any)
	if !ok || typ.Key().Kind() != reflect.String {
		return value
	}
	normalized := make(map[string]any, len(object))
	for key, child := range object {
		normalized[key] = canonicalizeKnownJSONFieldNamesForType(child, typ.Elem())
	}
	return normalized
}

func canonicalizeKnownJSONFieldNamesForSequence(value any, typ reflect.Type) any {
	values, ok := value.([]any)
	if !ok {
		return value
	}
	normalized := make([]any, len(values))
	for index, item := range values {
		normalized[index] = canonicalizeKnownJSONFieldNamesForType(item, typ.Elem())
	}
	return normalized
}

func canonicalizeKnownJSONFieldNamesForStruct(value any, typ reflect.Type) any {
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	fields := jsonFields(typ)
	normalized := make(map[string]any, len(object))
	for key, child := range object {
		field, known := fields[strings.ToLower(key)]
		if !known {
			normalized[key] = child
			continue
		}
		normalized[field.name] = canonicalizeKnownJSONFieldNamesForType(child, field.typ)
	}
	return normalized
}

func collectUnknownJSONPaths(data []byte) ([]string, error) {
	value, err := decodeOneJSONValue(data)
	if err != nil {
		return nil, err
	}
	var paths []string
	collectUnknownJSONPathsForType(value, reflect.TypeOf(factoryapi.GlobalConfig{}), "$", &paths)
	return sortedUniqueJSONPaths(paths), nil
}

func decodeOneJSONValue(data []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected trailing JSON value")
		}
		return nil, err
	}
	return value, nil
}

func collectUnknownJSONPathsForType(value any, typ reflect.Type, path string, paths *[]string) {
	if value == nil || typ == nil {
		return
	}
	typ = dereferenceJSONType(typ)
	if typ == globalConfigRawMessageType {
		return
	}

	switch typ.Kind() {
	case reflect.Map:
		collectUnknownJSONPathsForMap(value, typ, path, paths)
	case reflect.Slice, reflect.Array:
		collectUnknownJSONPathsForSequence(value, typ, path, paths)
	case reflect.Struct:
		collectUnknownJSONPathsForStruct(value, typ, path, paths)
	}
}

func dereferenceJSONType(typ reflect.Type) reflect.Type {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	return typ
}

func collectUnknownJSONPathsForMap(value any, typ reflect.Type, path string, paths *[]string) {
	object, ok := value.(map[string]any)
	if !ok || typ.Key().Kind() != reflect.String {
		return
	}
	for key, child := range object {
		collectUnknownJSONPathsForType(child, typ.Elem(), appendJSONPath(path, key), paths)
	}
}

func collectUnknownJSONPathsForSequence(value any, typ reflect.Type, path string, paths *[]string) {
	values, ok := value.([]any)
	if !ok {
		return
	}
	for index, item := range values {
		collectUnknownJSONPathsForType(item, typ.Elem(), path+"["+strconv.Itoa(index)+"]", paths)
	}
}

func collectUnknownJSONPathsForStruct(value any, typ reflect.Type, path string, paths *[]string) {
	object, ok := value.(map[string]any)
	if !ok {
		return
	}
	fields := jsonFieldTypes(typ)
	for key, child := range object {
		fieldPath := appendJSONPath(path, key)
		fieldType, known := fields[strings.ToLower(key)]
		if !known {
			*paths = append(*paths, fieldPath)
			continue
		}
		collectUnknownJSONPathsForType(child, fieldType, fieldPath, paths)
	}
}

func jsonFieldTypes(typ reflect.Type) map[string]reflect.Type {
	fields := jsonFields(typ)
	types := make(map[string]reflect.Type, len(fields))
	for key, field := range fields {
		types[key] = field.typ
	}
	return types
}

func jsonFields(typ reflect.Type) map[string]jsonField {
	fields := make(map[string]jsonField)
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name == "" {
			name = field.Name
		}
		fields[strings.ToLower(name)] = jsonField{name: name, typ: field.Type}
	}
	return fields
}

func appendJSONPath(path, key string) string {
	if isSimpleJSONPathKey(key) {
		return path + "." + key
	}
	return path + "[" + strconv.Quote(key) + "]"
}

func isSimpleJSONPathKey(key string) bool {
	if key == "" {
		return false
	}
	for index, character := range key {
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9' && index > 0) ||
			character == '_' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func sortedUniqueJSONPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	unique := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path != "" {
			unique[path] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for path := range unique {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
func mapConfig(generated factoryapi.GlobalConfig) (operatorsettings.Config, error) {
	config := operatorsettings.Config{
		BackendScopeID: optionalString(generated.BackendScopeID),
		PriceTable:     operatorsettings.PriceTable{Currency: operatorsettings.PriceTableCurrencyUSD, Models: []operatorsettings.PriceTableModel{}},
		Runtime:        defaultRuntimeSettings(),
	}
	if generated.Defaults != nil {
		config.Defaults = operatorsettings.Defaults{
			WorkerModelProvider: optionalString(generated.Defaults.WorkerModelProvider),
			WorkerModel:         optionalString(generated.Defaults.WorkerModel),
		}
	}
	if generated.Models != nil {
		config.Models = mapModels(*generated.Models)
	}
	if generated.Runtime != nil {
		var err error
		config.Runtime.Logging, err = mapRuntimeArtifactSettings(
			"runtime.logging",
			generated.Runtime.Logging,
			config.Runtime.Logging,
		)
		if err != nil {
			return operatorsettings.Config{}, err
		}
		config.Runtime.Metrics, err = mapRuntimeArtifactSettings(
			"runtime.metrics",
			generated.Runtime.Metrics,
			config.Runtime.Metrics,
		)
		if err != nil {
			return operatorsettings.Config{}, err
		}
	}
	if generated.PriceTable != nil {
		priceTable, err := mapPriceTable(*generated.PriceTable)
		if err != nil {
			return operatorsettings.Config{}, err
		}
		config.PriceTable = priceTable
	}
	if generated.Workers != nil && generated.Workers.Acp != nil && generated.Workers.Acp.Integrations != nil {
		config.Workers.ACP.Integrations = make([]operatorsettings.ACPIntegration, len(*generated.Workers.Acp.Integrations))
		for index, integration := range *generated.Workers.Acp.Integrations {
			config.Workers.ACP.Integrations[index] = operatorsettings.ACPIntegration{
				ID: integration.Id, Name: integration.Name,
				Transport: string(integration.Transport), Command: integration.Command,
			}
		}
	}
	if generated.Workers != nil && generated.Workers.Acp != nil && generated.Workers.Acp.AgentProfile != nil {
		profile := operatorsettings.ACPAgentProfile{
			DefaultTarget: generated.Workers.Acp.AgentProfile.DefaultTarget,
		}
		// An omitted allowedTargets means unrestricted, so it stays nil here.
		// An authored-but-empty array must be rejected rather than folded into
		// that same nil: silently widening "allow nothing" into "allow every
		// installed Factory" is the opposite of what the operator wrote. This
		// boundary is the only place the two shapes are still
		// distinguishable, because appending an empty slice onto a nil slice
		// yields nil.
		if allowed := generated.Workers.Acp.AgentProfile.AllowedTargets; allowed != nil {
			if len(*allowed) == 0 {
				return operatorsettings.Config{}, fmt.Errorf(
					"workers.acp.agentProfile.allowedTargets must not be empty; omit it to leave the profile unrestricted",
				)
			}
			profile.AllowedTargets = append([]string(nil), *allowed...)
		}
		config.Workers.ACP.AgentProfile = &profile
	}
	if generated.WorkerPresets == nil {
		return config, nil
	}

	config.WorkerPresets = make([]operatorsettings.WorkerPreset, len(*generated.WorkerPresets))
	for i, preset := range *generated.WorkerPresets {
		config.WorkerPresets[i] = operatorsettings.WorkerPreset{
			ID:              preset.Id,
			ModelProvider:   string(preset.ModelProvider),
			Model:           optionalString(preset.Model),
			ReasoningEffort: optionalEnum(preset.ReasoningEffort),
		}
	}
	return config, nil
}

func mapPriceTable(generated factoryapi.GlobalConfigPriceTable) (operatorsettings.PriceTable, error) {
	if string(generated.Currency) == "" {
		return operatorsettings.PriceTable{}, fmt.Errorf("priceTable.currency is required and must be USD")
	}
	if generated.Models == nil {
		return operatorsettings.PriceTable{}, fmt.Errorf(
			"priceTable.models is required; use an empty array when no prices are configured",
		)
	}
	models := make([]operatorsettings.PriceTableModel, len(generated.Models))
	for index, model := range generated.Models {
		models[index] = operatorsettings.PriceTableModel{
			Provider:                        model.Provider,
			Model:                           model.Model,
			InputPerMillionTokens:           model.InputPerMillionTokens,
			OutputPerMillionTokens:          model.OutputPerMillionTokens,
			CachedInputPerMillionTokens:     model.CachedInputPerMillionTokens,
			ReasoningOutputPerMillionTokens: model.ReasoningOutputPerMillionTokens,
		}
	}
	table, err := (operatorsettings.PriceTable{Currency: string(generated.Currency), Models: models}).Normalize()
	if err != nil {
		return operatorsettings.PriceTable{}, err
	}
	return table, nil
}

func defaultRuntimeSettings() operatorsettings.RuntimeSettings {
	defaults := operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
		MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
	}
	return operatorsettings.RuntimeSettings{Logging: defaults, Metrics: defaults}
}

func mapRuntimeArtifactSettings(
	fieldPath string,
	generated *factoryapi.GlobalConfigRuntimeArtifactSettings,
	defaults operatorsettings.RuntimeArtifactSettings,
) (operatorsettings.RuntimeArtifactSettings, error) {
	if generated == nil {
		return defaults, nil
	}
	settings := defaults
	if generated.Directory != nil {
		settings.Directory = strings.TrimSpace(*generated.Directory)
		if settings.Directory == "" {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.directory must be non-empty", fieldPath)
		}
	}
	if generated.MaxSizeMB != nil {
		if *generated.MaxSizeMB < 1 {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.maxSizeMB must be at least 1", fieldPath)
		}
		settings.MaxSizeMB = *generated.MaxSizeMB
	}
	if generated.MaxBackups != nil {
		if *generated.MaxBackups < 1 {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.maxBackups must be at least 1", fieldPath)
		}
		settings.MaxBackups = *generated.MaxBackups
	}
	if generated.MaxAgeDays != nil {
		if *generated.MaxAgeDays < 1 {
			return operatorsettings.RuntimeArtifactSettings{}, fmt.Errorf("%s.maxAgeDays must be at least 1", fieldPath)
		}
		settings.MaxAgeDays = *generated.MaxAgeDays
	}
	if generated.Compress != nil {
		settings.Compress = *generated.Compress
	}
	return settings, nil
}

// Encode maps normalized Operator Settings values into the generated
// GlobalConfig model and serializes one canonical filesystem document.
func Encode(config operatorsettings.Config) ([]byte, error) {
	normalized, err := config.Normalize()
	if err != nil {
		return nil, fmt.Errorf("encode generated global config: %w", err)
	}
	generated := encodeConfig(normalized)

	payload, err := json.MarshalIndent(generated, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode generated global config: %w", err)
	}
	return append(payload, '\n'), nil
}

func encodeConfig(config operatorsettings.Config) factoryapi.GlobalConfig {
	generated := factoryapi.GlobalConfig{
		PriceTable: encodePriceTable(config.PriceTable),
		Runtime: &factoryapi.GlobalConfigRuntime{
			Logging: mapRuntimeArtifactSettingsToAPI(config.Runtime.Logging),
			Metrics: mapRuntimeArtifactSettingsToAPI(config.Runtime.Metrics),
		},
	}
	generated.BackendScopeID = optionalStringPointer(strings.TrimSpace(config.BackendScopeID))
	generated.Defaults = encodeDefaults(config.Defaults)
	generated.Models = encodeModels(config.Models)
	generated.Workers = encodeWorkers(config.Workers)
	generated.WorkerPresets = encodeWorkerPresets(config.WorkerPresets)
	return generated
}

func encodePriceTable(table operatorsettings.PriceTable) *factoryapi.GlobalConfigPriceTable {
	models := make([]factoryapi.GlobalConfigPriceTableModel, len(table.Models))
	for index, model := range table.Models {
		models[index] = factoryapi.GlobalConfigPriceTableModel{
			Provider:                        model.Provider,
			Model:                           model.Model,
			InputPerMillionTokens:           model.InputPerMillionTokens,
			OutputPerMillionTokens:          model.OutputPerMillionTokens,
			CachedInputPerMillionTokens:     model.CachedInputPerMillionTokens,
			ReasoningOutputPerMillionTokens: model.ReasoningOutputPerMillionTokens,
		}
	}
	return &factoryapi.GlobalConfigPriceTable{
		Currency: factoryapi.GlobalConfigPriceTableCurrency(table.Currency),
		Models:   models,
	}
}

func encodeDefaults(defaults operatorsettings.Defaults) *factoryapi.GlobalConfigDefaults {
	if defaults == (operatorsettings.Defaults{}) {
		return nil
	}
	return &factoryapi.GlobalConfigDefaults{
		WorkerModelProvider: optionalStringPointer(defaults.WorkerModelProvider),
		WorkerModel:         optionalStringPointer(defaults.WorkerModel),
	}
}

func encodeModels(models map[string]operatorsettings.ModelConfig) *factoryapi.GlobalConfigModels {
	if models == nil {
		return nil
	}
	generated := modelsToGenerated(models)
	return &generated
}

func encodeWorkers(settings operatorsettings.WorkerSettings) *factoryapi.GlobalConfigWorkers {
	if settings.ACP.Integrations == nil && settings.ACP.AgentProfile == nil {
		return nil
	}
	acp := &factoryapi.GlobalConfigACPSettings{}
	if settings.ACP.Integrations != nil {
		integrations := make([]factoryapi.GlobalConfigACPIntegration, len(settings.ACP.Integrations))
		for index, integration := range settings.ACP.Integrations {
			integrations[index] = factoryapi.GlobalConfigACPIntegration{
				Id: integration.ID, Name: integration.Name, Command: integration.Command,
				Transport: factoryapi.GlobalConfigACPIntegrationTransport(integration.Transport),
			}
		}
		acp.Integrations = &integrations
	}
	if settings.ACP.AgentProfile != nil {
		acp.AgentProfile = encodeAgentProfile(*settings.ACP.AgentProfile)
	}
	return &factoryapi.GlobalConfigWorkers{Acp: acp}
}

func encodeAgentProfile(profile operatorsettings.ACPAgentProfile) *factoryapi.GlobalConfigACPAgentProfile {
	encoded := &factoryapi.GlobalConfigACPAgentProfile{DefaultTarget: profile.DefaultTarget}
	if !profile.IsUnrestricted() {
		allowed := append([]string(nil), profile.AllowedTargets...)
		encoded.AllowedTargets = &allowed
	}
	return encoded
}

func encodeWorkerPresets(values []operatorsettings.WorkerPreset) *[]factoryapi.GlobalConfigWorkerPreset {
	if values == nil {
		return nil
	}
	presets := make([]factoryapi.GlobalConfigWorkerPreset, len(values))
	for index, preset := range values {
		presets[index] = factoryapi.GlobalConfigWorkerPreset{
			Id:            preset.ID,
			ModelProvider: factoryapi.GlobalConfigWorkerPresetModelProvider(preset.ModelProvider),
			Model:         optionalStringPointer(preset.Model),
		}
		if preset.ReasoningEffort != "" {
			effort := factoryapi.GlobalConfigWorkerPresetReasoningEffort(preset.ReasoningEffort)
			presets[index].ReasoningEffort = &effort
		}
	}
	return &presets
}

func mapModels(generated factoryapi.GlobalConfigModels) map[string]operatorsettings.ModelConfig {
	models := make(map[string]operatorsettings.ModelConfig, len(generated))
	for name, model := range generated {
		entry := operatorsettings.ModelConfig{}
		if model.Source != nil {
			source := *model.Source
			entry.Source = &source
		}
		if model.Backend != nil {
			backend := *model.Backend
			entry.Backend = &backend
		}
		if model.LoadPolicy != nil {
			policy := operatorsettings.ModelLoadPolicy(*model.LoadPolicy)
			entry.LoadPolicy = &policy
		}
		if model.Operations != nil {
			operations := make([]string, len(*model.Operations))
			for index, operation := range *model.Operations {
				operations[index] = string(operation)
			}
			entry.Operations = operations
		}
		models[name] = entry
	}
	return models
}

func modelsToGenerated(values map[string]operatorsettings.ModelConfig) factoryapi.GlobalConfigModels {
	models := make(factoryapi.GlobalConfigModels, len(values))
	for name, config := range values {
		entry := factoryapi.GlobalConfigModel{}
		if config.Source != nil {
			source := *config.Source
			entry.Source = &source
		}
		if config.Backend != nil {
			backend := *config.Backend
			entry.Backend = &backend
		}
		if config.LoadPolicy != nil {
			policy := factoryapi.GlobalConfigModelLoadPolicy(*config.LoadPolicy)
			entry.LoadPolicy = &policy
		}
		if config.Operations != nil {
			operations := make([]factoryapi.GlobalConfigModelOperation, len(config.Operations))
			for index, operation := range config.Operations {
				operations[index] = factoryapi.GlobalConfigModelOperation(operation)
			}
			entry.Operations = &operations
		}
		models[name] = entry
	}
	return models
}

func mapRuntimeArtifactSettingsToAPI(
	settings operatorsettings.RuntimeArtifactSettings,
) *factoryapi.GlobalConfigRuntimeArtifactSettings {
	return &factoryapi.GlobalConfigRuntimeArtifactSettings{
		Directory:  optionalStringPointer(settings.Directory),
		MaxSizeMB:  &settings.MaxSizeMB,
		MaxBackups: &settings.MaxBackups,
		MaxAgeDays: &settings.MaxAgeDays,
		Compress:   &settings.Compress,
	}
}

func optionalStringPointer(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func optionalEnum(value *factoryapi.GlobalConfigWorkerPresetReasoningEffort) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
