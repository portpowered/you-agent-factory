package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/cronschedule"
	croncontract "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

const cronSubmissionNamePrefix = "cron:"

func (*service) ParseCronJitter(cronConfig *interfaces.CronConfig) (time.Duration, error) {
	authored := ""
	if cronConfig != nil {
		authored = cronConfig.Jitter
	}
	return cronschedule.ParseJitter(authored)
}

func (s *service) ParseCronExpiryWindow(cronConfig *interfaces.CronConfig, scheduleWindow time.Duration) (time.Duration, error) {
	authored := ""
	if cronConfig != nil {
		authored = cronConfig.ExpiryWindow
	}
	return cronschedule.ParseExpiryWindow(authored, scheduleWindow)
}

func (s *service) ValidateCronSchedule(schedule string) error {
	return cronschedule.ValidateSchedule(schedule)
}

func (s *service) EvaluateCronSchedule(schedule string, lastEvaluatedAt, evaluatedAt time.Time) (croncontract.CronScheduleEvaluation, error) {
	lastEvaluatedAt = lastEvaluatedAt.UTC()
	evaluatedAt = evaluatedAt.UTC()
	if evaluatedAt.Before(lastEvaluatedAt) {
		return croncontract.CronScheduleEvaluation{}, fmt.Errorf(
			"%w: evaluation time %s precedes last evaluation time %s",
			croncontract.ErrInvalidEvaluationWindow,
			evaluatedAt.Format(time.RFC3339Nano),
			lastEvaluatedAt.Format(time.RFC3339Nano),
		)
	}

	nominalAt, err := cronschedule.NextFire(schedule, lastEvaluatedAt)
	if err != nil {
		return croncontract.CronScheduleEvaluation{}, err
	}
	return croncontract.CronScheduleEvaluation{
		Due:       !nominalAt.After(evaluatedAt),
		NominalAt: nominalAt,
	}, nil
}

func (s *service) CronScheduleWindow(schedule string, nominalAt time.Time) (time.Duration, error) {
	nominalAt = nominalAt.UTC()
	next, err := cronschedule.NextFire(schedule, nominalAt)
	if err != nil {
		return 0, err
	}
	window := next.Sub(nominalAt)
	if window <= 0 {
		return 0, fmt.Errorf("cron schedule %q produced non-positive next fire window", schedule)
	}
	return window, nil
}

func (s *service) ParseCronTiming(cronConfig *interfaces.CronConfig, nominalAt time.Time) (croncontract.CronTiming, error) {
	if cronConfig == nil {
		return croncontract.CronTiming{}, fmt.Errorf("%w: missing cron config", croncontract.ErrInvalidSchedule)
	}
	schedule := strings.TrimSpace(cronConfig.Schedule)
	every := strings.TrimSpace(cronConfig.Every)
	var scheduleWindow time.Duration
	var err error
	if schedule != "" && every == "" {
		scheduleWindow, err = s.CronScheduleWindow(schedule, nominalAt)
		if err != nil {
			return croncontract.CronTiming{}, err
		}
	} else if schedule == "" && every != "" {
		scheduleWindow, err = time.ParseDuration(every)
		if err != nil || scheduleWindow <= 0 {
			return croncontract.CronTiming{}, fmt.Errorf("%w: every must be a positive duration", croncontract.ErrInvalidSchedule)
		}
	} else {
		return croncontract.CronTiming{}, fmt.Errorf("%w: exactly one of schedule or every is required", croncontract.ErrInvalidSchedule)
	}
	jitter, err := s.ParseCronJitter(cronConfig)
	if err != nil {
		return croncontract.CronTiming{}, fmt.Errorf("jitter: %w", err)
	}
	expiryWindow, err := s.ParseCronExpiryWindow(cronConfig, scheduleWindow)
	if err != nil {
		return croncontract.CronTiming{}, fmt.Errorf("expiry_window: %w", err)
	}
	return croncontract.CronTiming{MaxJitter: jitter, ExpiryWindow: expiryWindow}, nil
}

func (s *service) BuildCronTimeMetadata(input croncontract.CronTimeInput) (croncontract.CronTimeMetadata, error) {
	if input.WorkstationName == "" {
		return croncontract.CronTimeMetadata{}, fmt.Errorf("workstation name is required")
	}
	if input.MaxJitter < 0 {
		return croncontract.CronTimeMetadata{}, fmt.Errorf("%w: max jitter must be non-negative", croncontract.ErrInvalidJitter)
	}
	if input.ExpiryWindow <= 0 {
		return croncontract.CronTimeMetadata{}, fmt.Errorf("%w: expiry window must be positive", croncontract.ErrInvalidExpiryWindow)
	}

	nominalAt := input.NominalAt.UTC()
	jitter := s.DeterministicCronJitter(input.WorkflowIdentity, input.WorkstationName, nominalAt, input.MaxJitter)
	dueAt := nominalAt.Add(jitter)

	return croncontract.CronTimeMetadata{
		CronWorkstation: input.WorkstationName,
		NominalAt:       nominalAt,
		DueAt:           dueAt,
		ExpiresAt:       dueAt.Add(input.ExpiryWindow),
		Jitter:          jitter,
		Source:          interfaces.TimeWorkSourceCron,
	}, nil
}

func (*service) DeterministicCronJitter(workflowIdentity, workstationName string, nominalAt time.Time, maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	sum := cronHash(workflowIdentity, workstationName, nominalAt.UTC())
	raw := binary.BigEndian.Uint64(sum[:8])
	return time.Duration(raw % (uint64(maxJitter) + 1))
}

func (*service) CronTimeWorkID(workflowIdentity, workstationName string, nominalAt time.Time) string {
	sum := cronHash(workflowIdentity, workstationName, nominalAt.UTC())
	return "time-" + hex.EncodeToString(sum[:16])
}

func (s *service) CronTimeWorkRequest(workflowIdentity string, ws interfaces.FactoryWorkstationConfig, nominalAt time.Time) (work.WorkRequest, croncontract.CronTimeMetadata, error) {
	timing, err := s.ParseCronTiming(ws.Cron, nominalAt)
	if err != nil {
		return work.WorkRequest{}, croncontract.CronTimeMetadata{}, err
	}
	metadata, err := s.BuildCronTimeMetadata(croncontract.CronTimeInput{
		WorkflowIdentity: workflowIdentity,
		WorkstationName:  ws.Name,
		NominalAt:        nominalAt,
		MaxJitter:        timing.MaxJitter,
		ExpiryWindow:     timing.ExpiryWindow,
	})
	if err != nil {
		return work.WorkRequest{}, croncontract.CronTimeMetadata{}, err
	}
	payload, err := metadata.Payload()
	if err != nil {
		return work.WorkRequest{}, croncontract.CronTimeMetadata{}, err
	}

	workID := s.CronTimeWorkID(workflowIdentity, ws.Name, nominalAt)
	request := work.WorkRequest{
		RequestID: "request-" + workID,
		Type:      work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			WorkID:     workID,
			Name:       cronSubmissionNamePrefix + ws.Name,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			State:      interfaces.SystemTimePendingState,
			Payload:    payload,
			Tags:       metadata.Tags(),
		}},
	}
	return request, metadata, nil
}

func cronHash(workflowIdentity, workstationName string, nominalAt time.Time) [32]byte {
	input := workflowIdentity + "\x00" + workstationName + "\x00" + nominalAt.UTC().Format(time.RFC3339Nano)
	return sha256.Sum256([]byte(input))
}
