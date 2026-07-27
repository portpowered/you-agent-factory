package cron

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type cronServer struct {
	*support.FunctionalAPIServer
}

type cronRuntimeOption func(*support.FunctionalAPIServerConfig)

func withSubmissionRecorder(recorder recordings.SubmissionRecorder) cronRuntimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) { cfg.Edges.SubmissionRecorder = recorder }
}

func withClock(clock platformclock.Source) cronRuntimeOption {
	return func(cfg *support.FunctionalAPIServerConfig) { cfg.Edges.Clock = clock }
}

func startCronServer(t *testing.T, factoryDir string, options ...cronRuntimeOption) *cronServer {
	t.Helper()

	cfg := support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	}
	for _, option := range options {
		option(&cfg)
	}
	return &cronServer{FunctionalAPIServer: support.StartFunctionalAPIServer(t, cfg)}
}

func (fs *cronServer) listWork(t *testing.T) factoryapi.ListWorkResponse {
	t.Helper()
	return support.ListDefaultSessionWork(t, fs.URL())
}

func (fs *cronServer) submitSignalWork(t *testing.T, workID, name string, payload []byte) []work.SubmitRequest {
	t.Helper()

	response := support.SubmitDefaultSessionWork(t, fs.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "signal",
		Payload:      payload,
	})
	request := work.SubmitRequest{
		WorkTypeID: "signal",
		WorkID:     workID,
		Name:       name,
		Payload:    payload,
	}
	if response.WorkId != nil {
		request.WorkID = *response.WorkId
	}
	request.RequestID = response.RequestId
	request.TraceID = response.TraceId
	request.CurrentChainingTraceID = response.TraceId
	return []work.SubmitRequest{request}
}

func waitForFakeClockWaiters(t *testing.T, fakeClock *clockwork.FakeClock, waiters int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := fakeClock.BlockUntilContext(ctx, waiters); err != nil {
		t.Fatalf("timed out waiting for %d fake-clock waiter(s): %v", waiters, err)
	}
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

type cronDispatchObservation struct {
	DispatchID string
	Inputs     []factoryapi.Work
}

type publicWorkObservation struct {
	Work      factoryapi.Work
	LastError string
}

func waitForCronToken(
	t *testing.T,
	fs *cronServer,
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
				if support.StringPointerValue(item.WorkId) == workID &&
					cronFactoryEventTags(item.Tags)[interfaces.TimeWorkTagKeyCronWorkstation] == workstation {
					return item
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for cron token from %q", workstation)
	return factoryapi.Work{}
}

func waitForCronTimeWorkGone(t *testing.T, fs *cronServer, workID string, timeout time.Duration) {
	t.Helper()

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
			if support.StringPointerValue(input.WorkId) == workID {
				t.Fatalf("expired cron time work %q was dispatched", workID)
			}
		}
	}
}

func waitForCronDispatch(
	t *testing.T,
	fs *cronServer,
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
				if support.StringPointerValue(item.WorkId) == timeWorkID &&
					support.StringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
					return cronDispatchObservation{
						DispatchID: support.StringPointerValue(event.Context.DispatchId),
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
	fs *cronServer,
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
				if support.StringPointerValue(item.WorkId) == signalWorkID &&
					support.StringPointerValue(item.WorkTypeName) == "signal" {
					consumedSignal = true
				}
				if support.StringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
					consumedTime = true
				}
			}
			if consumedSignal && consumedTime {
				return cronDispatchObservation{
					DispatchID: support.StringPointerValue(event.Context.DispatchId),
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
		if support.StringPointerValue(item.WorkId) == workID &&
			support.StringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
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

	dispatchID := support.StringPointerValue(responseEvent.Context.DispatchId)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest ||
			support.StringPointerValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		for _, item := range support.DispatchInputWorksFromHistory(t, events, event, payload) {
			if support.StringPointerValue(item.WorkId) == workID {
				return true
			}
		}
	}
	return false
}

func waitForTokenInPlaceByParent(
	t *testing.T,
	fs *cronServer,
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
				itemPlace := support.StringPointerValue(item.WorkTypeName) + ":" + generatedWorkStateName(item.State)
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

func assertExpiredCronTimeWorkHandled(t *testing.T, fs *cronServer, expiredTimeWorkID string, workstation string) {
	t.Helper()

	assertNoCustomerCronOutput(t, fs, workstation)
	assertCronTimeWorkRetainedInCanonicalHistory(t, fs, expiredTimeWorkID, workstation)
}

func assertNoCustomerCronOutput(t *testing.T, fs *cronServer, workstation string) {
	t.Helper()

	for _, item := range fs.listWork(t).Results {
		if generatedWorkStateName(item.State) == "init" &&
			cronFactoryEventTags(item.Tags)[interfaces.TimeWorkTagKeyCronWorkstation] == workstation {
			t.Fatalf("cron emitted customer work instead of internal time work: %#v", item)
		}
	}
}

func assertNoCronDispatchForWorkstation(t *testing.T, fs *cronServer, workstation string) {
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

	if got := support.StringPointerValue(item.WorkTypeName); got != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("cron Work type = %q, want %q", got, interfaces.SystemTimeWorkTypeID)
	}
	tags := cronFactoryEventTags(item.Tags)
	if tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("cron Work source tag = %q, want %q", tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
	if item.Name != "cron:"+workstation {
		t.Fatalf("cron Work name = %q, want %q", item.Name, "cron:"+workstation)
	}
}

func assertCronTimeWorkHiddenFromNormalViews(t *testing.T, fs *cronServer, timeWorkID string) {
	t.Helper()

	assertStatusHidesCronTimeWork(t, fs, timeWorkID)

	work := fs.listWork(t)
	for _, item := range work.Results {
		if support.StringPointerValue(item.WorkId) == timeWorkID || support.StringPointerValue(item.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
			t.Fatalf("GET /work exposed internal cron time work %q: %#v", timeWorkID, item)
		}
	}
}

func assertStatusHidesCronTimeWork(t *testing.T, fs *cronServer, timeWorkID string) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	var lastMismatch string
	for time.Now().Before(deadline) {
		status := support.GetJSON[factoryapi.StatusResponse](t, fs.URL()+"/status")
		publicTokens := len(fs.listWork(t).Results)
		if status.TotalTokens == publicTokens {
			return
		}
		lastMismatch = fmt.Sprintf("GET /status total_tokens = %d, want public token count %d while internal cron time work %q is pending", status.TotalTokens, publicTokens, timeWorkID)
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal(lastMismatch)
}

func assertCronTimeWorkRetainedInCanonicalHistory(t *testing.T, fs *cronServer, timeWorkID string, workstation string) {
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
		for _, workItem := range *payload.Works {
			if support.StringPointerValue(workItem.WorkId) != timeWorkID {
				continue
			}
			assertCronHistoryTags(t, cronFactoryEventTags(workItem.Tags), workstation)
			return
		}
	}
	t.Fatalf("canonical history missing WORK_REQUEST for cron time work %q", timeWorkID)
}

func assertRequiredInputCronHistory(t *testing.T, fs *cronServer, dispatchID string, signalWorkID string) {
	t.Helper()

	events := fs.GetFactoryEvents(t)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil || support.StringPointerValue(event.Context.DispatchId) != dispatchID {
			continue
		}
		var sawSignal bool
		var sawTime bool
		for _, input := range support.DispatchInputWorksFromHistory(t, events, event, payload) {
			if support.StringPointerValue(input.WorkId) == signalWorkID && support.StringPointerValue(input.WorkTypeName) == "signal" {
				sawSignal = true
			}
			if support.StringPointerValue(input.WorkTypeName) == interfaces.SystemTimeWorkTypeID {
				sawTime = true
				assertCronHistoryTags(t, cronFactoryEventTags(input.Tags), "poll-with-input")
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

func cronFactoryEventTags(tags *factoryapi.StringMap) map[string]string {
	if tags == nil {
		return nil
	}
	return map[string]string(*tags)
}

func generatedWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}
