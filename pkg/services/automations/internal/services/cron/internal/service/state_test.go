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

func cronScheduleIdentity() cron.CronScheduleIdentity {
	return cron.CronScheduleIdentity{
		WorkflowIdentity: "factory/main",
		WorkstationName:  "daily-refresh",
	}
}

func cronWorkstationForStateTest() interfaces.FactoryWorkstationConfig {
	return interfaces.FactoryWorkstationConfig{
		Name: "daily-refresh",
		Cron: &interfaces.CronConfig{
			Schedule:     "*/5 * * * *",
			Jitter:       "5s",
			ExpiryWindow: "1m",
		},
	}
}

func TestResumeCronScheduleFacts_BootstrapsDetachedFacts(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	submitted := lastEvaluatedAt.Add(5 * time.Minute)
	resume := cron.CronScheduleFacts{
		Identity:               identity,
		LastEvaluatedAt:        submitted,
		LastSubmittedNominalAt: &submitted,
	}

	got, err := svc.ResumeCronScheduleFacts(identity, nil, &resume)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts: %v", err)
	}
	if got.Identity != resume.Identity ||
		!got.LastEvaluatedAt.Equal(resume.LastEvaluatedAt) ||
		got.LastSubmittedNominalAt == nil ||
		resume.LastSubmittedNominalAt == nil ||
		!got.LastSubmittedNominalAt.Equal(*resume.LastSubmittedNominalAt) {
		t.Fatalf("facts = %+v, want %+v", got, resume)
	}
}

func TestResumeCronScheduleFacts_ResumedNotDueProducesSameEvaluationAsFreshFacts(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	evaluatedAt := lastEvaluatedAt.Add(4 * time.Minute)
	facts := cron.CronScheduleFacts{
		Identity:        identity,
		LastEvaluatedAt: lastEvaluatedAt,
	}

	beforeRestart, err := svc.ResumeCronScheduleFacts(identity, nil, &facts)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts before restart: %v", err)
	}
	afterRestart, err := svc.ResumeCronScheduleFacts(identity, &beforeRestart, &beforeRestart)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts after restart: %v", err)
	}

	submitCalls := 0
	submitter := func(context.Context, work.WorkRequest) error {
		submitCalls++
		return nil
	}

	result, updated, err := svc.SubmitDueCronTickWithFacts(
		context.Background(),
		submitter,
		identity.WorkflowIdentity,
		cronWorkstationForStateTest(),
		afterRestart,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("SubmitDueCronTickWithFacts: %v", err)
	}
	if result.Submitted {
		t.Fatal("expected resumed schedule to remain not due")
	}
	if submitCalls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitCalls)
	}
	if !updated.LastEvaluatedAt.Equal(evaluatedAt.UTC()) {
		t.Fatalf("last evaluated = %s, want %s", updated.LastEvaluatedAt, evaluatedAt.UTC())
	}
}

func TestResumeCronScheduleFacts_ResumedDueSubmitsAndCommitsFacts(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	evaluatedAt := lastEvaluatedAt.Add(5 * time.Minute)
	facts := cron.CronScheduleFacts{
		Identity:        identity,
		LastEvaluatedAt: lastEvaluatedAt,
	}

	resumed, err := svc.ResumeCronScheduleFacts(identity, nil, &facts)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts: %v", err)
	}

	submitCalls := 0
	submitter := func(context.Context, work.WorkRequest) error {
		submitCalls++
		return nil
	}

	result, committed, err := svc.SubmitDueCronTickWithFacts(
		context.Background(),
		submitter,
		identity.WorkflowIdentity,
		cronWorkstationForStateTest(),
		resumed,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("SubmitDueCronTickWithFacts: %v", err)
	}
	if !result.Submitted {
		t.Fatal("expected resumed schedule to submit due tick")
	}
	if submitCalls != 1 {
		t.Fatalf("submitter calls = %d, want 1", submitCalls)
	}
	wantNominal := lastEvaluatedAt.Add(5 * time.Minute)
	if committed.LastSubmittedNominalAt == nil || !committed.LastSubmittedNominalAt.Equal(wantNominal) {
		t.Fatalf("submitted nominal = %+v, want %s", committed.LastSubmittedNominalAt, wantNominal)
	}
	if !committed.LastEvaluatedAt.Equal(wantNominal) {
		t.Fatalf("last evaluated = %s, want %s", committed.LastEvaluatedAt, wantNominal)
	}
}

func TestSubmitDueCronTickWithFacts_DoesNotResubmitAlreadySubmittedNominalFireAfterRestart(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	nominalAt := lastEvaluatedAt.Add(5 * time.Minute)
	evaluatedAt := nominalAt
	committed := cron.CronScheduleFacts{
		Identity:               identity,
		LastEvaluatedAt:        lastEvaluatedAt,
		LastSubmittedNominalAt: &nominalAt,
	}

	resumed, err := svc.ResumeCronScheduleFacts(identity, nil, &committed)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts: %v", err)
	}

	submitCalls := 0
	submitter := func(context.Context, work.WorkRequest) error {
		submitCalls++
		return nil
	}

	result, updated, err := svc.SubmitDueCronTickWithFacts(
		context.Background(),
		submitter,
		identity.WorkflowIdentity,
		cronWorkstationForStateTest(),
		resumed,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("SubmitDueCronTickWithFacts: %v", err)
	}
	if result.Submitted {
		t.Fatal("expected already-submitted nominal fire to skip submission")
	}
	if submitCalls != 0 {
		t.Fatalf("submitter calls = %d, want 0", submitCalls)
	}
	if updated.LastSubmittedNominalAt == nil || !updated.LastSubmittedNominalAt.Equal(nominalAt) {
		t.Fatalf("submitted nominal = %+v, want %s", updated.LastSubmittedNominalAt, nominalAt)
	}
	if !updated.LastEvaluatedAt.Equal(nominalAt) {
		t.Fatalf("last evaluated = %s, want %s", updated.LastEvaluatedAt, nominalAt)
	}
}

func TestResumeCronScheduleFacts_RejectsInvalidResumeFacts(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	submitted := lastEvaluatedAt.Add(5 * time.Minute)

	tests := []struct {
		name   string
		identity cron.CronScheduleIdentity
		resume cron.CronScheduleFacts
		want   error
	}{
		{
			name: "foreign identity",
			identity: cron.CronScheduleIdentity{
				WorkflowIdentity: "factory/other",
				WorkstationName:  "daily-refresh",
			},
			resume: cron.CronScheduleFacts{
				Identity: identity,
				LastEvaluatedAt: lastEvaluatedAt,
			},
			want: cron.ErrStaleResumeFacts,
		},
		{
			name:     "submitted nominal without last evaluated",
			identity: identity,
			resume: cron.CronScheduleFacts{
				Identity:               identity,
				LastSubmittedNominalAt: &submitted,
			},
			want: cron.ErrInvalidResumeFacts,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.ResumeCronScheduleFacts(test.identity, nil, &test.resume)
			if err == nil || !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestResumeCronScheduleFacts_RejectsStaleAndForeignResumeWithoutMutation(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	authoritative := cron.CronScheduleFacts{
		Identity:        identity,
		LastEvaluatedAt: lastEvaluatedAt,
	}

	got, err := svc.ResumeCronScheduleFacts(identity, nil, &authoritative)
	if err != nil {
		t.Fatalf("initial ResumeCronScheduleFacts: %v", err)
	}

	stale := authoritative
	stale.LastEvaluatedAt = lastEvaluatedAt.Add(time.Minute)
	_, err = svc.ResumeCronScheduleFacts(identity, &got, &stale)
	if err == nil || !errors.Is(err, cron.ErrStaleResumeFacts) {
		t.Fatalf("stale resume error = %v, want %v", err, cron.ErrStaleResumeFacts)
	}

	foreignIdentity := cron.CronScheduleIdentity{
		WorkflowIdentity: "factory/other",
		WorkstationName:  "daily-refresh",
	}
	foreign := authoritative
	_, err = svc.ResumeCronScheduleFacts(foreignIdentity, nil, &foreign)
	if err == nil || !errors.Is(err, cron.ErrStaleResumeFacts) {
		t.Fatalf("foreign resume error = %v, want %v", err, cron.ErrStaleResumeFacts)
	}

	preserved, err := svc.ResumeCronScheduleFacts(identity, &got, nil)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts authoritative read: %v", err)
	}
	if preserved != got {
		t.Fatalf("authoritative facts = %+v, want preserved %+v", preserved, got)
	}
}

func TestSubmitDueCronTickWithFacts_EquivalentClockAdvanceMatchesAfterRestart(t *testing.T) {
	svc := testCronService()
	identity := cronScheduleIdentity()
	lastEvaluatedAt := time.Date(2026, time.April, 18, 12, 30, 0, 0, time.UTC)
	evaluatedAt := lastEvaluatedAt.Add(14 * time.Minute)
	facts := cron.CronScheduleFacts{
		Identity:        identity,
		LastEvaluatedAt: lastEvaluatedAt,
	}

	var directSubmission cron.CronTickSubmission
	var directFacts cron.CronScheduleFacts
	submitter := func(context.Context, work.WorkRequest) error { return nil }

	directSubmission, directFacts, err := svc.SubmitDueCronTickWithFacts(
		context.Background(),
		submitter,
		identity.WorkflowIdentity,
		cronWorkstationForStateTest(),
		facts,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("direct SubmitDueCronTickWithFacts: %v", err)
	}

	resumed, err := svc.ResumeCronScheduleFacts(identity, nil, &facts)
	if err != nil {
		t.Fatalf("ResumeCronScheduleFacts: %v", err)
	}
	restartedSubmission, restartedFacts, err := svc.SubmitDueCronTickWithFacts(
		context.Background(),
		submitter,
		identity.WorkflowIdentity,
		cronWorkstationForStateTest(),
		resumed,
		evaluatedAt,
	)
	if err != nil {
		t.Fatalf("restarted SubmitDueCronTickWithFacts: %v", err)
	}
	if restartedSubmission.Submitted != directSubmission.Submitted {
		t.Fatalf("submitted = %t, want %t", restartedSubmission.Submitted, directSubmission.Submitted)
	}
	if restartedFacts.Identity != directFacts.Identity ||
		!restartedFacts.LastEvaluatedAt.Equal(directFacts.LastEvaluatedAt) {
		t.Fatalf("committed facts = %+v, want %+v", restartedFacts, directFacts)
	}
	switch {
	case restartedFacts.LastSubmittedNominalAt == nil && directFacts.LastSubmittedNominalAt == nil:
	case restartedFacts.LastSubmittedNominalAt != nil && directFacts.LastSubmittedNominalAt != nil &&
		restartedFacts.LastSubmittedNominalAt.Equal(*directFacts.LastSubmittedNominalAt):
	default:
		t.Fatalf("committed facts = %+v, want %+v", restartedFacts, directFacts)
	}
}

func timePtr(value time.Time) *time.Time {
	value = value.UTC()
	return &value
}
