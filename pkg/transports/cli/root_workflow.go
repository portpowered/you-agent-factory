package cli

import (
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func newSessionHandlerRegistry(options CommandFactory) (*commandregistry.Registry, error) {
	manifest, err := generated.SessionFamilyManifest()
	if err != nil {
		return nil, err
	}
	return commandregistry.NewSessionResolvedRegistry(manifest, options.sessionResolvedHandlers)
}
