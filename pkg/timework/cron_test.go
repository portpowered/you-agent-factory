package timework

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestBuildCronTimeMetadata_DeterministicForSameWorkflowWorkstationAndNominalTime(t *testing.T) {
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	input := CronTimeInput{
		WorkflowIdentity: "factory/main",
		WorkstationName:  "daily-refresh",
		NominalAt:        nominalAt,
		MaxJitter:        5 * time.Second,
		ExpiryWindow:     time.Minute,
	}

	first, err := BuildCronTimeMetadata(input)
	if err != nil {
		t.Fatalf("BuildCronTimeMetadata first: %v", err)
	}
	second, err := BuildCronTimeMetadata(input)
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
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	timing, err := ParseCronTiming(&interfaces.CronConfig{Schedule: "*/5 * * * *"}, nominalAt)
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
	_, err := ParseCronTiming(&interfaces.CronConfig{Schedule: "not a cron"}, time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected invalid schedule error")
	}
	if !strings.Contains(err.Error(), `"not a cron"`) {
		t.Fatalf("expected error to include bad schedule value, got %v", err)
	}
}

func TestCronTimeWorkRequest_UsesCanonicalInternalTimeWorkContract(t *testing.T) {
	nominalAt := time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC)
	req, metadata, err := CronTimeWorkRequest("factory/main", interfaces.FactoryWorkstationConfig{
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

	if req.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", req.Type, interfaces.WorkRequestTypeFactoryRequestBatch)
	}
	if req.RequestID == "" {
		t.Fatal("expected deterministic cron request ID")
	}
	if len(req.Works) != 1 {
		t.Fatalf("works = %d, want 1", len(req.Works))
	}
	work := req.Works[0]
	if work.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("work type = %q, want %q", work.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if work.State != interfaces.SystemTimePendingState {
		t.Fatalf("state = %q, want %q", work.State, interfaces.SystemTimePendingState)
	}
	if work.Tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("source tag = %q, want %q", work.Tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
	if work.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != "daily-refresh" {
		t.Fatalf("cron workstation tag = %q", work.Tags[interfaces.TimeWorkTagKeyCronWorkstation])
	}
	if work.Tags[interfaces.TimeWorkTagKeyDueAt] != metadata.DueAt.Format(time.RFC3339Nano) {
		t.Fatalf("due tag = %q, want %q", work.Tags[interfaces.TimeWorkTagKeyDueAt], metadata.DueAt.Format(time.RFC3339Nano))
	}
}

func TestCronTimeWorkRequest_EveryMinuteScheduleDeterministicWithFakeClock(t *testing.T) {
	clock := clockwork.NewFakeClockAt(time.Date(2026, 4, 18, 12, 30, 0, 0, time.UTC))
	nominalAt := clock.Now()
	ws := interfaces.FactoryWorkstationConfig{
		Name: "daily-refresh",
		Cron: &interfaces.CronConfig{
			Schedule: "* * * * *",
			Jitter:   "5s",
		},
	}

	req, metadata, err := CronTimeWorkRequest("factory/main", ws, nominalAt)
	if err != nil {
		t.Fatalf("CronTimeWorkRequest: %v", err)
	}
	if len(req.Works) != 1 {
		t.Fatalf("works = %d, want 1", len(req.Works))
	}
	work := req.Works[0]

	wantJitter := DeterministicCronJitter("factory/main", ws.Name, nominalAt, 5*time.Second)
	wantDueAt := nominalAt.Add(wantJitter)
	wantExpiresAt := wantDueAt.Add(time.Minute)
	wantWorkID := CronTimeWorkID("factory/main", ws.Name, nominalAt)

	if work.WorkID != wantWorkID {
		t.Fatalf("work ID = %q, want %q", work.WorkID, wantWorkID)
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
		if got := work.Tags[key]; got != want {
			t.Fatalf("tag %s = %q, want %q", key, got, want)
		}
	}

	var payload map[string]string
	payloadBytes, ok := work.Payload.([]byte)
	if !ok {
		t.Fatalf("payload type = %T, want []byte", work.Payload)
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
