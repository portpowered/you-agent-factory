// Package cli defines the Work service-owned CLI adapter.
package cli

import workdomain "github.com/portpowered/infinite-you/pkg/services/work"

// Service exposes Work CLI command operations to Cobra composition.
type Service interface {
	List(ListConfig) error
	Show(ShowConfig) error
	Move(MoveConfig) error
	Visualize(VisualizeConfig) error
}

// Config carries accepted Work-root collaborators for adapter construction.
type Config struct {
	ListPrepare workdomain.ListRequestPreparation
	Visualize   workdomain.VisualizationOperation
}

type service struct {
	listPrepare workdomain.ListRequestPreparation
	visualize   workdomain.VisualizationOperation
}

// New constructs the Work CLI service from the accepted Work list-preparation
// and visualization collaborators required by list/show/move/visualize.
func New(cfg Config) Service {
	return &service{
		listPrepare: cfg.ListPrepare,
		visualize:   cfg.Visualize,
	}
}
