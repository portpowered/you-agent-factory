package factorysession

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func deref(value *string) string {
	return derefString(value)
}
