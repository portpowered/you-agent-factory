package identityinputinventory

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

// ProjectInputInventory builds the deterministic operator-config input inventory
// from committed fixtures and documented loader outcomes.
func ProjectInputInventory() operatorsettings.InputInventory {
	cases := make([]operatorsettings.InputCase, 0, 25)
	cases = append(cases, parseValidInputCases()...)
	cases = append(cases, parseInvalidDefaultsInputCases()...)
	cases = append(cases, parseInvalidWorkerPresetInputCases()...)
	cases = append(cases, loadInputCases()...)
	cases = append(cases, resolveInputCases()...)

	return operatorsettings.InputInventory{
		FormatVersion: operatorsettings.InputInventoryFormatVersion,
		UnknownFieldPolicy: "the generated GlobalConfig decoder uses json.Decoder.DisallowUnknownFields and rejects unknown top-level keys, " +
			"unknown nested keys, and trailing JSON values",
		PrecedenceChain: operatorsettings.PrecedenceChain,
		Cases:           cases,
	}
}
