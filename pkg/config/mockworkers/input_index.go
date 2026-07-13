package mockworkers

// ProjectInputInventory builds the deterministic mock-worker input inventory
// from committed fixtures, docs examples, and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	cases := make([]InputCase, 0, 16)
	cases = append(cases, parseValidInputCases()...)
	cases = append(cases, loadValidInputCases()...)

	return InputInventory{
		FormatVersion: InputInventoryFormatVersion,
		UnknownFieldPolicy: "ParseMockWorkersConfig uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, " +
			"unknown nested keys, and trailing JSON values",
		LoaderEntrypoints: []string{
			entrypointParseMockWorkersConfig,
			entrypointLoadMockWorkersConfig,
		},
		Cases: cases,
	}
}
