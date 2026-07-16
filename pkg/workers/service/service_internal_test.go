package service

import "testing"

func TestNilServiceUsesSafeSchedulerDefaults(t *testing.T) {
	t.Parallel()

	var svc *Service
	if svc.logger() == nil || svc.commandRunner() == nil || svc.supervisorClock() == nil || svc.pollerLogger("workstation", "worker") == nil {
		t.Fatal("nil worker service did not provide safe scheduler defaults")
	}
}
