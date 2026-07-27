package service

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	croncontract "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	cronparser "github.com/robfig/cron/v3"
)

const cronSubmissionNamePrefix = "cron:"

func (*service) ParseCronJitter(cronConfig *interfaces.CronConfig) (time.Duration, error) {
	if cronConfig == nil || cronConfig.Jitter == "" {
		return 0, nil
	}
	jitter, err := time.ParseDuration(cronConfig.Jitter)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", croncontract.ErrInvalidJitter, err)
	}
	if jitter < 0 {
		return 0, fmt.Errorf("%w: duration must be non-negative", croncontract.ErrInvalidJitter)
	}
	return jitter, nil
}

func (s *service) ParseCronExpiryWindow(cronConfig *interfaces.CronConfig, scheduleWindow time.Duration) (time.Duration, error) {
	if cronConfig == nil || cronConfig.ExpiryWindow == "" {
		if scheduleWindow <= 0 {
			return 0, fmt.Errorf("%w: schedule window default must be positive", croncontract.ErrInvalidExpiryWindow)
		}
		return scheduleWindow, nil
	}
	expiryWindow, err := time.ParseDuration(cronConfig.ExpiryWindow)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", croncontract.ErrInvalidExpiryWindow, err)
	}
	if expiryWindow <= 0 {
		return 0, fmt.Errorf("%w: duration must be positive", croncontract.ErrInvalidExpiryWindow)
	}
	return expiryWindow, nil
}

func (s *service) ValidateCronSchedule(schedule string) error {
	_, err := nextCronScheduleFire(schedule, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	return err
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

	nominalAt, err := nextCronScheduleFire(schedule, lastEvaluatedAt)
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
	next, err := nextCronScheduleFire(schedule, nominalAt)
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
	if schedule == "" {
		return croncontract.CronTiming{}, fmt.Errorf("%w: schedule is required", croncontract.ErrInvalidSchedule)
	}
	scheduleWindow, err := s.CronScheduleWindow(schedule, nominalAt)
	if err != nil {
		return croncontract.CronTiming{}, err
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

func nextCronScheduleFire(schedule string, after time.Time) (time.Time, error) {
	schedule = strings.TrimSpace(schedule)
	if schedule == "" {
		return time.Time{}, fmt.Errorf("%w: schedule is required", croncontract.ErrInvalidSchedule)
	}
	after = after.UTC()
	parseInput := schedule
	if !strings.HasPrefix(schedule, "TZ=") && !strings.HasPrefix(schedule, "CRON_TZ=") {
		parseInput = "CRON_TZ=UTC " + schedule
	}
	parsed, err := cronparser.ParseStandard(parseInput)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid cron schedule %q: %v", croncontract.ErrInvalidSchedule, schedule, err)
	}
	next := parsed.Next(after)
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("%w: cron schedule %q produced no next fire", croncontract.ErrInvalidSchedule, schedule)
	}
	return next.UTC(), nil
}

func cronHash(workflowIdentity, workstationName string, nominalAt time.Time) [32]byte {
	input := workflowIdentity + "\x00" + workstationName + "\x00" + nominalAt.UTC().Format(time.RFC3339Nano)
	return sha256.Sum256([]byte(input))
}
