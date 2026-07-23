package identityinventory

// ProjectInputInventory builds the deterministic system-config input inventory
// from committed fixtures and documented loader outcomes.
func ProjectInputInventory() InputInventory {
	cases := make([]InputCase, 0, 12)
	cases = append(cases, ensureScopeInputCases()...)
	cases = append(cases, persistScopeInputCases()...)

	return InputInventory{
		FormatVersion:       InputInventoryFormatVersion,
		UnknownFieldPolicy:  "EnsureLocalBackendScope decodes the generated closed GlobalConfig contract and rejects unknown fields before identity reuse or persistence",
		SiblingPreservation: "persistBackendScopeID re-encodes the generated GlobalConfig contract and preserves decoded defaults and workerPresets",
		Cases:               cases,
	}
}
