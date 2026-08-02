package cli

import (
	"context"

	modelservice "github.com/portpowered/infinite-you/pkg/services/models"
)

// PresentationCollaborator is the Models CLI-owned port for opening the
// model presentation scopes needed by invoke. It is an adapter contract, not
// a Factory Sessions service authority.
type PresentationCollaborator interface {
	ModelsPresentationRoot() modelservice.Service
	OpenModelsCatalogScope(context.Context) (PresentationScope, error)
	OpenModelsPresentationScope(context.Context, PresentationScopeRequest) (PresentationScope, error)
}

type PresentationScopeRequest = modelservice.PresentationScopeRequest
type PresentationScope = modelservice.PresentationScope
type PresentationOperatorDefaults = modelservice.PresentationOperatorDefaults
