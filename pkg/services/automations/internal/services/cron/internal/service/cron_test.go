package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func testCronService() cron.Service {
	return New()
}

func TestBuildCronTimeMetadata_DeterministicForSameWorkflowWorkstationAndNominalTime(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	input := cron.CronTimeInput{
		WorkflowIdentity: "factory/main",
		WorkstationName:  "daily-refresh",
		NominalAt:        nominalAt,
		MaxJitter:        5 * time.Second,
		ExpiryWindow:     time.Minute,
	}

	first, err := svc.BuildCronTimeMetadata(input)
	if err != nil {
		t.Fatalf("BuildCronTimeMetadata first: %v", err)
	}
	second, err := svc.BuildCronTimeMetadata(input)
	if err != nil {
		t.Fatalf("BuildCronTimeMetadata second: %v", err)
	}

	if first != second {
		t.Fatalf("metadata is not deterministic:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if first.Jitter < 0 || first.Jitter > input.MaxJitter {
		t.Fatalf("jitter = %s, want within [0,%s]", first.Jitter, input.MaxJitter)
	}
	if !first.DueAt.Equal(nominalAt.Add(first.Jitter)) {
		t.Fatalf("due_at = %s, want nominal_at + jitter %s", first.DueAt, nominalAt.Add(first.Jitter))
	}
	if !first.ExpiresAt.Equal(first.DueAt.Add(input.ExpiryWindow)) {
		t.Fatalf("expires_at = %s, want due_at + expiry window %s", first.ExpiresAt, first.DueAt.Add(input.ExpiryWindow))
	}
}

func TestParseCronTiming_DefaultsJitterAndExpiryWindowFromSchedule(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	timing, err := svc.ParseCronTiming(&interfaces.CronConfig{Schedule: "*/5 * * * *"}, nominalAt)
	if err != nil {
		t.Fatalf("ParseCronTiming: %v", err)
	}
	if timing.MaxJitter != 0 {
		t.Fatalf("max jitter = %s, want 0", timing.MaxJitter)
	}
	if timing.ExpiryWindow != 5*time.Minute {
		t.Fatalf("expiry window = %s, want default schedule window 5m", timing.ExpiryWindow)
	}
}

func TestParseCronTiming_InvalidScheduleIncludesValue(t *testing.T) {
	svc := testCronService()
	_, err := svc.ParseCronTiming(&interfaces.CronConfig{Schedule: "not a cron"}, time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected invalid schedule error")
	}
	if !strings.Contains(err.Error(), `"not a cron"`) {
		t.Fatalf("expected error to include bad schedule value, got %v", err)
	}
}

func TestEvaluateCronSchedule_DueOnlyAtOrAfterBoundary(t *testing.T) {
	svc := testCronService()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		evaluatedAt time.Time
		wantDue     bool
	}{
		{name: "before boundary", evaluatedAt: lastEvaluatedAt.Add(4*time.Minute + 59*time.Second), wantDue: false},
		{name: "at boundary", evaluatedAt: lastEvaluatedAt.Add(5 * time.Minute), wantDue: true},
		{name: "after boundary", evaluatedAt: lastEvaluatedAt.Add(7 * time.Minute), wantDue: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := svc.EvaluateCronSchedule("*/5 * * * *", lastEvaluatedAt, test.evaluatedAt)
			if err != nil {
				t.Fatalf("EvaluateCronSchedule: %v", err)
			}
			if got.Due != test.wantDue {
				t.Fatalf("due = %t, want %t", got.Due, test.wantDue)
			}
			wantNominalAt := lastEvaluatedAt.Add(5 * time.Minute)
			if !got.NominalAt.Equal(wantNominalAt) {
				t.Fatalf("nominal at = %s, want %s", got.NominalAt, wantNominalAt)
			}
		})
	}
}

func TestEvaluateCronSchedule_NormalizesExplicitInstantsToUTC(t *testing.T) {
	svc := testCronService()
	losAngeles, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	lastEvaluatedAt := time.Date(2026, time.July, 13, 8, 59, 30, 0, losAngeles)
	evaluatedAt := time.Date(2026, time.July, 13, 16, 0, 0, 0, time.UTC)

	first, err := svc.EvaluateCronSchedule("0 * * * *", lastEvaluatedAt, evaluatedAt)
	if err != nil {
		t.Fatalf("EvaluateCronSchedule first: %v", err)
	}
	second, err := svc.EvaluateCronSchedule("0 * * * *", lastEvaluatedAt, evaluatedAt)
	if err != nil {
		t.Fatalf("EvaluateCronSchedule second: %v", err)
	}
	if first != second {
		t.Fatalf("repeated evaluation changed result: first=%+v second=%+v", first, second)
	}
	if !first.Due || !first.NominalAt.Equal(time.Date(2026, time.July, 13, 16, 0, 0, 0, time.UTC)) {
		t.Fatalf("evaluation = %+v, want due at 2026-07-13T16:00:00Z", first)
	}
}

func TestEvaluateCronSchedule_HonorsExplicitScheduleTimezone(t *testing.T) {
	svc := testCronService()
	lastEvaluatedAt := time.Date(2026, time.July, 13, 15, 59, 30, 0, time.UTC)
	evaluatedAt := time.Date(2026, time.July, 13, 16, 0, 0, 0, time.UTC)

	got, err := svc.EvaluateCronSchedule(
		"CRON_TZ=America/Los_Angeles 0 9 * * *",
		lastEvaluatedAt,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("EvaluateCronSchedule: %v", err)
	}
	if !got.Due || !got.NominalAt.Equal(evaluatedAt) {
		t.Fatalf("evaluation = %+v, want due at the 09:00 America/Los_Angeles boundary", got)
	}
}

func TestEvaluateCronSchedule_RejectsInvalidInput(t *testing.T) {
	svc := testCronService()
	now := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	if _, err := svc.EvaluateCronSchedule("not a cron", now, now); err == nil || !strings.Contains(err.Error(), `"not a cron"`) {
		t.Fatalf("invalid schedule error = %v, want actionable schedule value", err)
	}
	if _, err := svc.EvaluateCronSchedule("* * * * *", now, now.Add(-time.Second)); err == nil || !strings.Contains(err.Error(), "precedes") {
		t.Fatalf("reversed interval error = %v, want explicit ordering error", err)
	}
}

func TestCronTimeWorkRequest_UsesCanonicalInternalTimeWorkContract(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	req, metadata, err := svc.CronTimeWorkRequest("factory/main", interfaces.FactoryWorkstationConfig{
		Name: "daily-refresh",
		Cron: &interfaces.CronConfig{
			Schedule:     "* * * * *",
			Jitter:       "5s",
			ExpiryWindow: "1m",
		},
	}, nominalAt)
	if err != nil {
		t.Fatalf("CronTimeWorkRequest: %v", err)
	}

	if req.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", req.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if req.RequestID == "" {
		t.Fatal("expected deterministic cron request ID")
	}
	if len(req.Works) != 1 {
		t.Fatalf("works = %d, want 1", len(req.Works))
	}
	workItem := req.Works[0]
	if workItem.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("work type = %q, want %q", workItem.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if workItem.State != interfaces.SystemTimePendingState {
		t.Fatalf("state = %q, want %q", workItem.State, interfaces.SystemTimePendingState)
	}
	if workItem.Tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("source tag = %q, want %q", workItem.Tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
	if workItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != "daily-refresh" {
		t.Fatalf("cron workstation tag = %q", workItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation])
	}
	if workItem.Tags[interfaces.TimeWorkTagKeyDueAt] != metadata.DueAt.Format(time.RFC3339Nano) {
		t.Fatalf("due tag = %q, want %q", workItem.Tags[interfaces.TimeWorkTagKeyDueAt], metadata.DueAt.Format(time.RFC3339Nano))
	}
}

func TestCronTimeWorkRequest_EveryMinuteScheduleDeterministicWithFakeClock(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	ws := interfaces.FactoryWorkstationConfig{
		Name: "daily-refresh",
		Cron: &interfaces.CronConfig{
			Schedule: "* * * * *",
			Jitter:   "5s",
		},
	}

	req, metadata, err := svc.CronTimeWorkRequest("factory/main", ws, nominalAt)
	if err != nil {
		t.Fatalf("CronTimeWorkRequest: %v", err)
	}
	if len(req.Works) != 1 {
		t.Fatalf("works = %d, want 1", len(req.Works))
	}
	workItem := req.Works[0]

	wantJitter := svc.DeterministicCronJitter("factory/main", ws.Name, nominalAt, 5*time.Second)
	wantDueAt := nominalAt.Add(wantJitter)
	wantExpiresAt := wantDueAt.Add(time.Minute)
	wantWorkID := svc.CronTimeWorkID("factory/main", ws.Name, nominalAt)

	if workItem.WorkID != wantWorkID {
		t.Fatalf("work ID = %q, want %q", workItem.WorkID, wantWorkID)
	}
	if metadata.DueAt != wantDueAt {
		t.Fatalf("due_at = %s, want %s", metadata.DueAt, wantDueAt)
	}
	if metadata.ExpiresAt != wantExpiresAt {
		t.Fatalf("expires_at = %s, want %s", metadata.ExpiresAt, wantExpiresAt)
	}
	if metadata.Jitter != wantJitter {
		t.Fatalf("jitter = %s, want %s", metadata.Jitter, wantJitter)
	}

	wantTags := metadata.Tags()
	for key, want := range wantTags {
		if got := workItem.Tags[key]; got != want {
			t.Fatalf("tag %s = %q, want %q", key, got, want)
		}
	}

	var payload map[string]string
	payloadBytes, ok := workItem.Payload.([]byte)
	if !ok {
		t.Fatalf("payload type = %T, want []byte", workItem.Payload)
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v\npayload=%s", err, payloadBytes)
	}
	for _, key := range []string{"cron_workstation", "nominal_at", "due_at", "expires_at", "jitter", "source"} {
		if payload[key] == "" {
			t.Fatalf("payload missing %s: %#v", key, payload)
		}
	}
}
