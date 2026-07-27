package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	croncontract "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func (s *service) ResumeCronScheduleFacts(
	identity croncontract.CronScheduleIdentity,
	authoritative *croncontract.CronScheduleFacts,
	resume *croncontract.CronScheduleFacts,
) (croncontract.CronScheduleFacts, error) {
	if err := validateCronScheduleIdentity(identity); err != nil {
		return croncontract.CronScheduleFacts{}, err
	}

	switch {
	case resume == nil && authoritative == nil:
		return croncontract.CronScheduleFacts{Identity: identity}, nil
	case resume == nil:
		if authoritative.Identity != identity {
			return croncontract.CronScheduleFacts{}, fmt.Errorf(
				"%w: authoritative identity mismatch",
				croncontract.ErrStaleResumeFacts,
			)
		}
		return normalizeCronScheduleFacts(*authoritative), nil
	case authoritative == nil:
		if err := validateCronResumeFacts(identity, *resume); err != nil {
			return croncontract.CronScheduleFacts{}, err
		}
		return normalizeCronScheduleFacts(*resume), nil
	default:
		if authoritative.Identity != identity {
			return croncontract.CronScheduleFacts{}, fmt.Errorf(
				"%w: authoritative identity mismatch",
				croncontract.ErrStaleResumeFacts,
			)
		}
		if err := validateCronResumeFacts(identity, *resume); err != nil {
			return croncontract.CronScheduleFacts{}, err
		}
		if !cronScheduleFactsEquivalent(*authoritative, *resume) {
			return croncontract.CronScheduleFacts{}, fmt.Errorf(
				"%w: resume contradicts authoritative schedule facts",
				croncontract.ErrStaleResumeFacts,
			)
		}
		return normalizeCronScheduleFacts(*authoritative), nil
	}
}

func (s *service) SubmitDueCronTickWithFacts(
	ctx context.Context,
	submitter croncontract.WorkRequestSubmitter,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	facts croncontract.CronScheduleFacts,
	evaluatedAt time.Time,
) (croncontract.CronTickSubmission, croncontract.CronScheduleFacts, error) {
	identity := croncontract.CronScheduleIdentity{
		WorkflowIdentity: workflowIdentity,
		WorkstationName:  ws.Name,
	}
	if facts.Identity != identity {
		return croncontract.CronTickSubmission{}, facts, fmt.Errorf(
			"%w: schedule facts identity mismatch",
			croncontract.ErrStaleResumeFacts,
		)
	}
	if submitter == nil {
		return croncontract.CronTickSubmission{}, facts, fmt.Errorf("cron: work request submitter is required")
	}
	if ws.Cron == nil {
		return croncontract.CronTickSubmission{}, facts, fmt.Errorf("%w: missing cron config", croncontract.ErrInvalidSchedule)
	}
	schedule := strings.TrimSpace(ws.Cron.Schedule)
	if schedule == "" {
		return croncontract.CronTickSubmission{}, facts, fmt.Errorf("%w: schedule is required", croncontract.ErrInvalidSchedule)
	}

	evaluation, err := s.EvaluateCronSchedule(schedule, facts.LastEvaluatedAt, evaluatedAt)
	if err != nil {
		return croncontract.CronTickSubmission{}, facts, err
	}

	updated := normalizeCronScheduleFacts(facts)
	updated.Identity = identity
	if !evaluation.Due {
		updated.LastEvaluatedAt = evaluatedAt.UTC()
		return croncontract.CronTickSubmission{Submitted: false}, updated, nil
	}

	nominalAt := evaluation.NominalAt.UTC()
	if facts.LastSubmittedNominalAt != nil && facts.LastSubmittedNominalAt.Equal(nominalAt) {
		updated.LastEvaluatedAt = nominalAt
		return croncontract.CronTickSubmission{Submitted: false}, updated, nil
	}

	submission, err := s.submitCronTick(ctx, submitter, workflowIdentity, ws, nominalAt)
	if err != nil {
		return croncontract.CronTickSubmission{}, facts, err
	}
	updated.LastEvaluatedAt = nominalAt
	submitted := nominalAt
	updated.LastSubmittedNominalAt = &submitted
	return submission, updated, nil
}

func validateCronScheduleIdentity(identity croncontract.CronScheduleIdentity) error {
	if strings.TrimSpace(identity.WorkflowIdentity) == "" ||
		strings.TrimSpace(identity.WorkstationName) == "" {
		return fmt.Errorf("%w: workflow identity and workstation name are required", croncontract.ErrInvalidResumeFacts)
	}
	return nil
}

func validateCronResumeFacts(
	identity croncontract.CronScheduleIdentity,
	resume croncontract.CronScheduleFacts,
) error {
	if resume.Identity != identity {
		return fmt.Errorf("%w: resume identity mismatch", croncontract.ErrStaleResumeFacts)
	}
	if resume.LastSubmittedNominalAt != nil && resume.LastEvaluatedAt.IsZero() {
		return fmt.Errorf(
			"%w: submitted nominal fire requires last evaluated instant",
			croncontract.ErrInvalidResumeFacts,
		)
	}
	return nil
}

func normalizeCronScheduleFacts(facts croncontract.CronScheduleFacts) croncontract.CronScheduleFacts {
	normalized := facts
	normalized.LastEvaluatedAt = facts.LastEvaluatedAt.UTC()
	if facts.LastSubmittedNominalAt != nil {
		submitted := facts.LastSubmittedNominalAt.UTC()
		normalized.LastSubmittedNominalAt = &submitted
	}
	return normalized
}

func cronScheduleFactsEquivalent(
	left croncontract.CronScheduleFacts,
	right croncontract.CronScheduleFacts,
) bool {
	if left.Identity != right.Identity {
		return false
	}
	if !left.LastEvaluatedAt.Equal(right.LastEvaluatedAt) {
		return false
	}
	switch {
	case left.LastSubmittedNominalAt == nil && right.LastSubmittedNominalAt == nil:
		return true
	case left.LastSubmittedNominalAt == nil || right.LastSubmittedNominalAt == nil:
		return false
	default:
		return left.LastSubmittedNominalAt.Equal(*right.LastSubmittedNominalAt)
	}
}
