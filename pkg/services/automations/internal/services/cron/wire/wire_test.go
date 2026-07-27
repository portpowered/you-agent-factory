package wire_test

import (
	"testing"

	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	cronwire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron/wire"
)

func TestNewServiceConstructsCronOwner(t *testing.T) {
	t.Parallel()

	service := cronwire.NewService()
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	var _ cron.Service = service
}
