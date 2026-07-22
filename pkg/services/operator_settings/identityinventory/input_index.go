package identityinventory

// ProjectInputInventory builds the deterministic system-config input inventory
// from committed fixtures and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	cases := make([]InputCase, 0, 12)
	cases = append(cases, ensureScopeInputCases()...)
	cases = append(cases, persistScopeInputCases()...)

	return InputInventory{
		FormatVersion: InputInventoryFormatVersion,
		UnknownFieldPolicy: "loadBackendScopeID unmarshals only backendScopeID and ignores other top-level keys on read; " +
			"persistBackendScopeID rewrites through a raw-message map that preserves unrelated sibling keys",
		SiblingPreservation: "persistBackendScopeID preserves defaults, workerPresets, and unknown top-level sibling keys already present in the shared config file",
		Cases:               cases,
	}
}
