package authored

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorydefinition/retiredboundary"
	"gopkg.in/yaml.v3"
)

func decodeAgentsFrontmatter(
	frontmatter []byte,
	target any,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) error {
	rawFrontmatter, err := parseAgentsFrontmatterMap(frontmatter)
	if err != nil {
		return err
	}
	if err := validateAgentsFrontmatterFields(
		rawFrontmatter,
		reflect.TypeOf(target),
		"frontmatter",
		retiredAliases,
	); err != nil {
		return err
	}
	normalizeAgentsRuntimeResources(rawFrontmatter)
	normalized, err := yaml.Marshal(rawFrontmatter)
	if err != nil {
		return fmt.Errorf("normalize authored frontmatter: %w", err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(normalized))
	decoder.KnownFields(true)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("multiple frontmatter YAML documents are not supported")
		}
		return fmt.Errorf("decode trailing frontmatter YAML: %w", err)
	}
	return nil
}

func validateAgentsFrontmatterFields(
	value any,
	targetType reflect.Type,
	path string,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) error {
	targetType = indirectAuthoredType(targetType)
	switch targetType.Kind() {
	case reflect.Struct:
		return validateAuthoredStructFields(value, targetType, path, retiredAliases)
	case reflect.Slice, reflect.Array:
		return validateAuthoredSequenceFields(value, targetType, path, retiredAliases)
	case reflect.Map:
		return validateAuthoredMapFields(value, targetType, path, retiredAliases)
	default:
		return nil
	}
}

func indirectAuthoredType(targetType reflect.Type) reflect.Type {
	for targetType.Kind() == reflect.Pointer {
		targetType = targetType.Elem()
	}
	return targetType
}

func validateAuthoredStructFields(
	value any,
	targetType reflect.Type,
	path string,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) error {
	container, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	fields := authoredYAMLFieldTypes(targetType)
	for key, child := range container {
		if err := validateAuthoredStructField(child, fields, path, key, retiredAliases); err != nil {
			return err
		}
	}
	return nil
}

func validateAuthoredStructField(
	value any,
	fields map[string]reflect.Type,
	path string,
	key string,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) error {
	if alias := authoredRetiredAlias(path, key, retiredAliases); alias != nil {
		return fmt.Errorf(
			"%s.%s is not supported; %s",
			path,
			key,
			alias.Replacement,
		)
	}
	childType, ok := fields[key]
	if !ok {
		return fmt.Errorf(
			"%s.%s is not supported; use a canonical authored field",
			path,
			key,
		)
	}
	return validateAgentsFrontmatterFields(value, childType, path+"."+key, retiredAliases)
}

func validateAuthoredSequenceFields(
	value any,
	targetType reflect.Type,
	path string,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) error {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	for index, child := range values {
		if err := validateAgentsFrontmatterFields(
			child,
			targetType.Elem(),
			fmt.Sprintf("%s[%d]", path, index),
			retiredAliases,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateAuthoredMapFields(
	value any,
	targetType reflect.Type,
	path string,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) error {
	if targetType.Key().Kind() != reflect.String {
		return nil
	}
	values, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	for key, child := range values {
		if err := validateAgentsFrontmatterFields(
			child,
			targetType.Elem(),
			fmt.Sprintf("%s.%s", path, key),
			retiredAliases,
		); err != nil {
			return err
		}
	}
	return nil
}

func authoredYAMLFieldTypes(targetType reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type)
	for index := 0; index < targetType.NumField(); index++ {
		field := targetType.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
		if tag == "-" {
			continue
		}
		if tag == "" {
			tag = strings.ToLower(field.Name)
		}
		fields[tag] = field.Type
	}
	return fields
}

func authoredRetiredAlias(
	path string,
	key string,
	retiredAliases []retiredboundary.RetiredFieldAlias,
) *retiredboundary.RetiredFieldAlias {
	aliases := retiredAliases
	if path == "frontmatter.cron" {
		aliases = retiredboundary.RetiredCronFieldAliases()
	}
	for index := range aliases {
		if aliases[index].Key == key {
			return &aliases[index]
		}
	}
	return nil
}
