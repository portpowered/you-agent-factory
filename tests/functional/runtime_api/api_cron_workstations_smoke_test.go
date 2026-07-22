package runtime_api

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// portos:func-length-exception owner=agent-factory reason=cron-end-to-end-smoke review=2026-07-18 removal=split-smoke-helpers-before-next-cron-e2e-expansion
func TestCronWorkstations_ServiceModeSmoke_SubmitsInternalTimeWorkExpiresRetriesDispatchesAndFiltersViews(t *testing.T) {
	start := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := support.ScaffoldFactory(t, cronSmokeFactoryConfig("* * * * *"))

	observedSubmissions := make(chan work.FactorySubmissionRecord, 32)
	fs := startFunctionalServerWithArgs(t, dir, true, nil, withSubmissionRecorder(func(record work.FactorySubmissionRecord) {
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
	if stringPointerValue(noInputWork.WorkId) == "" {
		t.Fatal("no-input cron Work missing Work ID")
	}
	if stringPointerValue(noInputWork.TraceId) == "" {
		t.Fatal("no-input cron Work missing trace ID")
	}
	assertCronPublicMetadata(t, noInputWork, "poll-for-work")

	state := getGeneratedJSON[factoryapi.StatusResponse](t, fs.URL()+"/status")
	if state.RuntimeStatus == "" {
		t.Fatal("GET /state returned empty runtime_status after cron output")
	}
	if state.TotalTokens == 0 {
		t.Fatal("GET /state returned zero tokens after cron output")
	}
	noInputOutput := waitForTokenInPlaceByParent(t, fs, "task:init", noInputRecord.Request.WorkID, time.Second)
	if got := stringPointerValue(noInputOutput.Work.WorkTypeName); got != "task" {
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

	submittedSignals := fs.SubmitRuntimeWork(t, work.SubmitRequest{
		WorkTypeID: "signal",
		WorkID:     "signal-for-cron-smoke",
		Name:       "Cron smoke signal",
		Payload:    []byte(`{"ready":true}`),
	})
	signalWorkID := submittedSignals[0].WorkID
	requiredInputDispatch := waitForRequiredInputCronDispatch(t, fs, "poll-with-input", signalWorkID, 2*time.Second)
	requiredInputTimeWork := consumedCronTimeWork(t, requiredInputDispatch, retryRecord.Request.WorkID)
	if stringPointerValue(requiredInputTimeWork.WorkId) == requiredInputRecord.Request.WorkID {
		t.Fatalf("cron dispatched with expired time token %q after expiry; dispatch=%#v", requiredInputRecord.Request.WorkID, requiredInputDispatch)
	}
	assertCronPublicMetadata(t, requiredInputTimeWork, "poll-with-input")

	requiredOutput := waitForTokenInPlaceByParent(t, fs, "task:init", signalWorkID, 2*time.Second)
	if got := stringPointerValue(requiredOutput.Work.WorkTypeName); got != "task" {
		t.Fatalf("required-input cron output work type = %q, want task", got)
	}
	assertRequiredInputCronHistory(t, fs, requiredInputDispatch.DispatchID, signalWorkID)
	assertCronTimeWorkHiddenFromNormalViews(t, fs, stringPointerValue(requiredInputTimeWork.WorkId))
}

func TestCronWorkstations_ServiceModeExpiryConsumesStaleTriggerWithTerminalOutputAndDefaultWindow(t *testing.T) {
	start := time.Date(2026, time.April, 18, 13, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := support.ScaffoldFactory(t, cronDefaultExpiryTerminalOutputConfig("* * * * *"))

	observedSubmissions := make(chan work.FactorySubmissionRecord, 32)
	fs := startFunctionalServerWithArgs(t, dir, true, nil, withSubmissionRecorder(func(record work.FactorySubmissionRecord) {
		observedSubmissions <- record
	}), withClock(fakeClock))

	firstRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "poll-terminal-output", start, time.Second)
	assertCronSubmissionRecord(t, firstRecord, "poll-terminal-output", start)
	firstToken := waitForCronToken(t, fs, "poll-terminal-output", firstRecord.Request.WorkID, time.Second)
	assertCronPublicMetadata(t, firstToken, "poll-terminal-output")
	assertCronDefaultExpiryWindow(t, firstToken, time.Minute)

	assertNoCronDispatchForWorkstation(t, fs, "poll-terminal-output")
	assertNoTokensInPlace(t, fs, "task:complete")

	waitForFakeClockWaiters(t, fakeClock, 1)
	retryFire := start.Add(time.Minute)
	fakeClock.Advance(time.Minute)
	retryRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "poll-terminal-output", retryFire, time.Second)
	if retryRecord.Request.WorkID == firstRecord.Request.WorkID {
		t.Fatal("terminal-output cron retry reused the stale cron time work ID")
	}
	waitForCronTimeWorkGone(t, fs, firstRecord.Request.WorkID, time.Second)
	retryToken := waitForCronToken(t, fs, "poll-terminal-output", retryRecord.Request.WorkID, time.Second)
	if stringPointerValue(retryToken.WorkId) == "" {
		t.Fatal("expected retry cron time work ID after stale tick expiry")
	}

	assertNoCronDispatchForWorkstation(t, fs, "poll-terminal-output")
	assertNoTokensInPlace(t, fs, "task:complete")
	assertCronTimeWorkRetainedInCanonicalHistory(t, fs, firstRecord.Request.WorkID, "poll-terminal-output")
}

func TestCronWorkstations_ServiceModeImplicitFailureRoutingMovesFailedCronWorkIntoFailedState(t *testing.T) {
	start := time.Date(2026, time.April, 18, 14, 30, 0, 0, time.UTC)
	fakeClock := clockwork.NewFakeClockAt(start)
	dir := support.ScaffoldFactory(t, cronImplicitFailureFactoryConfig("* * * * *"))
	support.WriteAgentConfig(t, dir, "cron-worker", `---
type: MODEL_WORKER
executorProvider: codex-cli
modelProvider: openai
model: gpt-5.4
stopToken: COMPLETE
---
Fail the cron task.
`)

	observedSubmissions := make(chan work.FactorySubmissionRecord, 32)
	fs := startFunctionalServerWithArgs(t, dir, false, nil, withSubmissionRecorder(func(record work.FactorySubmissionRecord) {
		observedSubmissions <- record
	}), withClock(fakeClock), withWorkerCommands(testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte("cron worker unavailable"),
		ExitCode: 1,
	}), nil))

	startupRecord := waitForCronSubmissionFromWorkstation(t, observedSubmissions, "fail-cron", start, time.Second)
	assertCronSubmissionRecord(t, startupRecord, "fail-cron", start)
	waitForCronDispatch(t, fs, "fail-cron", startupRecord.Request.WorkID, time.Second)

	failedToken := waitForTokenInPlaceByParent(t, fs, "task:failed", startupRecord.Request.WorkID, time.Second)
	if got := stringPointerValue(failedToken.Work.WorkTypeName); got != "task" {
		t.Fatalf("failed cron output work type = %q, want task", got)
	}
	if failedToken.LastError == "" {
		t.Fatalf("failed cron Work observation = %#v, want last error evidence", failedToken)
	}
}

func assertExpiredCronTimeWorkHandled(t *testing.T, fs *functionalAPIServer, expiredTimeWorkID string, workstation string) {
	t.Helper()

	assertNoCustomerCronOutput(t, fs, workstation)
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

func cronImplicitFailureFactoryConfig(schedule string) map[string]any {
	return map[string]any{
		"name": "factory",
		"workTypes": []map[string]any{
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
				"name":     "fail-cron",
				"behavior": "CRON",
				"worker":   "cron-worker",
				"cron": map[string]any{
					"schedule":       schedule,
					"triggerAtStart": true,
					"expiryWindow":   "10s",
				},
				"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
			},
		},
	}
}

func waitForCronSubmissionFromWorkstation(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workstation string,
	nominalAt time.Time,
	timeout time.Duration,
) work.FactorySubmissionRecord {
	t.Helper()
	return waitForCronSubmissions(t, submissions, []string{workstation}, nominalAt, timeout)[workstation]
}

func waitForCronSubmissions(
	t *testing.T,
	submissions <-chan work.FactorySubmissionRecord,
	workstations []string,
	nominalAt time.Time,
	timeout time.Duration,
) map[string]work.FactorySubmissionRecord {
	t.Helper()

	want := make(map[string]bool, len(workstations))
	for _, workstation := range workstations {
		want[workstation] = true
	}
	found := make(map[string]work.FactorySubmissionRecord, len(workstations))
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

func assertCronSubmissionRecord(t *testing.T, record work.FactorySubmissionRecord, workstation string, nominalAt time.Time) {
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
	var payload map[string]string
	if err := json.Unmarshal(record.Request.Payload, &payload); err != nil {
		t.Fatalf("%s cron submission payload is not JSON: %v\npayload=%s", workstation, err, record.Request.Payload)
	}
	if payload["cron_workstation"] != workstation {
		t.Fatalf("cron submission payload workstation = %q, want %s", payload["cron_workstation"], workstation)
	}
	for _, key := range []string{"nominal_at", "due_at", "expires_at", "jitter", "source"} {
		if payload[key] == "" {
			t.Fatalf("cron submission payload missing %s: %#v", key, payload)
		}
	}
}

func waitForCronToken(
	t *testing.T,
	fs *functionalAPIServer,
	workstation string,
	workID string,
	timeout time.Duration,
) factoryapi.Work {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, event := range fs.GetFactoryEvents(t) {
			if event.Type != factoryapi.FactoryEventTypeWorkRequest {
				continue
			}
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil || payload.Works == nil {
				continue
			}
			for _, item := range *payload.Works {
				if stringPointerValue(item.WorkId) == workID &&
					generatedFactoryEventTags(item.Tags)[interfaces.TimeWorkTagKeyCronWorkstation] == workstation {
					return item
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for cron token from %q", workstation)
	return factoryapi.Work{}
}

func waitForCronTimeWorkGone(t *testing.T, fs *functionalAPIServer, workID string, timeout time.Duration) {
	t.Helper()

	// Internal time work is intentionally absent from the public Work read
	// model. Its expiry is observable by the next nominal submission and by
	// the absence of a dispatch consuming the stale identifier.
	_ = timeout
	for _, event := range fs.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		for _, input := range support.DispatchInputWorksFromHistory(t, fs.GetFactoryEvents(t), event, payload) {
			if stringPointerValue(input.WorkId) == workID {
				t.Fatalf("expired cron time work %q was dispatched", workID)
			}
		}
	}
}

type cronDispatchObservation struct {
	DispatchID string
	Inputs     []factoryapi.Work
}

type publicWorkObservation struct {
	Work      factoryapi.Work
	LastError string
}

func waitForCronDispatch(
	t *testing.T,
	fs *functionalAPIServer,
	workstation string,
	timeWorkID string,
	timeout time.Duration,
) cronDispatchObservation {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := fs.GetFactoryEvents(t)
		for _, event := range events {
			if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
				continue
			}
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != workstation {
				continue
			}
			inputs := support.DispatchInputWorksFromHistory(t, events, event, payload)
			for _, item := range inputs {
				if stringPointerValue(item.WorkId) == timeWorkID &&
					stringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
					return cronDispatchObservation{
						DispatchID: stringPointerValue(event.Context.DispatchId),
						Inputs:     inputs,
					}
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for cron dispatch from %q consuming %q", workstation, timeWorkID)
	return cronDispatchObservation{}
}

func waitForRequiredInputCronDispatch(
	t *testing.T,
	fs *functionalAPIServer,
	workstation string,
	signalWorkID string,
	timeout time.Duration,
) cronDispatchObservation {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := fs.GetFactoryEvents(t)
		for _, event := range events {
			if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
				continue
			}
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil || payload.TransitionId != workstation {
				continue
			}
			var consumedSignal bool
			var consumedTime bool
			inputs := support.DispatchInputWorksFromHistory(t, events, event, payload)
			for _, item := range inputs {
				if stringPointerValue(item.WorkId) == signalWorkID &&
					stringPointerValue(item.WorkTypeName) == "signal" {
					consumedSignal = true
				}
				if stringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
					consumedTime = true
				}
			}
			if consumedSignal && consumedTime {
				return cronDispatchObservation{
					DispatchID: stringPointerValue(event.Context.DispatchId),
					Inputs:     inputs,
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for cron dispatch from %q consuming signal %q", workstation, signalWorkID)
	return cronDispatchObservation{}
}

func consumedCronTimeWork(
	t *testing.T,
	dispatch cronDispatchObservation,
	workID string,
) factoryapi.Work {
	t.Helper()
	for _, item := range dispatch.Inputs {
		if stringPointerValue(item.WorkId) == workID &&
			stringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
			return item
		}
	}
	t.Fatalf("dispatch %q did not consume cron time Work %q: %#v", dispatch.DispatchID, workID, dispatch.Inputs)
	return factoryapi.Work{}
}

func publicDispatchConsumedWork(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvent factoryapi.FactoryEvent,
	workID string,
) bool {
	t.Helper()

	dispatchID := stringPointerValue(responseEvent.Context.DispatchId)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest ||
			stringPointerValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		for _, item := range support.DispatchInputWorksFromHistory(t, events, event, payload) {
			if stringPointerValue(item.WorkId) == workID {
				return true
			}
		}
	}
	return false
}

func waitForTokenInPlaceByParent(
	t *testing.T,
	fs *functionalAPIServer,
	placeID string,
	parentID string,
	timeout time.Duration,
) publicWorkObservation {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		events := fs.GetFactoryEvents(t)
		for _, responseEvent := range events {
			if responseEvent.Type != factoryapi.FactoryEventTypeDispatchResponse {
				continue
			}
			response, err := responseEvent.Payload.AsDispatchResponseEventPayload()
			if err != nil || response.OutputWork == nil {
				continue
			}
			if !publicDispatchConsumedWork(t, events, responseEvent, parentID) {
				continue
			}
			for _, item := range *response.OutputWork {
				itemPlace := stringPointerValue(item.WorkTypeName) + ":" + generatedWorkStateName(item.State)
				if itemPlace == placeID {
					observation := publicWorkObservation{Work: item}
					if response.Error != nil {
						observation.LastError = *response.Error
					}
					return observation
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for token in %s with parent %q", placeID, parentID)
	return publicWorkObservation{}
}

func assertCronDefaultExpiryWindow(t *testing.T, item factoryapi.Work, expected time.Duration) {
	t.Helper()

	dueAt := parseCronTimeTag(t, item, interfaces.TimeWorkTagKeyDueAt)
	expiresAt := parseCronTimeTag(t, item, interfaces.TimeWorkTagKeyExpiresAt)
	if got := expiresAt.Sub(dueAt); got != expected {
		t.Fatalf("cron default expiry window = %s, want %s", got, expected)
	}
}

func parseCronTimeTag(t *testing.T, item factoryapi.Work, key string) time.Time {
	t.Helper()

	tags := generatedFactoryEventTags(item.Tags)
	value := tags[key]
	if value == "" {
		t.Fatalf("cron Work %q missing %s tag: %#v", stringPointerValue(item.WorkId), key, tags)
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("cron Work %q has invalid %s tag %q: %v", stringPointerValue(item.WorkId), key, value, err)
	}
	return parsed.UTC()
}

func assertNoCustomerCronOutput(t *testing.T, fs *functionalAPIServer, workstation string) {
	t.Helper()

	for _, item := range fs.ListWork(t).Results {
		if generatedWorkStateName(item.State) == "init" &&
			generatedFactoryEventTags(item.Tags)[interfaces.TimeWorkTagKeyCronWorkstation] == workstation {
			t.Fatalf("cron emitted customer work instead of internal time work: %#v", item)
		}
	}
}

func assertNoTokensInPlace(t *testing.T, fs *functionalAPIServer, placeID string) {
	t.Helper()

	for _, item := range fs.ListWork(t).Results {
		itemPlace := stringPointerValue(item.WorkTypeName) + ":" + generatedWorkStateName(item.State)
		if itemPlace == placeID {
			t.Fatalf("expected no public Work in %s, got %#v", placeID, item)
		}
	}
}

func assertNoCronDispatchForWorkstation(t *testing.T, fs *functionalAPIServer, workstation string) {
	t.Helper()

	for _, event := range fs.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err == nil && payload.TransitionId == workstation {
			t.Fatalf("cron workstation %q dispatched while required input was missing: %#v", workstation, event)
		}
	}
}

func assertCronPublicMetadata(t *testing.T, item factoryapi.Work, workstation string) {
	t.Helper()

	if got := stringPointerValue(item.WorkTypeName); got != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron Work type = %q, want %q", got, interfaces.SystemTimeWorkTypeID)
	}
	tags := generatedFactoryEventTags(item.Tags)
	if tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("cron Work source tag = %q, want %q", tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
	if item.Name != "cron:"+workstation {
		t.Fatalf("cron Work name = %q, want %q", item.Name, "cron:"+workstation)
	}

}

func assertCronTimeWorkHiddenFromNormalViews(t *testing.T, fs *functionalAPIServer, timeWorkID string) {
	t.Helper()

	assertStatusHidesCronTimeWork(t, fs, timeWorkID)

	work := fs.ListWork(t)
	for _, item := range work.Results {
		if stringPointerValue(item.WorkId) == timeWorkID || stringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
			t.Fatalf("GET /work exposed internal cron time work %q: %#v", timeWorkID, item)
		}
	}

}

func assertStatusHidesCronTimeWork(t *testing.T, fs *functionalAPIServer, timeWorkID string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	var lastMismatch string
	for time.Now().Before(deadline) {
		status := getGeneratedJSON[factoryapi.StatusResponse](t, fs.URL()+"/status")
		publicTokens := len(fs.ListWork(t).Results)
		if status.TotalTokens == publicTokens {
			return
		}
		lastMismatch = fmt.Sprintf("GET /status total_tokens = %d, want public token count %d while internal cron time work %q is pending", status.TotalTokens, publicTokens, timeWorkID)
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal(lastMismatch)
}

func assertCronTimeWorkRetainedInCanonicalHistory(t *testing.T, fs *functionalAPIServer, timeWorkID string, workstation string) {
	t.Helper()

	events := fs.GetFactoryEvents(t)
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

	events := fs.GetFactoryEvents(t)
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
		for _, input := range support.DispatchInputWorksFromHistory(t, events, event, payload) {
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

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
