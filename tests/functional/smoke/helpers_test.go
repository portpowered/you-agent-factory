package smoke

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
