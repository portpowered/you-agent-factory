package openapitests

// ProjectParityInventory builds the deterministic Factory/OpenAPI parity index
// from committed fixtures and documented API/config-loader outcomes.
func ProjectParityInventory() ParityInventory {
	cases := make([]ParityCase, 0, 20)
	cases = append(cases, baselineAcceptParityCases()...)
	cases = append(cases, baselineRejectParityCases()...)

	return ParityInventory{
		FormatVersion: ParityInventoryFormatVersion,
		Scope: "Factory/OpenAPI config parity index referencing existing openapitests " +
			"fixtures; each case records GeneratedFactoryFromOpenAPIJSON and " +
			"FactoryConfigFromOpenAPIJSON outcomes without changing schemas or mapping behavior",
		Cases: cases,
	}
}
