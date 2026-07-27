package automations_test

import (
	"testing"

	"github.com/jonboulle/clockwork"
	hostedsourceswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/wire"
	"go.uber.org/zap"
)

func TestAutomationsHostedSourcesConstructsInertly(t *testing.T) {
	t.Parallel()

	service := hostedsourceswire.NewHostedPollers(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		nil,
		"",
	)
	if service == nil {
		t.Fatal("NewService returned nil")
	}
}
