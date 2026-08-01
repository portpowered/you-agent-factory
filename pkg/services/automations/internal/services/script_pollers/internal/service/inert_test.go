package service_test

import (
	"testing"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	scriptpollerswire "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestNewServiceIsInert(t *testing.T) {
	t.Parallel()

	var loggerCalls int
	var clockCalls int
	var runnerCalls int

	service := scriptpollerswire.NewService(
		func(workstationName, workerName string) *zap.Logger {
			loggerCalls++
			return zap.NewNop()
		},
		func() clockwork.Clock {
			clockCalls++
			return clockwork.NewFakeClock()
		},
		func() workers.CommandRunner {
			runnerCalls++
			return nil
		},
		nil,
		nil,
	)
	if service == nil {
		t.Fatal("expected inert script pollers service")
	}
	if loggerCalls != 0 || clockCalls != 0 || runnerCalls != 0 {
		t.Fatalf(
			"construction invoked dependencies: logger=%d clock=%d runner=%d, want 0",
			loggerCalls,
			clockCalls,
			runnerCalls,
		)
	}
}
