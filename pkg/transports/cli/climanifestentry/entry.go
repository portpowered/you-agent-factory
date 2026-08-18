// Package climanifestentry publishes the composition-root entry point for the
// Work-owned CLI manifest run-input composition that non-owning transports
// consume.
//
// The canonical implementation is owned by the Work service under
// pkg/services/work/transports/cli/climanifest. Peer-service transports
// consume this package instead of reaching into that service's transport
// subpackages directly, which keeps CLI adapters owned by one service from
// depending on another service's transport tree.
package climanifestentry

import (
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/work/transports/cli/climanifest"
)

// Manifest is the static CLI manifest that run-input composition reads.
type Manifest = climanifest.Manifest

// EffectiveInputSchema is the composed run-input schema for one command.
type EffectiveInputSchema = climanifest.EffectiveInputSchema

// EffectiveFactoryInputModeSignature marks a schema whose Factory inputs are
// described by the selected Factory's invocation signature.
const EffectiveFactoryInputModeSignature = climanifest.EffectiveFactoryInputModeSignature

// ComposeRunInputs composes the effective run-input schema for one static
// command and the selected Factory's invocation signature.
func ComposeRunInputs(
	manifest Manifest,
	commandID string,
	signature *work.InvocationSignatureConfig,
) (EffectiveInputSchema, []climanifest.CompositionDiagnostic, error) {
	return climanifest.ComposeRunInputs(manifest, commandID, signature)
}
