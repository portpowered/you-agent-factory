package climanifestcobra

import (
	"context"

	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/spf13/cobra"
)

// CompletionRegistry maps stable manifest input IDs to transport-edge dynamic
// completion callbacks.
type CompletionRegistry map[string]cobra.CompletionFunc

// GenericHandler receives one invocation's normalized inputs keyed by stable
// manifest input ID. It remains independent of Cobra and public command names.
type GenericHandler func(context.Context, map[string]any) error

// HandlerRegistry maps stable manifest handler IDs to transport-edge behavior.
type HandlerRegistry map[string]GenericHandler

// CobraHandler adapts an existing transport-owned Cobra handler while a command
// family migrates to normalized stable-ID inputs. New handler implementations
// should prefer GenericHandler so they remain independent of Cobra.
type CobraHandler func(*cobra.Command, []string, map[string]any) error

// CobraHandlerRegistry maps stable manifest handler IDs to migration adapters.
type CobraHandlerRegistry map[string]CobraHandler

// InputBinding receives a normalized value whenever Cobra parses the named
// stable input. It is a migration seam for handlers that still own typed
// option structs; generic handlers should consume their values map directly.
type InputBinding func(any) error

// InputBindingRegistry maps stable manifest input IDs to migration bindings.
type InputBindingRegistry map[string]InputBinding

// GenericBindings supplies executable transport bindings used while projecting
// a generic manifest. Additional stable-ID registries can be added here without
// coupling manifest records to public command or input spellings.
type GenericBindings struct {
	Completions   CompletionRegistry
	Handlers      HandlerRegistry
	CobraHandlers CobraHandlerRegistry
	Inputs        InputBindingRegistry
}

// SessionFamilyBindings supplies the legacy typed option structs updated by
// stable input-ID bindings while Session handlers migrate to normalized values.
type SessionFamilyBindings struct {
	Create     *sessioncli.CreateConfig
	List       *sessioncli.ListConfig
	Delete     *sessioncli.DeleteConfig
	Dispatches *sessioncli.DispatchesConfig
	Pause      *sessioncli.LifecycleControlConfig
	Resume     *sessioncli.LifecycleControlConfig
	FlagUsages map[string]string
}
