package lineagegraph

func cloneContentParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]ContentPart, len(parts))
	for i, part := range parts {
		cloned[i] = part
		cloned[i].JSON = append([]byte(nil), part.JSON...)
		cloned[i].Metadata = cloneAnyMap(part.Metadata)
	}
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	clone := make(map[string]any, len(values))
	for key, value := range values {
		clone[key] = cloneAnyValue(value)
	}
	return clone
}

func cloneAnyValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMap(typed)
	case []any:
		return cloneAnySlice(typed)
	default:
		return value
	}
}

func cloneAnySlice(values []any) []any {
	if len(values) == 0 {
		return nil
	}
	clone := make([]any, len(values))
	for index, value := range values {
		clone[index] = cloneAnyValue(value)
	}
	return clone
}
