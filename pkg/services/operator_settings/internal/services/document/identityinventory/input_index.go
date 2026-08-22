package identityinventory

// ProjectInputInventory builds the deterministic system-config input inventory
// from committed fixtures and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	cases := make([]InputCase, 0, 12)
	cases = append(cases, ensureScopeInputCases()...)
	cases = append(cases, persistScopeInputCases()...)

	return InputInventory{
		FormatVersion:       InputInventoryFormatVersion,
		UnknownFieldPolicy:  "EnsureLocalBackendScope accepts unknown object fields through the tolerant GlobalConfig decoder; known-field validation and exactly one JSON document remain strict, with ignored paths available to the owning caller",
		SiblingPreservation: "persistBackendScopeID re-encodes the generated GlobalConfig contract and preserves decoded defaults and workerPresets",
		Cases:               cases,
	}
}
