package requestadmission

import "encoding/json"

func cloneTags(tags map[string]string) map[string]string {
	if tags == nil {
		return nil
	}
	cloned := make(map[string]string, len(tags))
	for key, value := range tags {
		cloned[key] = value
	}
	return cloned
}

func cloneRelations(relations []Relation) []Relation {
	if relations == nil {
		return nil
	}
	cloned := make([]Relation, len(relations))
	copy(cloned, relations)
	return cloned
}

func clonePayload(payload []byte) []byte {
	if payload == nil {
		return nil
	}
	return append([]byte(nil), payload...)
}

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

func cloneInvocationArguments(args *InvocationArguments) *InvocationArguments {
	if args == nil || len(args.Arguments) == 0 {
		return nil
	}
	clone := &InvocationArguments{Arguments: make(map[string]InvocationArgument, len(args.Arguments))}
	for name, argument := range args.Arguments {
		next := InvocationArgument{
			Values:    cloneStringSlice(argument.Values),
			ValueMode: argument.ValueMode,
			Sensitive: argument.Sensitive,
		}
		if len(argument.Sources) > 0 {
			next.Sources = append([]InvocationArgumentSource(nil), argument.Sources...)
		}
		clone.Arguments[name] = next
	}
	return clone
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
	case []any:
		return cloneAnySlice(typed)
	case map[string]any:
		return cloneAnyMap(typed)
	case []string:
		return append([]string(nil), typed...)
	case []byte:
		return append([]byte(nil), typed...)
	case map[string]string:
		return cloneTags(typed)
	case map[string][]string:
		return cloneStringSliceMap(typed)
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

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string][]string, len(values))
	for key, items := range values {
		clone[key] = cloneStringSlice(items)
	}
	return clone
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	return append([]string(nil), values...)
}

func cloneRequestPayload(payload any) (any, error) {
	switch value := payload.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), value...), nil
	case json.RawMessage:
		return append(json.RawMessage(nil), value...), nil
	case string, bool, float64:
		return value, nil
	default:
		encoded, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		var cloned any
		if err := json.Unmarshal(encoded, &cloned); err != nil {
			return nil, err
		}
		return cloned, nil
	}
}

func cloneRequest(request Request) (Request, error) {
	cloned := request
	cloned.Relations = append([]WorkRelation(nil), request.Relations...)
	cloned.Works = make([]Work, len(request.Works))
	for index, item := range request.Works {
		clonedItem := item
		clonedItem.PreviousChainingTraceIDs = append([]string(nil), item.PreviousChainingTraceIDs...)
		clonedItem.Content = cloneContentParts(item.Content)
		payload, err := cloneRequestPayload(item.Payload)
		if err != nil {
			return Request{}, err
		}
		clonedItem.Payload = payload
		clonedItem.Tags = cloneTags(item.Tags)
		clonedItem.RuntimeRelations = cloneRelations(item.RuntimeRelations)
		clonedItem.InvocationArguments = cloneInvocationArguments(item.InvocationArguments)
		cloned.Works[index] = clonedItem
	}
	return cloned, nil
}
