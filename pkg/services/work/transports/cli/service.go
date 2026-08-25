// Package cli defines the Work service-owned CLI adapter.
package cli

import (
	workdomain "github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

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

func normalizeWorkConfirmationState(work *factoryapi.Work) {
	if work == nil {
		return
	}
	normalizeStopSummaryConfirmationState(work.StopSummary)
	if work.ConfirmationState != nil {
		switch *work.ConfirmationState {
		case factoryapi.CONFIRMED, factoryapi.UNCONFIRMED:
			return
		}
	}
	state := factoryapi.UNCONFIRMED
	work.ConfirmationState = &state
}

func normalizeStopSummaryConfirmationState(summary *factoryapi.FactoryStopSummary) {
	if summary == nil || summary.LatestDispatch == nil {
		return
	}
	if summary.LatestDispatch.ConfirmationState == factoryapi.CONFIRMED {
		return
	}
	summary.LatestDispatch.ConfirmationState = factoryapi.UNCONFIRMED
}

func normalizeWorkConfirmationStates(result *factoryapi.ListWorkResponse) {
	if result == nil {
		return
	}
	for index := range result.Results {
		normalizeWorkConfirmationState(&result.Results[index])
	}
}

func workConfirmationState(work factoryapi.Work) string {
	normalizeWorkConfirmationState(&work)
	return string(*work.ConfirmationState)
}
