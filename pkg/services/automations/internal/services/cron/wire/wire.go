// Package wire constructs the Automations cron subservice.
package wire

import (
	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cronservice "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/internal/service"
)

// NewService constructs an inert cron scheduling service.
func NewService() cron.Service {
	return cronservice.New()
}
