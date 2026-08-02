package service_test

import (
	"testing"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
)

func TestNewServiceIsInert(t *testing.T) {
	t.Parallel()

	service := scriptpollerswire.NewService(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		nil,
	)
	if service == nil {
		t.Fatal("expected inert script pollers service")
	}
}
