// Factory envelope, metadata, and invocation mapping between the internal definition and generated API models.
package factoryconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorydefinition/retiredboundary"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
	generated   factoryapi.Factory
	diagnostics FactoryDecodeDiagnostics
}

const generatedFactoryBoundaryErrorPrefix = "decode factory generated-schema boundary"

// Expand parses and normalizes a user-provided factory payload into the internal
// canonical configuration representation.
func (m *FactoryConfigMapper) Expand(data []byte) (*interfaces.FactoryConfig, error) {
	cfg, _, err := m.ExpandWithDiagnostics(data)
	return cfg, err
}

// ExpandStrict parses a Factory payload for repository-internal catalog and
// packaging tooling. Customer-facing loading must use Expand or
// ExpandWithDiagnostics so additive fields remain forward compatible.
func (m *FactoryConfigMapper) ExpandStrict(data []byte) (*interfaces.FactoryConfig, error) {
	boundary, err := decodeGeneratedFactoryBoundaryJSONWithPolicy(data, true)
	if err != nil {
		return nil, err
	}

	cfg, err := FactoryConfigFromOpenAPI(boundary.generated)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ExpandWithDiagnostics parses and normalizes a user-provided factory payload,
// retaining safe paths for fields that were ignored as forward-compatible
// additions.
func (m *FactoryConfigMapper) ExpandWithDiagnostics(
	data []byte,
) (*interfaces.FactoryConfig, FactoryDecodeDiagnostics, error) {
	boundary, err := decodeGeneratedFactoryBoundaryJSON(data)
	if err != nil {
		return nil, FactoryDecodeDiagnostics{}, err
	}

	cfg, err := FactoryConfigFromOpenAPI(boundary.generated)
	if err != nil {
		return nil, FactoryDecodeDiagnostics{}, err
	}
	cfg.SetIgnoredJSONPaths(boundary.diagnostics.Paths())
	diagnostics := FactoryDecodeDiagnostics{IgnoredJSONPaths: boundary.diagnostics.Paths()}
	return &cfg, diagnostics, nil
}

func decodeGeneratedFactoryBoundaryJSON(data []byte) (generatedFactoryBoundary, error) {
	return decodeGeneratedFactoryBoundaryJSONWithPolicy(data, false)
}

func decodeGeneratedFactoryBoundaryJSONWithPolicy(
	data []byte,
	rejectUnknownFields bool,
) (generatedFactoryBoundary, error) {
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
	ignoredJSONPaths, err := collectUnknownFactoryJSONPaths(normalizedData)
	if err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	if rejectUnknownFields && len(ignoredJSONPaths) > 0 {
		return generatedFactoryBoundary{}, fmt.Errorf(
			"%s: json: unknown field %q",
			generatedFactoryBoundaryErrorPrefix,
			jsonFieldNameFromPath(ignoredJSONPaths[0]),
		)
	}

	apiCfg, err := decodeGeneratedFactoryBoundaryWithPolicy(normalizedData, rejectUnknownFields)
	if err != nil {
		return generatedFactoryBoundary{}, fmt.Errorf("%s: %w", generatedFactoryBoundaryErrorPrefix, err)
	}
	return generatedFactoryBoundary{
		generated: apiCfg,
		diagnostics: FactoryDecodeDiagnostics{
			IgnoredJSONPaths: ignoredJSONPaths,
		},
	}, nil
}

func jsonFieldNameFromPath(path string) string {
	if index := strings.LastIndex(path, "."); index >= 0 && index+1 < len(path) {
		return path[index+1:]
	}
	return path
}

func decodeGeneratedFactoryBoundaryWithPolicy(data []byte, rejectUnknownFields bool) (factoryapi.Factory, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if rejectUnknownFields {
		decoder.DisallowUnknownFields()
	}

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

func decodeAuthoredFactoryBoundary(data []byte) (factoryapi.Factory, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	var apiCfg factoryapi.Factory
	if err := decoder.Decode(&apiCfg); err != nil {
		return factoryapi.Factory{}, fmt.Errorf("unmarshal factory api model: %w", err)
	}
	if err := ensureFactoryBoundaryEOF(decoder); err != nil {
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
	if apiCfg.Examples != nil {
		for index, example := range *apiCfg.Examples {
			if strings.TrimSpace(example.Name) == "" {
				return fmt.Errorf("factory.examples[%d].name must be a non-empty string", index)
			}
			if example.Args == nil {
				return fmt.Errorf("factory.examples[%d].args is required", index)
			}
		}
	}
	return nil
}

// Flatten serializes an internal factory configuration into canonical JSON that is
// stable for persisted output and downstream tooling.
func (m *FactoryConfigMapper) Flatten(cfg *interfaces.FactoryConfig) ([]byte, error) {
	apiCfg, err := factoryAPIFromInternalConfig(cfg)
	if err != nil {
		return nil, err
	}
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

func factoryAPIFromInternalConfig(cfg *interfaces.FactoryConfig) (factoryapi.Factory, error) {
	if cfg == nil {
		return factoryapi.Factory{}, nil
	}
	if err := validateInternalFactoryMetadata(cfg); err != nil {
		return factoryapi.Factory{}, err
	}
	examples, err := invocationExamplesAPIFromInternal(cfg.Examples)
	if err != nil {
		return factoryapi.Factory{}, err
	}

	return factoryapi.Factory{
		Name:                factoryReferenceName(cfg),
		Runner:              runnerIDPtrIfNotEmpty(cfg.Runner),
		Description:         NameValueAPIFromInternal(cfg.Description),
		Id:                  stringPtrIfNotEmpty(cfg.Project),
		Version:             hybridLogicalTimestampPtr(cfg.Version),
		Guards:              factoryGuardsAPIFromInternal(cfg.Guards),
		InputTypes:          inputTypesAPIFromInternal(cfg.InputTypes),
		InvocationReturn:    invocationReturnAPIFromInternal(cfg.InvocationReturn),
		InvocationSignature: invocationSignatureAPIFromInternal(cfg.InvocationSignature),
		Examples:            examples,
		Webhooks:            factoryWebhooksAPIFromInternal(cfg.Webhooks),
		Orchestrator:        orchestratorAPIFromInternal(cfg),
		WorkTypes:           workTypesAPIFromInternal(cfg.WorkTypes),
		Resources:           resourcesAPIFromInternal(cfg.Resources),
		SupportingFiles:     resourceManifestAPIFromInternal(cfg.ResourceManifest),
		Layout:              factoryLayoutAPIFromInternal(cfg.Layout),
		Workers:             workersAPIFromInternal(cfg.Workers, cfg.Workstations),
		Workstations:        workstationsAPIFromInternal(cfg.Workstations, workerTypesByName(cfg.Workers)),
	}, nil
}

func factoryWebhooksAPIFromInternal(values []interfaces.FactoryWebhookConfig) *[]factoryapi.FactoryWebhook {
	if len(values) == 0 {
		return nil
	}
	result := make([]factoryapi.FactoryWebhook, len(values))
	for index, value := range values {
		filter := factoryapi.FactoryWebhookFilter{
			EventTypes: make([]factoryapi.FactoryWebhookEventType, len(value.Filter.EventTypes)),
		}
		for eventIndex, eventType := range value.Filter.EventTypes {
			filter.EventTypes[eventIndex] = factoryapi.FactoryWebhookEventType(eventType)
		}
		if len(value.Filter.DispatchStatuses) > 0 {
			statuses := make([]factoryapi.FactoryWebhookDispatchStatus, len(value.Filter.DispatchStatuses))
			for statusIndex, status := range value.Filter.DispatchStatuses {
				statuses[statusIndex] = factoryapi.FactoryWebhookDispatchStatus(status)
			}
			filter.DispatchStatuses = &statuses
		}
		result[index] = factoryapi.FactoryWebhook{
			Name:             value.Name,
			Enabled:          value.Enabled,
			Url:              value.URL,
			SigningSecretRef: value.SigningSecretRef,
			Filter:           filter,
			DeliveryPolicy:   factoryWebhookDeliveryPolicyAPIFromInternal(value.DeliveryPolicy),
		}
	}
	return &result
}

func factoryWebhookDeliveryPolicyAPIFromInternal(value *interfaces.FactoryWebhookDeliveryPolicyConfig) *factoryapi.FactoryWebhookDeliveryPolicy {
	if value == nil {
		return nil
	}
	result := &factoryapi.FactoryWebhookDeliveryPolicy{
		RequestTimeout: value.RequestTimeout,
		InitialBackoff: value.InitialBackoff,
		MaxBackoff:     value.MaxBackoff,
	}
	if value.MaxAttempts != nil {
		result.MaxAttempts = value.MaxAttempts
	}
	if value.BackoffMultiplier != nil {
		multiplier := float32(*value.BackoffMultiplier)
		result.BackoffMultiplier = &multiplier
	}
	return result
}

func validateInternalFactoryMetadata(cfg *interfaces.FactoryConfig) error {
	if err := validateInternalNameValue(cfg.Description, "factory.description"); err != nil {
		return err
	}
	for index := range cfg.WorkTypes {
		if err := validateInternalNameValue(cfg.WorkTypes[index].Description, fmt.Sprintf("factory.workTypes[%d].description", index)); err != nil {
			return err
		}
	}
	for index := range cfg.Workers {
		if err := validateInternalNameValue(cfg.Workers[index].Description, fmt.Sprintf("factory.workers[%d].description", index)); err != nil {
			return err
		}
	}
	for index := range cfg.Workstations {
		if err := validateInternalNameValue(cfg.Workstations[index].Description, fmt.Sprintf("factory.workstations[%d].description", index)); err != nil {
			return err
		}
	}
	for index := range cfg.Examples {
		if strings.TrimSpace(cfg.Examples[index].Name) == "" {
			return fmt.Errorf("factory.examples[%d].name must be a non-empty string", index)
		}
		if err := validateInternalNameValue(&cfg.Examples[index].Description, fmt.Sprintf("factory.examples[%d].description", index)); err != nil {
			return err
		}
	}
	return nil
}

func validateInternalNameValue(value *interfaces.NameValueConfig, path string) error {
	if value == nil {
		return nil
	}
	if err := value.Validate(); err != nil {
		return fmt.Errorf("%s.%w", path, err)
	}
	return nil
}

// NameValueAPIFromInternal maps a validated internal localized value into the
// generated public contract without sharing mutable collection storage.
func NameValueAPIFromInternal(value *interfaces.NameValueConfig) *factoryapi.NameValue {
	if value == nil {
		return nil
	}
	result := &factoryapi.NameValue{
		Type:  factoryapi.NameValueType(value.Type),
		Value: value.Value,
		Id:    stringPtrIfNotEmpty(value.ID),
	}
	if len(value.Locales) > 0 {
		locales := append([]string(nil), value.Locales...)
		result.Locales = &locales
	}
	if len(value.Values) > 0 {
		values := make(map[string]string, len(value.Values))
		for locale, localized := range value.Values {
			values[locale] = localized
		}
		result.Values = &values
	}
	return result
}

func invocationSignatureAPIFromInternal(value *interfaces.InvocationSignatureConfig) *factoryapi.FactoryInvocationSignature {
	if value == nil {
		return nil
	}
	return &factoryapi.FactoryInvocationSignature{
		Parameters:                 invocationParametersAPIFromInternal(value.Parameters),
		UnknownNamedArgumentPolicy: invocationUnknownNamedArgumentPolicyPtr(value.UnknownNamedArgumentPolicy),
		OutputContract:             invocationOutputContractAPIFromInternal(value.OutputContract),
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

func invocationExamplesAPIFromInternal(examples []interfaces.InvocationExampleConfig) (*[]factoryapi.FactoryInvocationExample, error) {
	if len(examples) == 0 {
		return nil, nil
	}
	values := make([]factoryapi.FactoryInvocationExample, len(examples))
	for i, example := range examples {
		args, err := invocationExampleArgsAPIFromInternal(example.Args, fmt.Sprintf("factory.examples[%d].args", i))
		if err != nil {
			return nil, err
		}
		values[i] = factoryapi.FactoryInvocationExample{
			Name:        example.Name,
			Description: *NameValueAPIFromInternal(&example.Description),
			Args:        args,
		}
	}
	return &values, nil
}

func invocationExampleArgsAPIFromInternal(args map[string]interface{}, path string) (factoryapi.FactoryInvocationArguments, error) {
	values := make(factoryapi.FactoryInvocationArguments, len(args))
	for name, value := range args {
		var union factoryapi.FactoryInvocationArguments_AdditionalProperties
		switch typed := value.(type) {
		case string:
			if err := union.FromFactoryInvocationArguments0(typed); err != nil {
				return nil, fmt.Errorf("%s.%s: %w", path, name, err)
			}
		case []string:
			if err := union.FromFactoryInvocationArguments1(append([]string(nil), typed...)); err != nil {
				return nil, fmt.Errorf("%s.%s: %w", path, name, err)
			}
		default:
			return nil, fmt.Errorf("%s.%s must be a string or array of strings", path, name)
		}
		values[name] = union
	}
	return values, nil
}

func factoryInternalFromAPI(apiCfg factoryapi.Factory) (interfaces.FactoryConfig, error) {
	cfg := interfaces.FactoryConfig{Name: string(apiCfg.Name)}
	cfg.Runner = enumStringValue(apiCfg.Runner)
	if err := mapFactoryMetadataInternalFromAPI(&cfg, apiCfg.Description, apiCfg.Examples); err != nil {
		return interfaces.FactoryConfig{}, err
	}
	if apiCfg.Id != nil {
		cfg.Project = *apiCfg.Id
	}
	if apiCfg.Version != nil {
		cfg.Version = &interfaces.FactoryVersion{
			Logical:  apiCfg.Version.Logical.Int64(),
			Physical: apiCfg.Version.Physical.UTC(),
		}
	}
	cfg.Guards = factoryGuardsInternalFromAPI(apiCfg.Guards)
	if apiCfg.InputTypes != nil {
		cfg.InputTypes = inputTypesInternalFromAPI(*apiCfg.InputTypes)
	}
	cfg.InvocationReturn = invocationReturnInternalFromAPI(apiCfg.InvocationReturn)
	cfg.InvocationSignature = invocationSignatureInternalFromAPI(apiCfg.InvocationSignature)
	cfg.Webhooks = factoryWebhooksInternalFromAPI(apiCfg.Webhooks)
	orchestrator, err := orchestratorInternalFromAPI(apiCfg.Orchestrator)
	if err != nil {
		return interfaces.FactoryConfig{}, err
	}
	cfg.Orchestrator = orchestrator
	if apiCfg.WorkTypes != nil {
		workTypes, err := workTypesInternalFromAPI(*apiCfg.WorkTypes)
		if err != nil {
			return interfaces.FactoryConfig{}, err
		}
		cfg.WorkTypes = workTypes
	}
	if apiCfg.Resources != nil {
		cfg.Resources = resourcesInternalFromAPI(*apiCfg.Resources)
	}
	if apiCfg.SupportingFiles != nil {
		cfg.ResourceManifest = resourceManifestInternalFromAPI(apiCfg.SupportingFiles)
	}
	if apiCfg.Layout != nil {
		cfg.Layout = factoryLayoutInternalFromAPI(apiCfg.Layout)
	}
	if apiCfg.Workers != nil {
		workers, err := workersInternalFromAPI(*apiCfg.Workers)
		if err != nil {
			return interfaces.FactoryConfig{}, err
		}
		cfg.Workers = workers
	}
	if apiCfg.Workstations != nil {
		workstations, err := workstationsInternalFromAPI(*apiCfg.Workstations)
		if err != nil {
			return interfaces.FactoryConfig{}, err
		}
		cfg.Workstations = workstations
	}
	return cfg, nil
}

func factoryWebhooksInternalFromAPI(values *[]factoryapi.FactoryWebhook) []interfaces.FactoryWebhookConfig {
	if values == nil {
		return nil
	}
	result := make([]interfaces.FactoryWebhookConfig, len(*values))
	for index, value := range *values {
		webhook := interfaces.FactoryWebhookConfig{
			Name:             value.Name,
			Enabled:          value.Enabled,
			URL:              value.Url,
			SigningSecretRef: value.SigningSecretRef,
			Filter: interfaces.FactoryWebhookFilterConfig{
				EventTypes: make([]string, len(value.Filter.EventTypes)),
			},
			DeliveryPolicy: factoryWebhookDeliveryPolicyInternalFromAPI(value.DeliveryPolicy),
		}
		for eventIndex, eventType := range value.Filter.EventTypes {
			webhook.Filter.EventTypes[eventIndex] = string(eventType)
		}
		if value.Filter.DispatchStatuses != nil {
			webhook.Filter.DispatchStatuses = make([]string, len(*value.Filter.DispatchStatuses))
			for statusIndex, status := range *value.Filter.DispatchStatuses {
				webhook.Filter.DispatchStatuses[statusIndex] = string(status)
			}
		}
		result[index] = webhook
	}
	return result
}

func factoryWebhookDeliveryPolicyInternalFromAPI(value *factoryapi.FactoryWebhookDeliveryPolicy) *interfaces.FactoryWebhookDeliveryPolicyConfig {
	if value == nil {
		return nil
	}
	result := &interfaces.FactoryWebhookDeliveryPolicyConfig{
		RequestTimeout: value.RequestTimeout,
		InitialBackoff: value.InitialBackoff,
		MaxBackoff:     value.MaxBackoff,
	}
	if value.MaxAttempts != nil {
		attempts := *value.MaxAttempts
		result.MaxAttempts = &attempts
	}
	if value.BackoffMultiplier != nil {
		multiplier := float64(*value.BackoffMultiplier)
		result.BackoffMultiplier = &multiplier
	}
	return result
}

func mapFactoryMetadataInternalFromAPI(cfg *interfaces.FactoryConfig, description *factoryapi.NameValue, examples *[]factoryapi.FactoryInvocationExample) error {
	mappedDescription, err := nameValueInternalFromAPI(description, "factory.description")
	if err != nil {
		return err
	}
	mappedExamples, err := invocationExamplesInternalFromAPI(examples)
	if err != nil {
		return err
	}
	cfg.Description = mappedDescription
	cfg.Examples = mappedExamples
	return nil
}

// FactoryConfigFromOpenAPI converts the generated OpenAPI factory model into
// the internal config representation.
func FactoryConfigFromOpenAPI(apiCfg factoryapi.Factory) (interfaces.FactoryConfig, error) {
	return factoryInternalFromAPI(apiCfg)
}

// NameValueFromOpenAPI validates and maps one generated localized value into
// the transport-independent Factory definition contract.
func NameValueFromOpenAPI(value factoryapi.NameValue) (interfaces.NameValueConfig, error) {
	return nameValueFromOpenAPI(value, "")
}

func nameValueInternalFromAPI(value *factoryapi.NameValue, fieldPath string) (*interfaces.NameValueConfig, error) {
	if value == nil {
		return nil, nil
	}
	mapped, err := nameValueFromOpenAPI(*value, fieldPath)
	if err != nil {
		return nil, err
	}
	return &mapped, nil
}

func nameValueFromOpenAPI(value factoryapi.NameValue, fieldPath string) (interfaces.NameValueConfig, error) {
	result := interfaces.NameValueConfig{
		Type:  string(value.Type),
		Value: value.Value,
		ID:    stringValue(value.Id),
	}
	if value.Locales != nil {
		result.Locales = append([]string(nil), (*value.Locales)...)
	}
	if value.Values != nil {
		result.Values = make(map[string]string, len(*value.Values))
		for locale, localized := range *value.Values {
			result.Values[locale] = localized
		}
	}
	if err := result.Validate(); err != nil {
		if fieldPath != "" {
			return interfaces.NameValueConfig{}, fmt.Errorf("%s.%w", fieldPath, err)
		}
		return interfaces.NameValueConfig{}, err
	}
	return result, nil
}

func inputTypesInternalFromAPI(inputTypes []factoryapi.InputType) []interfaces.InputTypeConfig {
	values := make([]interfaces.InputTypeConfig, len(inputTypes))
	for i, inputType := range inputTypes {
		values[i] = interfaces.InputTypeConfig{
			Name: inputType.Name,
			Type: internalFactoryInputKindFromPublic(inputType.Type),
		}
	}
	return values
}

func invocationReturnInternalFromAPI(value *factoryapi.InvocationReturn) *interfaces.InvocationReturnConfig {
	if value == nil {
		return nil
	}
	return &interfaces.InvocationReturnConfig{
		Policy:        string(value.Policy),
		WorkTypeName:  stringValue(value.WorkTypeName),
		TerminalState: stringValue(value.TerminalState),
		WorkName:      stringValue(value.WorkName),
	}
}

func invocationSignatureInternalFromAPI(value *factoryapi.FactoryInvocationSignature) *interfaces.InvocationSignatureConfig {
	if value == nil {
		return nil
	}
	return &interfaces.InvocationSignatureConfig{
		Parameters:                 invocationParametersInternalFromAPI(value.Parameters),
		UnknownNamedArgumentPolicy: enumStringValue(value.UnknownNamedArgumentPolicy),
		OutputContract:             invocationOutputContractInternalFromAPI(value.OutputContract),
	}
}

func invocationParametersInternalFromAPI(parameters *[]factoryapi.FactoryInvocationParameter) []interfaces.InvocationParameterConfig {
	if parameters == nil {
		return nil
	}
	values := make([]interfaces.InvocationParameterConfig, len(*parameters))
	for i, parameter := range *parameters {
		values[i] = interfaces.InvocationParameterConfig{
			Name:          parameter.Name,
			Description:   stringValue(parameter.Description),
			ExternalName:  stringValue(parameter.ExternalName),
			Aliases:       stringSliceValue(parameter.Aliases),
			TypeHint:      enumStringValue(parameter.TypeHint),
			ValueMode:     enumStringValue(parameter.ValueMode),
			Required:      boolValue(parameter.Required),
			Sensitive:     boolValue(parameter.Sensitive),
			Choices:       stringSliceValue(parameter.Choices),
			DefaultValue:  stringValue(parameter.DefaultValue),
			DefaultValues: stringSliceValue(parameter.DefaultValues),
			Bindings:      invocationParameterBindingsInternalFromAPI(parameter.Bindings),
		}
	}
	return values
}

func invocationParameterBindingsInternalFromAPI(bindings *[]factoryapi.FactoryInvocationParameterBinding) []interfaces.InvocationParameterBindingConfig {
	if bindings == nil {
		return nil
	}
	values := make([]interfaces.InvocationParameterBindingConfig, len(*bindings))
	for i, binding := range *bindings {
		values[i] = interfaces.InvocationParameterBindingConfig{
			Kind:     string(binding.Kind),
			Position: intValue(binding.Position),
		}
	}
	return values
}

func invocationOutputContractInternalFromAPI(value *factoryapi.FactoryInvocationOutputContract) *interfaces.InvocationOutputContractConfig {
	if value == nil {
		return nil
	}
	return &interfaces.InvocationOutputContractConfig{
		Mode:          enumStringValue(value.Mode),
		PathParameter: stringValue(value.PathParameter),
		ContentType:   stringValue(value.ContentType),
		FileExtension: stringValue(value.FileExtension),
		Description:   stringValue(value.Description),
	}
}

func invocationExamplesInternalFromAPI(examples *[]factoryapi.FactoryInvocationExample) ([]interfaces.InvocationExampleConfig, error) {
	if examples == nil {
		return nil, nil
	}
	values := make([]interfaces.InvocationExampleConfig, len(*examples))
	for i, example := range *examples {
		description, err := nameValueFromOpenAPI(example.Description, fmt.Sprintf("factory.examples[%d].description", i))
		if err != nil {
			return nil, err
		}
		args, err := invocationExampleArgsInternalFromAPI(example.Args)
		if err != nil {
			return nil, fmt.Errorf("factory.examples[%d].args: %w", i, err)
		}
		values[i] = interfaces.InvocationExampleConfig{
			Name:        example.Name,
			Description: description,
			Args:        args,
		}
	}
	return values, nil
}

func invocationExampleArgsInternalFromAPI(args factoryapi.FactoryInvocationArguments) (interfaces.InvocationExampleArguments, error) {
	values := make(interfaces.InvocationExampleArguments, len(args))
	for name, value := range args {
		if scalar, err := value.AsFactoryInvocationArguments0(); err == nil {
			values[name] = scalar
			continue
		}
		list, err := value.AsFactoryInvocationArguments1()
		if err != nil {
			return nil, fmt.Errorf("%s must be a string or array of strings", name)
		}
		values[name] = append([]string(nil), list...)
	}
	return values, nil
}

func orchestratorInternalFromAPI(value *factoryapi.FactoryOrchestrator) (*interfaces.FactoryOrchestratorConfig, error) {
	if value == nil {
		return nil, nil
	}
	kind := interfaces.StrictPublicFactoryOrchestratorKind(string(value.Kind))
	if kind == "" {
		return &interfaces.FactoryOrchestratorConfig{Kind: string(value.Kind)}, nil
	}
	cfg := &interfaces.FactoryOrchestratorConfig{
		Kind: kind,
	}
	if value.Petri != nil {
		cfg.Petri = &interfaces.FactoryOrchestratorPetriConfig{}
	}
	if value.Javascript != nil {
		jsCfg, err := orchestratorJavaScriptInternalFromAPI(*value.Javascript)
		if err != nil {
			return nil, err
		}
		cfg.JavaScript = jsCfg
	}
	return cfg, nil
}

func orchestratorJavaScriptInternalFromAPI(value factoryapi.FactoryOrchestratorJavaScriptConfig) (*interfaces.FactoryOrchestratorJavaScriptConfig, error) {
	cfg := &interfaces.FactoryOrchestratorJavaScriptConfig{
		Dialect:    stringValue(value.Dialect),
		SourceRef:  stringValue(value.SourceRef),
		SourceHash: stringValue(value.SourceHash),
		Entrypoint: stringValue(value.Entrypoint),
	}
	if value.Metadata != nil {
		cfg.Metadata = map[string]string(*value.Metadata)
	}
	if value.InlineSource != nil {
		cfg.InlineSource = &interfaces.FactoryOrchestratorJavaScriptInlineSource{
			Encoding: string(value.InlineSource.Encoding),
			Inline:   value.InlineSource.Inline,
		}
	}
	if value.ArgsSchema != nil {
		raw, err := json.Marshal(value.ArgsSchema)
		if err != nil {
			return nil, err
		}
		cfg.ArgsSchema = raw
	}
	if value.DefaultPolicy != nil {
		raw, err := json.Marshal(value.DefaultPolicy)
		if err != nil {
			return nil, err
		}
		cfg.DefaultPolicy = raw
	}
	if value.Agents != nil {
		cfg.Agents = make(map[string]interfaces.FactoryOrchestratorJavaScriptAgent, len(*value.Agents))
		for id, agent := range *value.Agents {
			cfg.Agents[id] = interfaces.FactoryOrchestratorJavaScriptAgent{Preset: agent.Preset}
		}
	}
	return cfg, nil
}

func orchestratorAPIFromInternal(cfg *interfaces.FactoryConfig) *factoryapi.FactoryOrchestrator {
	if cfg == nil || cfg.Orchestrator == nil {
		return nil
	}
	kind := interfaces.EffectiveOrchestratorKind(cfg)
	apiKind := factoryapi.FactoryOrchestratorKind(interfaces.StrictPublicFactoryOrchestratorKind(kind))
	result := &factoryapi.FactoryOrchestrator{
		Kind: apiKind,
	}
	if cfg.Orchestrator.Petri != nil || kind == interfaces.OrchestratorKindPetri {
		result.Petri = &factoryapi.FactoryOrchestratorPetriConfig{}
	}
	if cfg.Orchestrator.JavaScript != nil {
		result.Javascript = orchestratorJavaScriptAPIFromInternal(cfg.Orchestrator.JavaScript)
	}
	return result
}

// ProjectEffectiveOrchestratorForAPIRead fills the compatibility PETRI orchestrator
// projection when a factory has no authored orchestrator block.
func ProjectEffectiveOrchestratorForAPIRead(api factoryapi.Factory, cfg *interfaces.FactoryConfig) factoryapi.Factory {
	if api.Orchestrator != nil {
		return api
	}
	if interfaces.EffectiveOrchestratorKind(cfg) == interfaces.OrchestratorKindPetri {
		api.Orchestrator = defaultPetriOrchestratorAPI()
	}
	return api
}

func defaultPetriOrchestratorAPI() *factoryapi.FactoryOrchestrator {
	kind := factoryapi.PETRI
	return &factoryapi.FactoryOrchestrator{
		Kind:  kind,
		Petri: &factoryapi.FactoryOrchestratorPetriConfig{},
	}
}

func orchestratorJavaScriptAPIFromInternal(cfg *interfaces.FactoryOrchestratorJavaScriptConfig) *factoryapi.FactoryOrchestratorJavaScriptConfig {
	if cfg == nil {
		return nil
	}
	result := &factoryapi.FactoryOrchestratorJavaScriptConfig{
		Dialect:    stringPtrIfNotEmpty(cfg.Dialect),
		SourceRef:  stringPtrIfNotEmpty(cfg.SourceRef),
		SourceHash: stringPtrIfNotEmpty(cfg.SourceHash),
		Entrypoint: stringPtrIfNotEmpty(cfg.Entrypoint),
	}
	if len(cfg.Metadata) > 0 {
		metadata := factoryapi.StringMap(cfg.Metadata)
		result.Metadata = &metadata
	}
	if cfg.InlineSource != nil {
		result.InlineSource = &factoryapi.FactoryOrchestratorJavaScriptInlineSource{
			Encoding: factoryapi.FactoryOrchestratorJavaScriptInlineSourceEncoding(cfg.InlineSource.Encoding),
			Inline:   cfg.InlineSource.Inline,
		}
	}
	if len(cfg.ArgsSchema) > 0 {
		var argsSchema map[string]any
		if err := json.Unmarshal(cfg.ArgsSchema, &argsSchema); err == nil {
			result.ArgsSchema = &argsSchema
		}
	}
	if len(cfg.DefaultPolicy) > 0 {
		var defaultPolicy map[string]any
		if err := json.Unmarshal(cfg.DefaultPolicy, &defaultPolicy); err == nil {
			result.DefaultPolicy = &defaultPolicy
		}
	}
	if len(cfg.Agents) > 0 {
		agents := make(map[string]factoryapi.FactoryOrchestratorJavaScriptAgent, len(cfg.Agents))
		for id, agent := range cfg.Agents {
			agents[id] = factoryapi.FactoryOrchestratorJavaScriptAgent{Preset: agent.Preset}
		}
		result.Agents = &agents
	}
	return result
}

func isDefaultPetriOrchestratorAPI(value *factoryapi.FactoryOrchestrator) bool {
	if value == nil {
		return true
	}
	if value.Kind != factoryapi.PETRI {
		return false
	}
	if value.Javascript != nil {
		return false
	}
	if value.Petri == nil {
		return true
	}
	return true
}
