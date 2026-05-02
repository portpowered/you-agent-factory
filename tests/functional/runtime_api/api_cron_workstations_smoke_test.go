package runtime_api

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/factory"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=cron-end-to-end-smoke review=2026-07-18 removal=split-smoke-helpers-before-next-cron-e2e-expansion
func TestCronWorkstations_ServiceModeSmoke_SubmitsInternalTimeWorkExpiresRetriesDispatchesAndFiltersViews(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := support.ScaffoldFactory(t, cronSmokeFactoryConfig("* * * * *"))

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 32)
	fs := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Clock = fakeClock
	}, factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
		observedSubmissions <- record
	}))

	startupRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "startup-refresh", start, time.Second)
	assertCronSubmissionRecord(t, startupRecord, "startup-refresh", start)
	startupDispatch := waitForCronDispatch(t, fs, "startup-refresh", startupRecord.Request.WorkID, time.Second)
	startupToken := consumedCronTimeToken(t, startupDispatch, startupRecord.Request.WorkID)
	assertCronPayload(t, startupToken, "startup-refresh")
	assertCronTimeWorkHiddenFromNormalViews(t, fs, startupRecord.Request.WorkID)

	waitForFakeClockWaiters(t, fakeClock, 1)
	firstFire := start.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	firstFireRecords := waitForCronSubmissions(t, observedSubmissions, []string{"poll-for-work", "poll-with-input"}, firstFire, time.Second)

	noInputRecord := firstFireRecords["poll-for-work"]
	assertCronSubmissionRecord(t, noInputRecord, "poll-for-work", firstFire)
	noInputDispatch := waitForCronDispatch(t, fs, "poll-for-work", noInputRecord.Request.WorkID, time.Second)
	noInputToken := consumedCronTimeToken(t, noInputDispatch, noInputRecord.Request.WorkID)
	if noInputToken.Color.WorkID == "" {
		t.Fatal("no-input cron token missing work ID")
	}
	if noInputToken.Color.TraceID == "" {
		t.Fatal("no-input cron token missing trace ID")
	}
	assertCronPayload(t, noInputToken, "poll-for-work")

	state := getGeneratedJSON[factoryapi.StatusResponse](t, fs.URL()+"/status")
	if state.RuntimeStatus == "" {
		t.Fatal("GET /state returned empty runtime_status after cron output")
	}
	if state.TotalTokens == 0 {
		t.Fatal("GET /state returned zero tokens after cron output")
	}
	noInputOutput := waitForTokenInPlaceByParent(t, fs, "task:init", noInputRecord.Request.WorkID, time.Second)
	if noInputOutput.Color.WorkTypeID != "task" {
		t.Fatalf("no-input cron output work type = %q, want task", noInputOutput.Color.WorkTypeID)
	}

	requiredInputRecord := firstFireRecords["poll-with-input"]
	assertCronSubmissionRecord(t, requiredInputRecord, "poll-with-input", firstFire)
	requiredInputToken := waitForCronToken(t, fs, "poll-with-input", requiredInputRecord.Request.WorkID, time.Second)
	assertCronPayload(t, requiredInputToken, "poll-with-input")
	assertCronTimeWorkHiddenFromNormalViews(t, fs, requiredInputRecord.Request.WorkID)
	assertCronTimeWorkRetainedInCanonicalHistory(t, fs, requiredInputRecord.Request.WorkID, "poll-with-input")

	pendingWithoutInput := fs.GetEngineStateSnapshot(t)
	assertNoCronDispatchForWorkstation(t, pendingWithoutInput, "poll-with-input")
	assertNoCustomerCronOutput(t, fs.GetEngineStateSnapshot(t), "poll-with-input")

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
	assertCronPayload(t, retryToken, "poll-with-input")
	assertNoCronDispatchForWorkstation(t, fs.GetEngineStateSnapshot(t), "poll-with-input")
	assertExpiredCronTimeWorkHandled(t, fs, requiredInputRecord.Request.WorkID, "poll-with-input")

	submittedSignals := fs.SubmitRuntimeWork(t, interfaces.SubmitRequest{
		WorkTypeID: "signal",
		WorkID:     "signal-for-cron-smoke",
		Name:       "Cron smoke signal",
		Payload:    []byte(`{"ready":true}`),
	})
	signalWorkID := submittedSignals[0].WorkID
	requiredInputDispatch := waitForRequiredInputCronDispatch(t, fs, "poll-with-input", signalWorkID, 2*time.Second)
	requiredInputTimeToken := consumedCronTimeToken(t, requiredInputDispatch, retryRecord.Request.WorkID)
	if requiredInputTimeToken.Color.WorkID == requiredInputRecord.Request.WorkID {
		t.Fatalf("cron dispatched with expired time token %q after expiry; dispatch=%#v", requiredInputRecord.Request.WorkID, requiredInputDispatch)
	}
	assertCronPayload(t, requiredInputTimeToken, "poll-with-input")

	requiredOutput := waitForTokenInPlaceByParent(t, fs, "task:init", signalWorkID, 2*time.Second)
	if requiredOutput.Color.WorkTypeID != "task" {
		t.Fatalf("required-input cron output work type = %q, want task", requiredOutput.Color.WorkTypeID)
	}
	assertRequiredInputCronHistory(t, fs, requiredInputDispatch.DispatchID, signalWorkID)
	assertCronTimeWorkHiddenFromNormalViews(t, fs, requiredInputTimeToken.Color.WorkID)
}

func TestCronWorkstations_ServiceModeExpiryConsumesStaleTriggerWithTerminalOutputAndDefaultWindow(t *testing.T) {
	start := time.Date(2026, time.April, 18, 13, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := support.ScaffoldFactory(t, cronDefaultExpiryTerminalOutputConfig("* * * * *"))

	observedSubmissions := make(chan interfaces.FactorySubmissionRecord, 32)
	fs := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Clock = fakeClock
	}, factory.WithSubmissionRecorder(func(record interfaces.FactorySubmissionRecord) {
		observedSubmissions <- record
	}))

	firstRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "poll-terminal-output", start, time.Second)
	firstToken := waitForCronToken(t, fs, "poll-terminal-output", firstRecord.Request.WorkID, time.Second)
	assertCronPayload(t, firstToken, "poll-terminal-output")
	assertCronDefaultExpiryWindow(t, firstToken, time.Minute)

	pendingWithoutInput := fs.GetEngineStateSnapshot(t)
	assertNoCronDispatchForWorkstation(t, pendingWithoutInput, "poll-terminal-output")
	assertNoTokensInPlace(t, pendingWithoutInput, "task:complete")

	waitForFakeClockWaiters(t, fakeClock, 1)
	retryFire := start.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	retryRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "poll-terminal-output", retryFire, time.Second)
	if retryRecord.Request.WorkID == firstRecord.Request.WorkID {
		t.Fatal("terminal-output cron retry reused the stale cron time work ID")
	}
	waitForCronTimeWorkGone(t, fs, firstRecord.Request.WorkID, time.Second)
	retryToken := waitForCronToken(t, fs, "poll-terminal-output", retryRecord.Request.WorkID, time.Second)
	if retryToken.Color.WorkID == "" {
		t.Fatal("expected retry cron time work ID after stale tick expiry")
	}

	afterExpiry := fs.GetEngineStateSnapshot(t)
	assertNoCronDispatchForWorkstation(t, afterExpiry, "poll-terminal-output")
	assertNoTokensInPlace(t, afterExpiry, "task:complete")
	assertCronTimeWorkRetainedInCanonicalHistory(t, fs, firstRecord.Request.WorkID, "poll-terminal-output")
}

func assertExpiredCronTimeWorkHandled(t *testing.T, fs *functionalAPIServer, expiredTimeWorkID string, workstation string) {
	t.Helper()

	snap := fs.GetEngineStateSnapshot(t)
	for _, token := range snap.Marking.TokensInPlace(interfaces.SystemTimePendingPlaceID) {
		if token.Color.WorkID == expiredTimeWorkID {
			t.Fatalf("expired cron time work %q still pending in system time place: %#v", expiredTimeWorkID, token)
		}
	}
	assertNoCustomerCronOutput(t, snap, workstation)
	assertCronTimeWorkRetainedInCanonicalHistory(t, fs, expiredTimeWorkID, workstation)
}

func cronSmokeFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "signal",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "startup-refresh",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       schedule,
					"triggerAtStart": true,
					"expiryWindow":   "10s",
				},
				"outputs": []map[string]string{{"workType": "task", "state": "init"}},
			},
			{
				"name":     "poll-for-work",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]any{"schedule": schedule, "expiryWindow": "10s"},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
			{
				"name":     "poll-with-input",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]any{"schedule": schedule, "expiryWindow": "10s"},
				"inputs":   []map[string]string{{"workType": "signal", "state": "init"}},
				"outputs":  []map[string]string{{"workType": "task", "state": "init"}},
			},
		},
	}
}

func cronDefaultExpiryTerminalOutputConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
			{
				"name": "signal",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{{"name": "cron-worker"}},
		"workstations": []map[string]any{
			{
				"name":     "poll-terminal-output",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron":     map[string]any{"schedule": schedule, "triggerAtStart": true},
				"inputs":   []map[string]string{{"workType": "signal", "state": "init"}},
				"outputs":  []map[string]string{{"workType": "task", "state": "complete"}},
			},
		},
	}
}

func waitForCronSubmissionFromWorkstation(
	t *testing.T,
	submissions <-chan interfaces.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	timeout time.Duration,
) interfaces.FactorySubmissionRecord {
	t.Helper()
	return waitForCronSubmissions(t, submissions, []string{workstation}, nominalAt, timeout)[workstation]
}

func waitForCronSubmissions(
	t *testing.T,
	submissions <-chan interfaces.FactorySubmissionRecord,
	workstations []string,
	nominalAt time.Time,
	timeout time.Duration,
) map[string]interfaces.FactorySubmissionRecord {
	t.Helper()

	want := make(map[string]bool, len(workstations))
	for _, workstation := range workstations {
		want[workstation] = true
	}
	found := make(map[string]interfaces.FactorySubmissionRecord, len(workstations))
	wantNominalAt := nominalAt.UTC().Format(time.RFC3339Nano)
	deadline := time.After(timeout)
	for len(found) < len(want) {
		select {
		case record := <-submissions:
			workstation := record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation]
			if !want[workstation] {
				continue
			}
			if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
				t.Fatalf("cron submission work type = %q, want %q", record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
			}
			if got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]; got != wantNominalAt {
				t.Fatalf("cron submission from %q nominal_at = %q, want %q", workstation, got, wantNominalAt)
			}
			found[workstation] = record
		case <-deadline:
			t.Fatalf("timed out waiting for cron submissions from %#v at %s; found=%#v", workstations, wantNominalAt, found)
		}
	}
	return found
}

func assertCronSubmissionRecord(t *testing.T, record interfaces.FactorySubmissionRecord, workstation string, nominalAt time.Time) {
	t.Helper()

	if record.Source != "external-submit" {
		t.Fatalf("%s cron submission source = %q, want external-submit", workstation, record.Source)
	}
	if record.Request.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("%s cron submission work type = %q, want %q", workstation, record.Request.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if record.Request.TargetState != interfaces.SystemTimePendingState {
		t.Fatalf("%s cron submission target state = %q, want %q", workstation, record.Request.TargetState, interfaces.SystemTimePendingState)
	}
	if record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != workstation {
		t.Fatalf("cron submission workstation tag = %q, want %q", record.Request.Tags[interfaces.TimeWorkTagKeyCronWorkstation], workstation)
	}
	if got := record.Request.Tags[interfaces.TimeWorkTagKeyNominalAt]; got != nominalAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("%s cron nominal_at tag = %q, want %q", workstation, got, nominalAt.UTC().Format(time.RFC3339Nano))
	}
}

func waitForCronToken(t *testing.T, fs *functionalAPIServer, workstation string, workID string, timeout time.Duration) interfaces.Token {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap := fs.GetEngineStateSnapshot(t)
		for _, token := range snap.Marking.TokensInPlace(interfaces.SystemTimePendingPlaceID) {
			if token.Color.WorkID == workID && token.Color.Tags[interfaces.TimeWorkTagKeyCronWorkstation] == workstation {
				return token
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for cron token from %q", workstation)
	return interfaces.Token{}
}

func waitForCronTimeWorkGone(t *testing.T, fs *functionalAPIServer, workID string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap := fs.GetEngineStateSnapshot(t)
		if _, ok := snap.Marking.Tokens[workID]; !ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	snap := fs.GetEngineStateSnapshot(t)
	t.Fatalf("timed out waiting for stale cron time work %q to expire; token=%#v", workID, snap.Marking.Tokens[workID])
}

func waitForCronDispatch(t *testing.T, fs *functionalAPIServer, workstation string, timeWorkID string, timeout time.Duration) interfaces.CompletedDispatch {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap := fs.GetEngineStateSnapshot(t)
		for _, dispatch := range snap.DispatchHistory {
			if dispatch.WorkstationName != workstation {
				continue
			}
			for _, token := range dispatch.ConsumedTokens {
				if token.Color.WorkID == timeWorkID && token.Color.WorkTypeID == interfaces.SystemTimeWorkTypeID {
					return dispatch
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for cron dispatch from %q consuming %q", workstation, timeWorkID)
	return interfaces.CompletedDispatch{}
}

func waitForRequiredInputCronDispatch(t *testing.T, fs *functionalAPIServer, workstation string, signalWorkID string, timeout time.Duration) interfaces.CompletedDispatch {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap := fs.GetEngineStateSnapshot(t)
		for _, dispatch := range snap.DispatchHistory {
			if dispatch.WorkstationName != workstation {
				continue
			}
			var consumedSignal bool
			var consumedTime bool
			for _, token := range dispatch.ConsumedTokens {
				if token.Color.WorkID == signalWorkID && token.Color.WorkTypeID == "signal" {
					consumedSignal = true
				}
				if token.Color.WorkTypeID == interfaces.SystemTimeWorkTypeID {
					consumedTime = true
				}
			}
			if consumedSignal && consumedTime {
				return dispatch
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	snap := fs.GetEngineStateSnapshot(t)
	t.Fatalf("timed out waiting for cron dispatch from %q consuming signal %q; dispatch history=%#v", workstation, signalWorkID, snap.DispatchHistory)
	return interfaces.CompletedDispatch{}
}

func consumedCronTimeToken(t *testing.T, dispatch interfaces.CompletedDispatch, workID string) interfaces.Token {
	t.Helper()
	for _, token := range dispatch.ConsumedTokens {
		if token.Color.WorkID == workID && token.Color.WorkTypeID == interfaces.SystemTimeWorkTypeID {
			return token
		}
	}
	t.Fatalf("dispatch %q did not consume cron time token %q: %#v", dispatch.DispatchID, workID, dispatch.ConsumedTokens)
	return interfaces.Token{}
}

func waitForTokenInPlaceByParent(t *testing.T, fs *functionalAPIServer, placeID string, parentID string, timeout time.Duration) interfaces.Token {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		snap := fs.GetEngineStateSnapshot(t)
		for _, token := range snap.Marking.TokensInPlace(placeID) {
			if token.Color.ParentID == parentID {
				return token
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for token in %s with parent %q", placeID, parentID)
	return interfaces.Token{}
}

func assertCronDefaultExpiryWindow(t *testing.T, token interfaces.Token, expected time.Duration) {
	t.Helper()

	dueAt := parseCronTimeTag(t, token, interfaces.TimeWorkTagKeyDueAt)
	expiresAt := parseCronTimeTag(t, token, interfaces.TimeWorkTagKeyExpiresAt)
	if got := expiresAt.Sub(dueAt); got != expected {
		t.Fatalf("cron default expiry window = %s, want %s", got, expected)
	}
}

func parseCronTimeTag(t *testing.T, token interfaces.Token, key string) time.Time {
	t.Helper()

	value := token.Color.Tags[key]
	if value == "" {
		t.Fatalf("cron token %q missing %s tag: %#v", token.ID, key, token.Color.Tags)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("cron token %q has invalid %s tag %q: %v", token.ID, key, value, err)
	}
	return parsed.UTC()
}

func assertNoCustomerCronOutput(t *testing.T, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workstation string) {
	t.Helper()

	for _, token := range snap.Marking.TokensInPlace("task:init") {
		if token.Color.Tags[interfaces.TimeWorkTagKeyCronWorkstation] == workstation {
			t.Fatalf("cron emitted customer work instead of internal time work: %#v", token)
		}
	}
}

func assertNoTokensInPlace(t *testing.T, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) {
	t.Helper()

	if tokens := snap.Marking.TokensInPlace(placeID); len(tokens) != 0 {
		t.Fatalf("expected no tokens in %s, got %#v", placeID, tokens)
	}
}

func assertNoCronDispatchForWorkstation(t *testing.T, snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], workstation string) {
	t.Helper()

	for _, dispatch := range snap.DispatchHistory {
		if dispatch.WorkstationName == workstation {
			t.Fatalf("cron workstation %q dispatched while required input was missing: %#v", workstation, dispatch)
		}
	}
}

func assertCronPayload(t *testing.T, token interfaces.Token, workstation string) {
	t.Helper()

	if token.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron token work type = %q, want %q", token.Color.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if token.Color.Tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("cron token source tag = %q, want %q", token.Color.Tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
	if token.Color.Name != "cron:"+workstation {
		t.Fatalf("cron token name = %q, want %q", token.Color.Name, "cron:"+workstation)
	}

	var payload map[string]string
	if err := json.Unmarshal(token.Color.Payload, &payload); err != nil {
		t.Fatalf("cron token payload is not JSON: %v\npayload=%s", err, token.Color.Payload)
	}
	if payload["cron_workstation"] != workstation {
		t.Fatalf("cron payload workstation = %q, want %s", payload["cron_workstation"], workstation)
	}
	for _, key := range []string{"nominal_at", "due_at", "expires_at", "jitter", "source"} {
		if payload[key] == "" {
			t.Fatalf("cron payload missing %s: %#v", key, payload)
		}
	}
}

func assertCronTimeWorkHiddenFromNormalViews(t *testing.T, fs *functionalAPIServer, timeWorkID string) {
	t.Helper()

	assertStatusHidesCronTimeWork(t, fs, timeWorkID)

	work := fs.ListWork(t)
	for _, token := range work.Results {
		if token.WorkId == timeWorkID || token.WorkType == interfaces.SystemTimeWorkTypeID {
			t.Fatalf("GET /work exposed internal cron time work %q: %#v", timeWorkID, token)
		}
	}

}

func assertStatusHidesCronTimeWork(t *testing.T, fs *functionalAPIServer, timeWorkID string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	var lastMismatch string
	for time.Now().Before(deadline) {
		snap := fs.GetEngineStateSnapshot(t)
		status := getGeneratedJSON[factoryapi.StatusResponse](t, fs.URL()+"/status")
		publicTokens := countPublicCronSmokeTokens(snap)
		if status.TotalTokens == publicTokens {
			return
		}
		if _, ok := snap.Marking.Tokens[timeWorkID]; !ok {
			return
		}
		lastMismatch = fmt.Sprintf("GET /status total_tokens = %d, want public token count %d while internal cron time work %q is pending", status.TotalTokens, publicTokens, timeWorkID)
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal(lastMismatch)
}

func countPublicCronSmokeTokens(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]) int {
	count := 0
	for _, token := range snap.Marking.Tokens {
		if token == nil || interfaces.IsSystemTimeToken(token) {
			continue
		}
		count++
	}
	return count
}

func assertCronTimeWorkRetainedInCanonicalHistory(t *testing.T, fs *functionalAPIServer, timeWorkID string, workstation string) {
	t.Helper()

	events, err := fs.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil || payload.Works == nil {
			continue
		}
		for _, work := range *payload.Works {
			if stringPointerValue(work.WorkId) != timeWorkID {
				continue
			}
			assertCronHistoryTags(t, generatedFactoryEventTags(work.Tags), workstation)
			return
		}
	}
	t.Fatalf("canonical history missing WORK_REQUEST for cron time work %q", timeWorkID)
}

func assertRequiredInputCronHistory(t *testing.T, fs *functionalAPIServer, dispatchID string, signalWorkID string) {
	t.Helper()

	events, err := fs.service.GetFactoryEvents(context.Background())
	if err != nil {
		t.Fatalf("GetFactoryEvents: %v", err)
	}
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil || stringPointerValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		var sawSignal bool
		var sawTime bool
		for _, input := range dispatchInputWorksFromHistory(t, events, event, payload) {
			if stringPointerValue(input.WorkId) == signalWorkID && stringPointerValue(input.WorkTypeName) == "signal" {
				sawSignal = true
			}
			if stringPointerValue(input.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
				sawTime = true
				assertCronHistoryTags(t, generatedFactoryEventTags(input.Tags), "poll-with-input")
			}
		}
		if !sawSignal || !sawTime {
			t.Fatalf("cron dispatch history inputs sawSignal=%v sawTime=%v payload=%#v", sawSignal, sawTime, payload)
		}
		return
	}
	t.Fatalf("canonical history missing WORKSTATION_REQUEST for cron dispatch %q", dispatchID)
}

func assertCronHistoryTags(t *testing.T, tags map[string]string, workstation string) {
	t.Helper()

	if tags[interfaces.TimeWorkTagKeyCronWorkstation] != workstation {
		t.Fatalf("cron history workstation tag = %q, want %q; tags=%#v", tags[interfaces.TimeWorkTagKeyCronWorkstation], workstation, tags)
	}
	for _, key := range []string{
		interfaces.TimeWorkTagKeyNominalAt,
		interfaces.TimeWorkTagKeyDueAt,
		interfaces.TimeWorkTagKeyExpiresAt,
		interfaces.TimeWorkTagKeyJitter,
		interfaces.TimeWorkTagKeySource,
	} {
		if tags[key] == "" {
			t.Fatalf("cron history missing %s tag: %#v", key, tags)
		}
	}
}

func generatedFactoryEventTags(tags *factoryapi.StringMap) map[string]string {
	if tags == nil {
		return nil
	}
	return map[string]string(*tags)
}

func dispatchInputWorksFromHistory(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	event factoryapi.FactoryEvent,
	payload factoryapi.DispatchRequestEventPayload,
) []factoryapi.Work {
	t.Helper()

	workByID := workRequestWorksByID(t, events)
	ordered := make([]factoryapi.Work, 0, len(payload.Inputs))
	for _, workID := range dispatchInputWorkIDsForTests(payload, event.Context) {
		if work, ok := workByID[workID]; ok {
			ordered = append(ordered, work)
		}
	}
	return ordered
}

func workRequestWorksByID(t *testing.T, events []factoryapi.FactoryEvent) map[string]factoryapi.Work {
	t.Helper()

	workByID := make(map[string]factoryapi.Work)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode WORK_REQUEST payload %q: %v", event.Id, err)
		}
		for _, work := range workSliceForTests(payload.Works) {
			if workID := eventString(work.WorkId); workID != "" {
				workByID[workID] = work
			}
		}
	}
	return workByID
}

func dispatchInputWorkIDsForTests(
	payload factoryapi.DispatchRequestEventPayload,
	context factoryapi.FactoryEventContext,
) []string {
	ordered := make([]string, 0, len(payload.Inputs)+len(eventStringSlice(context.WorkIds)))
	for _, input := range payload.Inputs {
		ordered = appendUniqueDispatchWorkID(ordered, input.WorkId)
	}
	for _, workID := range eventStringSlice(context.WorkIds) {
		ordered = appendUniqueDispatchWorkID(ordered, workID)
	}
	return ordered
}

func appendUniqueDispatchWorkID(values []string, value string) []string {
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func workSliceForTests(works *[]factoryapi.Work) []factoryapi.Work {
	if works == nil {
		return nil
	}
	return *works
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func eventString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func eventStringSlice(values *[]string) []string {
	if values == nil {
		return nil
	}
	return *values
}
