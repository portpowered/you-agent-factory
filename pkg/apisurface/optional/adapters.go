package optional

import factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"

func StringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func IntValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}

func NonEmptyStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func PositiveIntPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func NonZeroIntPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func TrueBoolPtr(value bool) *bool {
	if !value {
		return nil
	}
	return &value
}

func StringsValue(values *[]string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), (*values)...)
}

func CopiedStringsPtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	copied := append([]string(nil), values...)
	return &copied
}

func ReferencedStringsPtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	return &values
}

func CopiedStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	copied := make(factoryapi.StringMap, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return &copied
}

func ReferencedStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap(values)
	return &converted
}

func StringMapValue(values *factoryapi.StringMap) map[string]string {
	if values == nil {
		return nil
	}
	out := make(map[string]string, len(*values))
	for key, value := range *values {
		out[key] = value
	}
	return out
}
