package timework

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	cronSubmissionNamePrefix = "cron:"
)

// CronTiming contains parsed timing values from a cron workstation config.
type CronTiming struct {
	MaxJitter    time.Duration
	ExpiryWindow time.Duration
}

// CronTimeInput contains the stable inputs used to materialize one cron time tick.
type CronTimeInput struct {
	WorkflowIdentity string
	WorkstationName  string
	NominalAt        time.Time
	MaxJitter        time.Duration
	ExpiryWindow     time.Duration
}

// CronTimeMetadata is the canonical timing metadata attached to an internal time work item.
type CronTimeMetadata struct {
	CronWorkstation string        `json:"cron_workstation"`
	NominalAt       time.Time     `json:"nominal_at"`
	DueAt           time.Time     `json:"due_at"`
	ExpiresAt       time.Time     `json:"expires_at"`
	Jitter          time.Duration `json:"jitter"`
	Source          string        `json:"source"`
}

// ParseCronJitter parses cron.jitter as a non-negative Go duration and defaults to zero.
func ParseCronJitter(cron *interfaces.CronConfig) (time.Duration, error) {
	if cron == nil || cron.Jitter == "" {
		return 0, nil
	}
	jitter, err := time.ParseDuration(cron.Jitter)
	if err != nil {
		return 0, err
	}
	if jitter < 0 {
		return 0, fmt.Errorf("duration must be non-negative")
	}
	return jitter, nil
}

// ParseCronExpiryWindow parses cron.expiry_window as a positive Go duration.
// When omitted, it defaults to the supplied schedule window.
func ParseCronExpiryWindow(cron *interfaces.CronConfig, scheduleWindow time.Duration) (time.Duration, error) {
	if cron == nil || cron.ExpiryWindow == "" {
		if scheduleWindow <= 0 {
			return 0, fmt.Errorf("schedule window default must be positive")
		}
		return scheduleWindow, nil
	}
	expiryWindow, err := time.ParseDuration(cron.ExpiryWindow)
	if err != nil {
		return 0, err
	}
	if expiryWindow <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return expiryWindow, nil
}

// ValidateCronSchedule validates a standard 5-field cron schedule through gocron.
func ValidateCronSchedule(schedule string) error {
	_, err := nextCronScheduleFire(schedule, time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	return err
}

// CronScheduleWindow returns the duration between a nominal cron fire and the
// next scheduled fire for the same schedule.
func CronScheduleWindow(schedule string, nominalAt time.Time) (time.Duration, error) {
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

// ParseCronTiming parses the jitter and expiry window for schedule-based cron workstations.
func ParseCronTiming(cron *interfaces.CronConfig, nominalAt time.Time) (CronTiming, error) {
	if cron == nil {
		return CronTiming{}, fmt.Errorf("missing cron config")
	}
	schedule := strings.TrimSpace(cron.Schedule)
	if schedule == "" {
		return CronTiming{}, fmt.Errorf("schedule is required")
	}
	scheduleWindow, err := CronScheduleWindow(schedule, nominalAt)
	if err != nil {
		return CronTiming{}, err
	}
	jitter, err := ParseCronJitter(cron)
	if err != nil {
		return CronTiming{}, fmt.Errorf("jitter: %w", err)
	}
	expiryWindow, err := ParseCronExpiryWindow(cron, scheduleWindow)
	if err != nil {
		return CronTiming{}, fmt.Errorf("expiry_window: %w", err)
	}
	return CronTiming{MaxJitter: jitter, ExpiryWindow: expiryWindow}, nil
}

// BuildCronTimeMetadata creates deterministic timing metadata for one cron tick.
func BuildCronTimeMetadata(input CronTimeInput) (CronTimeMetadata, error) {
	if input.WorkstationName == "" {
		return CronTimeMetadata{}, fmt.Errorf("workstation name is required")
	}
	if input.MaxJitter < 0 {
		return CronTimeMetadata{}, fmt.Errorf("max jitter must be non-negative")
	}
	if input.ExpiryWindow <= 0 {
		return CronTimeMetadata{}, fmt.Errorf("expiry window must be positive")
	}

	nominalAt := input.NominalAt.UTC()
	jitter := DeterministicCronJitter(input.WorkflowIdentity, input.WorkstationName, nominalAt, input.MaxJitter)
	dueAt := nominalAt.Add(jitter)

	return CronTimeMetadata{
		CronWorkstation: input.WorkstationName,
		NominalAt:       nominalAt,
		DueAt:           dueAt,
		ExpiresAt:       dueAt.Add(input.ExpiryWindow),
		Jitter:          jitter,
		Source:          interfaces.TimeWorkSourceCron,
	}, nil
}

// DeterministicCronJitter returns a stable jitter offset in the inclusive range [0, maxJitter].
func DeterministicCronJitter(workflowIdentity, workstationName string, nominalAt time.Time, maxJitter time.Duration) time.Duration {
	if maxJitter <= 0 {
		return 0
	}

	sum := cronHash(workflowIdentity, workstationName, nominalAt.UTC())
	raw := binary.BigEndian.Uint64(sum[:8])
	return time.Duration(raw % (uint64(maxJitter) + 1))
}

// CronTimeWorkID returns a stable work item identity for the cron time tick.
func CronTimeWorkID(workflowIdentity, workstationName string, nominalAt time.Time) string {
	sum := cronHash(workflowIdentity, workstationName, nominalAt.UTC())
	return "time-" + hex.EncodeToString(sum[:16])
}

// Tags returns the canonical time-work tags for this metadata.
func (m CronTimeMetadata) Tags() map[string]string {
	return map[string]string{
		interfaces.TimeWorkTagKeySource:          m.Source,
		interfaces.TimeWorkTagKeyCronWorkstation: m.CronWorkstation,
		interfaces.TimeWorkTagKeyNominalAt:       m.NominalAt.UTC().Format(time.RFC3339Nano),
		interfaces.TimeWorkTagKeyDueAt:           m.DueAt.UTC().Format(time.RFC3339Nano),
		interfaces.TimeWorkTagKeyExpiresAt:       m.ExpiresAt.UTC().Format(time.RFC3339Nano),
		interfaces.TimeWorkTagKeyJitter:          m.Jitter.String(),
	}
}

// Payload returns a compact JSON representation of the time-work metadata.
func (m CronTimeMetadata) Payload() ([]byte, error) {
	payload := struct {
		CronWorkstation string `json:"cron_workstation"`
		NominalAt       string `json:"nominal_at"`
		DueAt           string `json:"due_at"`
		ExpiresAt       string `json:"expires_at"`
		Jitter          string `json:"jitter"`
		Source          string `json:"source"`
	}{
		CronWorkstation: m.CronWorkstation,
		NominalAt:       m.NominalAt.UTC().Format(time.RFC3339Nano),
		DueAt:           m.DueAt.UTC().Format(time.RFC3339Nano),
		ExpiresAt:       m.ExpiresAt.UTC().Format(time.RFC3339Nano),
		Jitter:          m.Jitter.String(),
		Source:          m.Source,
	}
	return json.Marshal(payload)
}

// CronTimeWorkRequest creates the canonical internal work request for one cron tick.
func CronTimeWorkRequest(workflowIdentity string, ws interfaces.FactoryWorkstationConfig, nominalAt time.Time) (interfaces.WorkRequest, CronTimeMetadata, error) {
	timing, err := ParseCronTiming(ws.Cron, nominalAt)
	if err != nil {
		return interfaces.WorkRequest{}, CronTimeMetadata{}, err
	}
	metadata, err := BuildCronTimeMetadata(CronTimeInput{
		WorkflowIdentity: workflowIdentity,
		WorkstationName:  ws.Name,
		NominalAt:        nominalAt,
		MaxJitter:        timing.MaxJitter,
		ExpiryWindow:     timing.ExpiryWindow,
	})
	if err != nil {
		return interfaces.WorkRequest{}, CronTimeMetadata{}, err
	}
	payload, err := metadata.Payload()
	if err != nil {
		return interfaces.WorkRequest{}, CronTimeMetadata{}, err
	}

	workID := CronTimeWorkID(workflowIdentity, ws.Name, nominalAt)
	request := interfaces.WorkRequest{
		RequestID: "request-" + workID,
		Type:      interfaces.WorkRequestTypeFactoryRequestBatch,
		Works: []interfaces.Work{{
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
		return time.Time{}, fmt.Errorf("cron schedule is required")
	}
	after = after.UTC()
	scheduler, err := gocron.NewScheduler(
		gocron.WithClock(clockwork.NewFakeClockAt(after)),
		gocron.WithLocation(time.UTC),
	)
	if err != nil {
		return time.Time{}, fmt.Errorf("create cron schedule parser for %q: %w", schedule, err)
	}
	defer func() {
		_ = scheduler.Shutdown()
	}()

	scheduler.Start()
	job, err := scheduler.NewJob(gocron.CronJob(schedule, false), gocron.NewTask(func() {}))
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron schedule %q: %w", schedule, err)
	}
	next, err := job.NextRun()
	if err != nil {
		return time.Time{}, fmt.Errorf("next cron fire for %q: %w", schedule, err)
	}
	if next.IsZero() {
		return time.Time{}, fmt.Errorf("cron schedule %q produced no next fire", schedule)
	}
	return next.UTC(), nil
}

func cronHash(workflowIdentity, workstationName string, nominalAt time.Time) [32]byte {
	input := workflowIdentity + "\x00" + workstationName + "\x00" + nominalAt.UTC().Format(time.RFC3339Nano)
	return sha256.Sum256([]byte(input))
}
