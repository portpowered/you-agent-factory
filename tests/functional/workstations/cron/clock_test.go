package cron

import (
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCronFiresAtInjectedTimeWithoutWallClockSleep proves cron workstations submit
// internal time-work at injected schedule boundaries, expire stale triggers, retry on
// the next boundary, dispatch when required inputs arrive, and keep time-work hidden
// from normal customer views by advancing a controllable external clock rather than
// waiting on wall-clock sleeps for schedule progress.
func TestCronFiresAtInjectedTimeWithoutWallClockSleep(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := support.ScaffoldFactory(t, cronSmokeFactoryConfig("* * * * *"))

	observedSubmissions := make(chan work.FactorySubmissionRecord, 32)
	fs := startCronServer(t, dir, withSubmissionRecorder(func(record work.FactorySubmissionRecord) {
		observedSubmissions <- record
	}), withClock(fakeClock))

	startupRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "startup-refresh", start, time.Second)
	assertCronSubmissionRecord(t, startupRecord, "startup-refresh", start)
	startupDispatch := waitForCronDispatch(t, fs, "startup-refresh", startupRecord.Request.WorkID, time.Second)
	startupWork := consumedCronTimeWork(t, startupDispatch, startupRecord.Request.WorkID)
	assertCronPublicMetadata(t, startupWork, "startup-refresh")
	assertCronTimeWorkHiddenFromNormalViews(t, fs, startupRecord.Request.WorkID)

	waitForFakeClockWaiters(t, fakeClock, 1)
	firstFire := start.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	firstFireRecords := waitForCronSubmissions(t, observedSubmissions, []string{"poll-for-work", "poll-with-input"}, firstFire, time.Second)

	noInputRecord := firstFireRecords["poll-for-work"]
	assertCronSubmissionRecord(t, noInputRecord, "poll-for-work", firstFire)
	noInputDispatch := waitForCronDispatch(t, fs, "poll-for-work", noInputRecord.Request.WorkID, time.Second)
	noInputWork := consumedCronTimeWork(t, noInputDispatch, noInputRecord.Request.WorkID)
	if support.StringPointerValue(noInputWork.WorkId) == "" {
		t.Fatal("no-input cron Work missing Work ID")
	}
	if support.StringPointerValue(noInputWork.TraceId) == "" {
		t.Fatal("no-input cron Work missing trace ID")
	}
	assertCronPublicMetadata(t, noInputWork, "poll-for-work")

	state := support.GetJSON[factoryapi.StatusResponse](t, fs.URL()+"/status")
	if state.RuntimeStatus == "" {
		t.Fatal("GET /state returned empty runtime_status after cron output")
	}
	if state.TotalTokens == 0 {
		t.Fatal("GET /state returned zero tokens after cron output")
	}
	noInputOutput := waitForTokenInPlaceByParent(t, fs, "task:init", noInputRecord.Request.WorkID, time.Second)
	if got := support.StringPointerValue(noInputOutput.Work.WorkTypeName); got != "task" {
		t.Fatalf("no-input cron output work type = %q, want task", got)
	}

	requiredInputRecord := firstFireRecords["poll-with-input"]
	assertCronSubmissionRecord(t, requiredInputRecord, "poll-with-input", firstFire)
	requiredInputToken := waitForCronToken(t, fs, "poll-with-input", requiredInputRecord.Request.WorkID, time.Second)
	assertCronPublicMetadata(t, requiredInputToken, "poll-with-input")
	assertCronTimeWorkHiddenFromNormalViews(t, fs, requiredInputRecord.Request.WorkID)
	assertCronTimeWorkRetainedInCanonicalHistory(t, fs, requiredInputRecord.Request.WorkID, "poll-with-input")

	assertNoCronDispatchForWorkstation(t, fs, "poll-with-input")
	assertNoCustomerCronOutput(t, fs, "poll-with-input")

	waitForFakeClockWaiters(t, fakeClock, 1)
	retryFire := firstFire.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	retryRecords := waitForCronSubmissions(t, observedSubmissions, []string{"poll-with-input"}, retryFire, time.Second)
	retryRecord := retryRecords["poll-with-input"]
	assertCronSubmissionRecord(t, retryRecord, "poll-with-input", retryFire)
	if retryRecord.Request.WorkID == requiredInputRecord.Request.WorkID {
		t.Fatal("required-input retry reused the stale cron time work ID")
	}
	waitForCronTimeWorkGone(t, fs, requiredInputRecord.Request.WorkID, time.Second)
	retryToken := waitForCronToken(t, fs, "poll-with-input", retryRecord.Request.WorkID, time.Second)
	assertCronPublicMetadata(t, retryToken, "poll-with-input")
	assertNoCronDispatchForWorkstation(t, fs, "poll-with-input")
	assertExpiredCronTimeWorkHandled(t, fs, requiredInputRecord.Request.WorkID, "poll-with-input")

	submittedSignals := fs.submitSignalWork(t, "signal-for-cron-smoke", "Cron smoke signal", []byte(`{"ready":true}`))
	signalWorkID := submittedSignals[0].WorkID
	requiredInputDispatch := waitForRequiredInputCronDispatch(t, fs, "poll-with-input", signalWorkID, 2*time.Second)
	requiredInputTimeWork := consumedCronTimeWork(t, requiredInputDispatch, retryRecord.Request.WorkID)
	if support.StringPointerValue(requiredInputTimeWork.WorkId) == requiredInputRecord.Request.WorkID {
		t.Fatalf("cron dispatched with expired time token %q after expiry; dispatch=%#v", requiredInputRecord.Request.WorkID, requiredInputDispatch)
	}
	assertCronPublicMetadata(t, requiredInputTimeWork, "poll-with-input")

	requiredOutput := waitForTokenInPlaceByParent(t, fs, "task:init", signalWorkID, 2*time.Second)
	if got := support.StringPointerValue(requiredOutput.Work.WorkTypeName); got != "task" {
		t.Fatalf("required-input cron output work type = %q, want task", got)
	}
	assertRequiredInputCronHistory(t, fs, requiredInputDispatch.DispatchID, signalWorkID)
	assertCronTimeWorkHiddenFromNormalViews(t, fs, support.StringPointerValue(requiredInputTimeWork.WorkId))
}
