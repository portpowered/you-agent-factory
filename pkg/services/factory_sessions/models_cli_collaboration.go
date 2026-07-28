package factorysessions

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/models"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"go.uber.org/zap"
)

// ModelsCLIPresentationCollaborator exposes the accepted Models root and
// invoke-scope opening for the Models CLI adapter without importing CLI
// transport types.
type ModelsCLIPresentationCollaborator interface {
	ModelsPresentationRoot() models.Service
	OpenModelsCatalogScope(context.Context) (ModelsPresentationScope, error)
	OpenModelsPresentationScope(context.Context, ModelsPresentationScopeRequest) (ModelsPresentationScope, error)
}

// ModelsPresentationScopeRequest carries invoke-scope opening inputs selected
// for one Models CLI invocation.
type ModelsPresentationScopeRequest struct {
	FactoryDir       string
	HomeDir          string
	OperatorDefaults operatorsettings.ResolvedDefaults
	Logger           *zap.Logger
	Verbose          bool
	ModelCacheDir    string
}

// ModelsPresentationScope carries one opened Models runtime scope for CLI invoke.
type ModelsPresentationScope struct {
	Scope models.RuntimeScopeRef
	Close func(context.Context) error
}
