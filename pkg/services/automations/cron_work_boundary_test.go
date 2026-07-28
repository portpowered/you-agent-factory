package automations_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/services/automations"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations/service"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func cronWorkstationForBoundaryTest() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: "daily-refresh",
		Kind: interfaces.WorkstationKindCron,
		Cron: &interfaces.CronConfig{
			Schedule:     "*/5 * * * *",
			Jitter:       "5s",
			ExpiryWindow: "1m",
		},
	}
}

func newCronBoundaryAutomationService() *automationservice.Service {
	return automationservice.New(
		zap.NewNop(),
		clockwork.NewFakeClock(),
		nil,
		"factory/main",
		"",
		nil,
		nil,
		nil,
	)
}

// TestCronSubmitTick_HandsWorkRootRequestToAutomationsSubmitter proves cron
// automations construct and admit Work Requests only through Work root types
// before handing them to the Automations WorkRequestSubmitter contract.
func TestCronSubmitTick_HandsWorkRootRequestToAutomationsSubmitter(t *testing.T) {
	t.Parallel()

	nominalAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	ws := cronWorkstationForBoundaryTest()
	svc := newCronBoundaryAutomationService()

	var submitCalls int
	var submitted work.WorkRequest
	var submitter automations.WorkRequestSubmitter = func(_ context.Context, request work.WorkRequest) error {
		submitCalls++
		submitted = request
		return nil
	}

	if err := svc.SubmitCronTick(
		context.Background(),
		nil,
		"factory/main",
		submitter,
		ws,
		nominalAt,
	); err != nil {
		t.Fatalf("SubmitCronTick: %v", err)
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}

	if submitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", submitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if submitted.RequestID == "" {
		t.Fatal("expected deterministic cron request ID")
	}
	if len(submitted.Works) != 1 {
		t.Fatalf("works count = %d, want 1", len(submitted.Works))
	}

	workItem := submitted.Works[0]
	if workItem.WorkID == "" {
		t.Fatal("expected deterministic cron work ID")
	}
	if submitted.RequestID != "request-"+workItem.WorkID {
		t.Fatalf("request ID = %q, want request-%s", submitted.RequestID, workItem.WorkID)
	}
	if workItem.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("work type = %q, want %q", workItem.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if workItem.State != interfaces.SystemTimePendingState {
		t.Fatalf("state = %q, want %q", workItem.State, interfaces.SystemTimePendingState)
	}
	if workItem.Name != "cron:"+ws.Name {
		t.Fatalf("work name = %q, want %q", workItem.Name, "cron:"+ws.Name)
	}
	if workItem.Tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("source tag = %q, want %q", workItem.Tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
	if workItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation] != ws.Name {
		t.Fatalf("cron workstation tag = %q, want %q", workItem.Tags[interfaces.TimeWorkTagKeyCronWorkstation], ws.Name)
	}
	if workItem.Tags[interfaces.TimeWorkTagKeyNominalAt] != nominalAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("nominal_at tag = %q, want %q", workItem.Tags[interfaces.TimeWorkTagKeyNominalAt], nominalAt.UTC().Format(time.RFC3339Nano))
	}
	payloadBytes, ok := workItem.Payload.([]byte)
	if !ok || len(payloadBytes) == 0 {
		t.Fatalf("expected cron time-work payload bytes, got %T", workItem.Payload)
	}

	var payload struct {
		CronWorkstation string `json:"cron_workstation"`
		NominalAt       string `json:"nominal_at"`
		DueAt           string `json:"due_at"`
		ExpiresAt       string `json:"expires_at"`
		Jitter          string `json:"jitter"`
		Source          string `json:"source"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		t.Fatalf("payload JSON: %v", err)
	}
	if payload.CronWorkstation != ws.Name {
		t.Fatalf("payload cron_workstation = %q, want %q", payload.CronWorkstation, ws.Name)
	}
	if payload.Source != interfaces.TimeWorkSourceCron {
		t.Fatalf("payload source = %q, want %q", payload.Source, interfaces.TimeWorkSourceCron)
	}
	if payload.NominalAt != nominalAt.UTC().Format(time.RFC3339Nano) {
		t.Fatalf("payload nominal_at = %q, want %q", payload.NominalAt, nominalAt.UTC().Format(time.RFC3339Nano))
	}
	if payload.DueAt != workItem.Tags[interfaces.TimeWorkTagKeyDueAt] {
		t.Fatalf("payload due_at = %q, want tag %q", payload.DueAt, workItem.Tags[interfaces.TimeWorkTagKeyDueAt])
	}
}

// TestCronSubmitTick_PreservesDeterministicWorkRootIdentityForEquivalentInputs
// proves equivalent cron inputs still produce the same observable Work Request
// identity fields before submitter handoff.
func TestCronSubmitTick_PreservesDeterministicWorkRootIdentityForEquivalentInputs(t *testing.T) {
	t.Parallel()

	nominalAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	ws := cronWorkstationForBoundaryTest()
	svc := newCronBoundaryAutomationService()

	capture := func() work.WorkRequest {
		t.Helper()
		var submitted work.WorkRequest
		submitter := automations.WorkRequestSubmitter(func(_ context.Context, request work.WorkRequest) error {
			submitted = request
			return nil
		})
		if err := svc.SubmitCronTick(context.Background(), nil, "factory/main", submitter, ws, nominalAt); err != nil {
			t.Fatalf("SubmitCronTick: %v", err)
		}
		return submitted
	}

	first := capture()
	second := capture()
	if first.RequestID != second.RequestID {
		t.Fatalf("request ID changed: first=%q second=%q", first.RequestID, second.RequestID)
	}
	if len(first.Works) != 1 || len(second.Works) != 1 {
		t.Fatal("expected one work item per cron request")
	}
	if first.Works[0].WorkID != second.Works[0].WorkID {
		t.Fatalf("work ID changed: first=%q second=%q", first.Works[0].WorkID, second.Works[0].WorkID)
	}
	firstPayload, ok := first.Works[0].Payload.([]byte)
	if !ok {
		t.Fatalf("first payload type = %T, want []byte", first.Works[0].Payload)
	}
	secondPayload, ok := second.Works[0].Payload.([]byte)
	if !ok {
		t.Fatalf("second payload type = %T, want []byte", second.Works[0].Payload)
	}
	if string(firstPayload) != string(secondPayload) {
		t.Fatalf("payload changed: first=%s second=%s", firstPayload, secondPayload)
	}
}
