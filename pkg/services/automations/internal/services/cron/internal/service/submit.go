package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	croncontract "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (s *service) SubmitDueCronTick(
	ctx context.Context,
	submitter croncontract.WorkRequestSubmitter,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	lastEvaluatedAt, evaluatedAt time.Time,
) (croncontract.CronTickSubmission, error) {
	if submitter == nil {
		return croncontract.CronTickSubmission{}, fmt.Errorf("cron: work request submitter is required")
	}
	if ws.Cron == nil {
		return croncontract.CronTickSubmission{}, fmt.Errorf("%w: missing cron config", croncontract.ErrInvalidSchedule)
	}
	schedule := strings.TrimSpace(ws.Cron.Schedule)
	if schedule == "" {
		return croncontract.CronTickSubmission{}, fmt.Errorf("%w: schedule is required", croncontract.ErrInvalidSchedule)
	}

	evaluation, err := s.EvaluateCronSchedule(schedule, lastEvaluatedAt, evaluatedAt)
	if err != nil {
		return croncontract.CronTickSubmission{}, err
	}
	if !evaluation.Due {
		return croncontract.CronTickSubmission{Submitted: false}, nil
	}
	return s.submitCronTick(ctx, submitter, workflowIdentity, ws, evaluation.NominalAt)
}

func (s *service) SubmitCronTick(
	ctx context.Context,
	submitter croncontract.WorkRequestSubmitter,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	nominalAt time.Time,
) (croncontract.CronTickSubmission, error) {
	if submitter == nil {
		return croncontract.CronTickSubmission{}, fmt.Errorf("cron: work request submitter is required")
	}
	return s.submitCronTick(ctx, submitter, workflowIdentity, ws, nominalAt)
}

func (s *service) submitCronTick(
	ctx context.Context,
	submitter croncontract.WorkRequestSubmitter,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	nominalAt time.Time,
) (croncontract.CronTickSubmission, error) {
	workRequest, metadata, err := s.CronTimeWorkRequest(workflowIdentity, ws, nominalAt)
	if err != nil {
		return croncontract.CronTickSubmission{}, err
	}
	if err := submitter(ctx, workRequest); err != nil {
		return croncontract.CronTickSubmission{}, err
	}
	return croncontract.CronTickSubmission{
		Submitted: true,
		Metadata:  metadata,
	}, nil
}
