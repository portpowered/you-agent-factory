package invocationreturnpolicy

func cloneContentParts(parts []ContentPart) []ContentPart {
	if len(parts) == 0 {
		return nil
	}
	cloned := make([]ContentPart, len(parts))
	for i, part := range parts {
		cloned[i] = part
		cloned[i].JSON = append([]byte(nil), part.JSON...)
		if part.Metadata != nil {
			cloned[i].Metadata = make(map[string]any, len(part.Metadata))
			for key, value := range part.Metadata {
				cloned[i].Metadata[key] = value
			}
		}
	}
	return cloned
}
