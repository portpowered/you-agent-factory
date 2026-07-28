package automations_test

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/automations"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	automationinternal "github.com/portpowered/infinite-you/pkg/services/automations/internal"
	"go.uber.org/zap"
)

func TestAutomationsRootReExportsCronScheduling(t *testing.T) {
	t.Parallel()

	if err := automations.ValidateCronSchedule("*/5 * * * *"); err != nil {
		t.Fatalf("ValidateCronSchedule() error = %v", err)
	}
	if err := automations.ValidateCronSchedule(""); err == nil {
		t.Fatal("ValidateCronSchedule() accepted empty schedule")
	}

	jitter, err := automations.ParseCronJitter(&interfaces.CronConfig{Jitter: "5s"})
	if err != nil {
		t.Fatalf("ParseCronJitter() error = %v", err)
	}
	if jitter != 5*time.Second {
		t.Fatalf("ParseCronJitter() = %s, want 5s", jitter)
	}

	expiry, err := automations.ParseCronExpiryWindow(
		&interfaces.CronConfig{ExpiryWindow: "1h"},
		time.Minute,
	)
	if err != nil {
		t.Fatalf("ParseCronExpiryWindow() error = %v", err)
	}
	if expiry != time.Hour {
		t.Fatalf("ParseCronExpiryWindow() = %s, want 1h", expiry)
	}
}

func TestAutomationsServiceConstructsCronOwnerInertly(t *testing.T) {
	t.Parallel()

	service := automationinternal.NewService(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		"workflow-composition",
		"",
		nil,
		nil,
		nil,
	)
	if service == nil {
		t.Fatal("NewService returned nil")
	}
	if err := automations.ValidateCronSchedule("* * * * *"); err != nil {
		t.Fatalf("root cron re-export failed after service construction: %v", err)
	}
}
