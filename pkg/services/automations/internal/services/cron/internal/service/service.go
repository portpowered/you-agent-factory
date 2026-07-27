package service

import (
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
)

type service struct{}

var _ cron.Service = (*service)(nil)

// New constructs an inert cron scheduling service.
func New() cron.Service {
	return &service{}
}
