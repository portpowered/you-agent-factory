package climanifestcobra

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// PersistentFlagBindings supplies live variables for root persistent flags declared
// in generated representative-family metadata.
type PersistentFlagBindings struct {
	Verbose                    *bool
	Debug                      *bool
	Server                     *string
	JSON                       *bool
	DefaultWorkerModelProvider *string
	DefaultWorkerModel         *string
	// FlagUsages supplies Cobra help text for persistent flags when the manifest
	// does not yet carry per-flag usage descriptions.
	FlagUsages map[string]string
}

const genericInputAnnotationPrefix = "infinite-you/input-id/"

// InputValues returns the parsed named inputs declared for cmd, keyed by their
// stable manifest input IDs. Returned repeated values are detached copies.
func InputValues(cmd *cobra.Command) (map[string]any, error) {
	if cmd == nil {
		return nil, fmt.Errorf("read generic command inputs: command is required")
	}
	values := make(map[string]any)
	for key, longName := range cmd.Annotations {
		if !strings.HasPrefix(key, genericInputAnnotationPrefix) {
			continue
		}
		inputID := strings.TrimPrefix(key, genericInputAnnotationPrefix)
		flag := lookupCommandFlag(cmd, longName)
		if flag == nil {
			return nil, fmt.Errorf("read generic command input %q: flag %q is unavailable", inputID, longName)
		}
		getter, ok := flag.Value.(interface{ Get() any })
		if !ok {
			return nil, fmt.Errorf("read generic command input %q: flag %q has no typed value", inputID, longName)
		}
		values[inputID] = cloneGenericInputValue(getter.Get())
	}
	return values, nil
}

func lookupCommandFlag(cmd *cobra.Command, name string) *pflag.Flag {
	for current := cmd; current != nil; current = current.Parent() {
		if flag := current.Flags().Lookup(name); flag != nil {
			return flag
		}
		if flag := current.PersistentFlags().Lookup(name); flag != nil {
			return flag
		}
	}
	return nil
}

type genericFlagValue struct {
	valueType     string
	normalization string
	enum          map[string]struct{}
	value         any
	arrayChanged  bool
}

func newGenericFlagValue(flag climanifest.Flag) (*genericFlagValue, error) {
	value, err := genericFlagDefault(flag)
	if err != nil {
		return nil, err
	}
	choices := make(map[string]struct{}, len(flag.Enum))
	for _, choice := range flag.Enum {
		choices[choice] = struct{}{}
	}
	return &genericFlagValue{
		valueType:     flag.ValueType,
		normalization: flag.Normalization,
		enum:          choices,
		value:         cloneGenericInputValue(normalizeGenericInput(value, flag.Normalization)),
	}, nil
}

func (v *genericFlagValue) Set(raw string) error {
	value, err := parseGenericFlagString(v.valueType, raw)
	if err != nil {
		return err
	}
	value = normalizeGenericInput(value, v.normalization)
	if err := validateGenericChoice(v.enum, value); err != nil {
		return err
	}
	if v.valueType != "stringArray" {
		v.value = value
		return nil
	}
	item := value.([]string)[0]
	if !v.arrayChanged {
		v.value = []string{item}
		v.arrayChanged = true
		return nil
	}
	v.value = append(v.value.([]string), item)
	return nil
}

func (v *genericFlagValue) Type() string {
	return v.valueType
}

func (v *genericFlagValue) String() string {
	return genericFlagString(v.value)
}

func (v *genericFlagValue) Get() any {
	return cloneGenericInputValue(v.value)
}

func (v *genericFlagValue) Append(raw string) error {
	if v.valueType != "stringArray" {
		return fmt.Errorf("value type %q is not a repeated string", v.valueType)
	}
	value, err := parseGenericFlagString(v.valueType, raw)
	if err != nil {
		return err
	}
	item := normalizeGenericInput(value, v.normalization).([]string)[0]
	if err := validateGenericChoice(v.enum, item); err != nil {
		return err
	}
	v.value = append(v.value.([]string), item)
	return nil
}

func (v *genericFlagValue) Replace(values []string) error {
	replacement := make([]string, 0, len(values))
	for _, raw := range values {
		value := normalizeGenericInput([]string{raw}, v.normalization).([]string)[0]
		if err := validateGenericChoice(v.enum, value); err != nil {
			return err
		}
		replacement = append(replacement, value)
	}
	v.value = replacement
	v.arrayChanged = true
	return nil
}

func (v *genericFlagValue) GetSlice() []string {
	if values, ok := v.value.([]string); ok {
		return append([]string(nil), values...)
	}
	return nil
}

func genericFlagDefault(flag climanifest.Flag) (any, error) {
	if flag.DefaultValue != nil {
		return typedGenericInputValue(flag.ValueType, flag.DefaultValue)
	}
	if flag.ValueType == "stringArray" {
		if flag.Default != "" {
			return nil, fmt.Errorf("repeated-string default must use defaultValue.stringArray")
		}
		return []string{}, nil
	}
	return parseGenericFlagString(flag.ValueType, flag.Default)
}

func hasGenericFlagDefault(flag climanifest.Flag) bool {
	if flag.DefaultValue != nil || flag.Default != "" {
		return true
	}
	for _, source := range flag.AcceptedSources {
		if source == "manifest-default" {
			return true
		}
	}
	return false
}

func genericNoOptionValue(flag climanifest.Flag) (any, error) {
	if flag.NoOptionValue != nil {
		return typedGenericInputValue(flag.ValueType, flag.NoOptionValue)
	}
	return parseGenericFlagString(flag.ValueType, flag.NoOptionDefault)
}

func typedGenericInputValue(valueType string, authored *climanifest.InputValue) (any, error) {
	count := 0
	var value any
	if authored.Boolean != nil {
		count++
		value = *authored.Boolean
	}
	if authored.String != nil {
		count++
		value = *authored.String
	}
	if authored.Int != nil {
		count++
		value = *authored.Int
	}
	if authored.Int64 != nil {
		count++
		value = *authored.Int64
	}
	if authored.StringArray != nil {
		count++
		value = append([]string(nil), (*authored.StringArray)...)
	}
	if count != 1 {
		return nil, fmt.Errorf("typed value must author exactly one member")
	}
	matches := map[string]bool{
		"bool":        authored.Boolean != nil,
		"string":      authored.String != nil,
		"int":         authored.Int != nil,
		"int64":       authored.Int64 != nil,
		"stringArray": authored.StringArray != nil,
	}
	if !matches[valueType] {
		return nil, fmt.Errorf("typed value member does not match value type %q", valueType)
	}
	return value, nil
}

func parseGenericFlagString(valueType, raw string) (any, error) {
	switch valueType {
	case "bool":
		if raw == "" {
			return false, nil
		}
		return strconv.ParseBool(raw)
	case "string":
		return raw, nil
	case "int":
		if raw == "" {
			return 0, nil
		}
		return strconv.Atoi(raw)
	case "int64":
		if raw == "" {
			return int64(0), nil
		}
		return strconv.ParseInt(raw, 10, 64)
	case "stringArray":
		return []string{raw}, nil
	default:
		return nil, fmt.Errorf("unsupported value type %q", valueType)
	}
}

func normalizeGenericInput(value any, mode string) any {
	if mode != "trim" {
		return value
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []string:
		normalized := make([]string, len(typed))
		for index := range typed {
			normalized[index] = strings.TrimSpace(typed[index])
		}
		return normalized
	default:
		return value
	}
}

func validateEnumValue(flag climanifest.Flag, value any) error {
	choices := make(map[string]struct{}, len(flag.Enum))
	for _, choice := range flag.Enum {
		choices[choice] = struct{}{}
	}
	return validateGenericChoice(choices, normalizeGenericInput(value, flag.Normalization))
}

func validateGenericChoice(choices map[string]struct{}, value any) error {
	if len(choices) == 0 {
		return nil
	}
	values, ok := value.([]string)
	if !ok {
		values = []string{value.(string)}
	}
	for _, candidate := range values {
		if _, exists := choices[candidate]; !exists {
			return fmt.Errorf("value %q is not one of the declared choices", candidate)
		}
	}
	return nil
}

func genericFlagString(value any) string {
	switch typed := value.(type) {
	case []string:
		return "[" + strings.Join(typed, ",") + "]"
	default:
		return fmt.Sprint(value)
	}
}

func cloneGenericInputValue(value any) any {
	if values, ok := value.([]string); ok {
		return append([]string(nil), values...)
	}
	return value
}
