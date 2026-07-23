package factoryconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// mapLegacyInvocationExamples is the single input-only compatibility path for
// the former invocationSignature.examples shape. Canonical output never calls
// this function and therefore only emits Factory.examples.
func mapLegacyInvocationExamples(value any) error {
	root, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	signatureObject, ok := root["invocationSignature"].(map[string]any)
	if !ok {
		return nil
	}
	legacy, hasLegacy := signatureObject["examples"]
	if !hasLegacy {
		return nil
	}
	if _, hasCanonical := root["examples"]; hasCanonical {
		return fmt.Errorf("factory examples conflict: examples and invocationSignature.examples cannot both be defined")
	}
	delete(signatureObject, "examples")

	signature, err := decodeCompatibilityInvocationSignature(signatureObject)
	if err != nil {
		return fmt.Errorf("factory.invocationSignature.examples compatibility mapping: %w", err)
	}
	entries, ok := legacy.([]any)
	if !ok {
		return fmt.Errorf("factory.invocationSignature.examples must be an array")
	}
	canonical := make([]any, len(entries))
	for index, entry := range entries {
		mapped, err := mapLegacyInvocationExample(entry, signature, index)
		if err != nil {
			return err
		}
		canonical[index] = mapped
	}
	root["examples"] = canonical
	return nil
}

func decodeCompatibilityInvocationSignature(value map[string]any) (*interfaces.InvocationSignatureConfig, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var generated factoryapi.FactoryInvocationSignature
	if err := json.Unmarshal(raw, &generated); err != nil {
		return nil, err
	}
	return invocationSignatureInternalFromAPI(&generated), nil
}

func mapLegacyInvocationExample(value any, signature *interfaces.InvocationSignatureConfig, index int) (map[string]any, error) {
	path := fmt.Sprintf("factory.invocationSignature.examples[%d]", index)
	record, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", path)
	}
	for key := range record {
		switch key {
		case "name", "description", "argv", "stdin":
		default:
			return nil, fmt.Errorf("%s.%s is not supported", path, key)
		}
	}
	name, ok := record["name"].(string)
	if !ok || strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%s.name must be a non-empty string", path)
	}
	description := name
	if rawDescription, exists := record["description"]; exists {
		var ok bool
		description, ok = rawDescription.(string)
		if !ok || strings.TrimSpace(description) == "" {
			return nil, fmt.Errorf("%s.description must be a non-empty string", path)
		}
	}
	argv, err := legacyStringArray(record["argv"], path+".argv")
	if err != nil {
		return nil, err
	}
	var stdin *string
	if rawStdin, exists := record["stdin"]; exists {
		text, ok := rawStdin.(string)
		if !ok {
			return nil, fmt.Errorf("%s.stdin must be a string", path)
		}
		stdin = &text
	}
	args, err := normalizeLegacyExampleArguments(signature, argv, stdin)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return map[string]any{
		"name": name,
		"description": map[string]any{
			"type":  interfaces.NameValueTypeLocalizableAsset,
			"value": description,
		},
		"args": args,
	}, nil
}

func legacyStringArray(value any, path string) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an array of strings", path)
	}
	result := make([]string, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("%s[%d] must be a string", path, index)
		}
		result[index] = text
	}
	return result, nil
}

func normalizeLegacyExampleArguments(signature *interfaces.InvocationSignatureConfig, argv []string, stdin *string) (map[string]any, error) {
	normalized, err := (work.InvocationExampleNormalizer{}).NormalizeLegacyInvocationExample(argv, signature, stdin)
	if err != nil {
		return nil, err
	}
	valueModes := make(map[string]string, len(signature.Parameters))
	for _, parameter := range signature.Parameters {
		valueModes[parameter.Name] = work.NormalizeInvocationValueMode(parameter.ValueMode)
	}
	args := make(map[string]any)
	for name, argument := range normalized.Arguments {
		if onlyDefaultSources(argument.Sources) {
			continue
		}
		if valueModes[name] == work.InvocationParameterValueModeRepeated || valueModes[name] == work.InvocationParameterValueModeVariadic {
			args[name] = append([]string(nil), argument.Values...)
		} else if len(argument.Values) == 1 {
			args[name] = argument.Values[0]
		} else {
			args[name] = append([]string(nil), argument.Values...)
		}
	}
	for name, values := range normalized.UnknownNamedArgs {
		if len(values) == 1 {
			args[name] = values[0]
		} else {
			args[name] = append([]string(nil), values...)
		}
	}
	return args, nil
}

func onlyDefaultSources(sources []work.ArgumentSource) bool {
	if len(sources) == 0 {
		return false
	}
	for _, source := range sources {
		if source.Kind != work.ArgumentSourceKindDefault {
			return false
		}
	}
	return true
}
