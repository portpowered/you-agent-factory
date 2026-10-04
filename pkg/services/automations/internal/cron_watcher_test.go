package internal_test

import (
	"context"
	"github.com/jonboulle/clockwork"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"testing"
	"time"
)

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
}

func cronWorkstationConfigForTest(name string) interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: name,
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{Schedule: "* * * * *"},
		Outputs: []interfaces.IOConfig{
			{WorkTypeName: "task", StateName: "init"},
		},
	}
}

func waitForCronWorkRequest(t *testing.T, requests <-chan work.WorkRequest, timeout time.Duration) work.WorkRequest {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(timeout):
		t.Fatal("timed out waiting for cron work request")
		return work.WorkRequest{}
	}
}

func assertNoCronWorkRequestQueued(t *testing.T, requests <-chan work.WorkRequest) {
	t.Helper()
	select {
	case request := <-requests:
		t.Fatalf("unexpected cron work request: %#v", request)
	default:
	}
}

func assertCronWorkRequestForWorkstation(t *testing.T, request work.WorkRequest, want time.Time, workstation string) {
	t.Helper()
	assertCronWorkRequestNominalAt(t, request, want)
	if got := request.Works[0].Tags[interfaces.TimeWorkTagKeyCronWorkstation]; got != workstation {
		t.Fatalf("cron workstation tag = %q, want %q", got, workstation)
	}
}

func assertCronWorkRequestNominalAt(t *testing.T, request work.WorkRequest, want time.Time) {
	t.Helper()
	got := request.Works[0].Tags[interfaces.TimeWorkTagKeyNominalAt]
	wantTag := want.Format(time.RFC3339Nano)
	if got != wantTag {
		t.Fatalf("cron nominal_at tag = %q, want %q", got, wantTag)
	}
}
