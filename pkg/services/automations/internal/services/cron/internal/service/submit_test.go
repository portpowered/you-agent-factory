package service

import (
	"context"
	"errors"
	"testing"
	"time"

	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func cronWorkstationForSubmitTest() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: "daily-refresh",
		Cron: &interfaces.CronConfig{
			Schedule:     "*/5 * * * *",
			Jitter:       "5s",
			ExpiryWindow: "1m",
		},
	}
}

func TestSubmitDueCronTick_InvalidSchedulePerformsNoWorkSubmission(t *testing.T) {
	svc := testCronService()
	now := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		ws   interfaces.FactoryWorkstationConfig
	}{
		{
			name: "unparsable schedule",
			ws: interfaces.FactoryWorkstationConfig{
				Name: "daily-refresh",
				Cron: &interfaces.CronConfig{Schedule: "not-a-cron"},
			},
		},
		{
			name: "missing schedule",
			ws: interfaces.FactoryWorkstationConfig{
				Name: "daily-refresh",
				Cron: &interfaces.CronConfig{},
			},
		},
		{
			name: "missing cron config",
			ws: interfaces.FactoryWorkstationConfig{Name: "daily-refresh"},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			submitCalls := 0
			submitter := func(context.Context, work.WorkRequest) error {
				submitCalls++
				return nil
			}

			_, err := svc.SubmitDueCronTick(
				context.Background(),
				submitter,
				"factory/main",
				test.ws,
				now,
				now.Add(5*time.Minute),
			)
			if err == nil || !errors.Is(err, cron.ErrInvalidSchedule) {
				t.Fatalf("error = %v, want typed ErrInvalidSchedule", err)
			}
			if submitCalls != 0 {
				t.Fatalf("submitter calls = %d, want 0", submitCalls)
			}
		})
	}
}

func TestSubmitCronTick_InvalidSchedulePerformsNoWorkSubmission(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name string
		ws   interfaces.FactoryWorkstationConfig
	}{
		{
			name: "unparsable schedule",
			ws: interfaces.FactoryWorkstationConfig{
				Name: "daily-refresh",
				Cron: &interfaces.CronConfig{Schedule: "not-a-cron"},
			},
		},
		{
			name: "missing schedule",
			ws: interfaces.FactoryWorkstationConfig{
				Name: "daily-refresh",
				Cron: &interfaces.CronConfig{},
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			submitCalls := 0
			submitter := func(context.Context, work.WorkRequest) error {
				submitCalls++
				return nil
			}

			_, err := svc.SubmitCronTick(
				context.Background(),
				submitter,
				"factory/main",
				test.ws,
				nominalAt,
			)
			if err == nil || !errors.Is(err, cron.ErrInvalidSchedule) {
				t.Fatalf("error = %v, want typed ErrInvalidSchedule", err)
			}
			if submitCalls != 0 {
				t.Fatalf("submitter calls = %d, want 0", submitCalls)
			}
		})
	}
}

func TestSubmitDueCronTick_SubmitsCanonicalWorkRequestThroughInjectedCollaborator(t *testing.T) {
	svc := testCronService()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	evaluatedAt := lastEvaluatedAt.Add(5 * time.Minute)
	ws := cronWorkstationForSubmitTest()

	var submitCalls int
	var submitted work.WorkRequest
	submitter := func(_ context.Context, request work.WorkRequest) error {
		submitCalls++
		submitted = request
		return nil
	}

	result, err := svc.SubmitDueCronTick(
		context.Background(),
		submitter,
		"factory/main",
		ws,
		lastEvaluatedAt,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("SubmitDueCronTick: %v", err)
	}
	if !result.Submitted {
		t.Fatal("expected submitted tick")
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}

	wantRequest, wantMetadata, err := svc.CronTimeWorkRequest("factory/main", ws, evaluatedAt)
	if err != nil {
		t.Fatalf("CronTimeWorkRequest: %v", err)
	}
	if submitted.RequestID != wantRequest.RequestID {
		t.Fatalf("request ID = %q, want %q", submitted.RequestID, wantRequest.RequestID)
	}
	if submitted.Type != wantRequest.Type {
		t.Fatalf("request type = %q, want %q", submitted.Type, wantRequest.Type)
	}
	if len(submitted.Works) != len(wantRequest.Works) {
		t.Fatalf("works count = %d, want %d", len(submitted.Works), len(wantRequest.Works))
	}
	if submitted.Works[0].WorkID != wantRequest.Works[0].WorkID {
		t.Fatalf("work ID = %q, want %q", submitted.Works[0].WorkID, wantRequest.Works[0].WorkID)
	}
	if result.Metadata != wantMetadata {
		t.Fatalf("submission metadata = %+v, want %+v", result.Metadata, wantMetadata)
	}
	if submitted.Type != work.WorkRequestTypeFactoryRequestBatch {
		t.Fatalf("request type = %q, want %q", submitted.Type, work.WorkRequestTypeFactoryRequestBatch)
	}
	if len(submitted.Works) != 1 {
		t.Fatalf("works = %d, want 1", len(submitted.Works))
	}
	workItem := submitted.Works[0]
	if workItem.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		t.Fatalf("work type = %q, want %q", workItem.WorkTypeID, interfaces.SystemTimeWorkTypeID)
	}
	if workItem.State != interfaces.SystemTimePendingState {
		t.Fatalf("state = %q, want %q", workItem.State, interfaces.SystemTimePendingState)
	}
	if workItem.Tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		t.Fatalf("source tag = %q, want %q", workItem.Tags[interfaces.TimeWorkTagKeySource], interfaces.TimeWorkSourceCron)
	}
}

func TestSubmitDueCronTick_DoesNotCallSubmitterWhenScheduleNotDue(t *testing.T) {
	svc := testCronService()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	evaluatedAt := lastEvaluatedAt.Add(4 * time.Minute)
	ws := cronWorkstationForSubmitTest()

	submitCalls := 0
	submitter := func(context.Context, work.WorkRequest) error {
		submitCalls++
		return nil
	}

	result, err := svc.SubmitDueCronTick(
		context.Background(),
		submitter,
		"factory/main",
		ws,
		lastEvaluatedAt,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("SubmitDueCronTick: %v", err)
	}
	if result.Submitted {
		t.Fatal("expected no submission when schedule is not due")
	}
	if submitCalls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitCalls)
	}
}

func TestSubmitDueCronTick_InvalidEvaluationWindowPerformsNoWorkSubmission(t *testing.T) {
	svc := testCronService()
	now := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	ws := cronWorkstationForSubmitTest()

	submitCalls := 0
	submitter := func(context.Context, work.WorkRequest) error {
		submitCalls++
		return nil
	}

	_, err := svc.SubmitDueCronTick(
		context.Background(),
		submitter,
		"factory/main",
		ws,
		now,
		now.Add(-time.Second),
	)
	if err == nil || !errors.Is(err, cron.ErrInvalidEvaluationWindow) {
		t.Fatalf("error = %v, want typed ErrInvalidEvaluationWindow", err)
	}
	if submitCalls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitCalls)
	}
}

func TestSubmitCronTick_RequiresInjectedCollaborator(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)

	_, err := svc.SubmitCronTick(context.Background(), nil, "factory/main", cronWorkstationForSubmitTest(), nominalAt)
	if err == nil {
		t.Fatal("expected error for nil submitter")
	}
}

func TestSubmitDueCronTick_RequiresInjectedCollaborator(t *testing.T) {
	svc := testCronService()
	now := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)

	_, err := svc.SubmitDueCronTick(
		context.Background(),
		nil,
		"factory/main",
		cronWorkstationForSubmitTest(),
		now,
		now.Add(5*time.Minute),
	)
	if err == nil {
		t.Fatal("expected error for nil submitter")
	}
}

func TestCronTimeWorkRequest_IdempotentRequestIdentityForSameLogicalTick(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	ws := cronWorkstationForSubmitTest()

	first, _, err := svc.CronTimeWorkRequest("factory/main", ws, nominalAt)
	if err != nil {
		t.Fatalf("CronTimeWorkRequest first: %v", err)
	}
	second, _, err := svc.CronTimeWorkRequest("factory/main", ws, nominalAt)
	if err != nil {
		t.Fatalf("CronTimeWorkRequest second: %v", err)
	}
	if first.RequestID != second.RequestID {
		t.Fatalf("request ID changed: first=%q second=%q", first.RequestID, second.RequestID)
	}
	if len(first.Works) != 1 || len(second.Works) != 1 {
		t.Fatalf("expected one work item per request")
	}
	if first.Works[0].WorkID != second.Works[0].WorkID {
		t.Fatalf("work ID changed: first=%q second=%q", first.Works[0].WorkID, second.Works[0].WorkID)
	}
	wantWorkID := svc.CronTimeWorkID("factory/main", ws.Name, nominalAt)
	if first.Works[0].WorkID != wantWorkID {
		t.Fatalf("work ID = %q, want %q", first.Works[0].WorkID, wantWorkID)
	}
	if first.RequestID != "request-"+wantWorkID {
		t.Fatalf("request ID = %q, want request-%s", first.RequestID, wantWorkID)
	}
}

func TestSubmitCronTick_PropagatesSubmitterFailure(t *testing.T) {
	svc := testCronService()
	nominalAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	wantErr := errors.New("admission rejected")

	_, err := svc.SubmitCronTick(
		context.Background(),
		func(context.Context, work.WorkRequest) error { return wantErr },
		"factory/main",
		cronWorkstationForSubmitTest(),
		nominalAt,
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
}
