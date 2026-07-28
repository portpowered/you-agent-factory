// Package testlink is a transitional shim over internal test registration helpers.
// Implementation lives under operator_settings/internal/testlink.
package testlink

import internaltestlink "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/testlink"

// RegisterDocumentOwner wires the nested document owner into Operator Settings unit tests.
var RegisterDocumentOwner = internaltestlink.RegisterDocumentOwner

// RegisterProvidersRoot wires the Providers root constructor used by transitional tests.
var RegisterProvidersRoot = internaltestlink.RegisterProvidersRoot

// RegisterComposition wires transitional Settings composition hooks for tests.
var RegisterComposition = internaltestlink.RegisterComposition
