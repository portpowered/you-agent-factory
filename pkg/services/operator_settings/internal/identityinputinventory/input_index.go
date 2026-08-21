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
		FormatVersion:      operatorsettings.InputInventoryFormatVersion,
		UnknownFieldPolicy: "unknown object fields are ignored at any nesting level and reported as sorted unique JSON paths; known-field validation and exactly one JSON document remain strict",
		PrecedenceChain:    operatorsettings.PrecedenceChain,
		Cases:              cases,
	}
}
