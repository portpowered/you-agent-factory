package wire

import internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"

// RegisterTestComposition wires Operator Settings test composition hooks for
// external test packages that cannot import operator_settings/internal/testlink.
func RegisterTestComposition() {
	internaltestlink.RegisterComposition()
}

// RegisterTestProvidersRoot wires the test Providers root constructor for
// external test packages that cannot import operator_settings/internal/testlink.
func RegisterTestProvidersRoot() {
	internaltestlink.RegisterProvidersRoot()
}
