// Package cron owns Automations schedule evaluation, deterministic jitter and
// expiry, and cron tick Work-request materialization. Callers outside
// Automations consume the outer Automations service root instead of this
// parent-private package.
package cron

import (
	"encoding/json"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// Service owns schedule evaluation, timing metadata, and tick Work-request
// materialization for cron workstations.
type Service interface {
	ValidateCronSchedule(schedule string) error
	ParseCronJitter(cron *interfaces.CronConfig) (time.Duration, error)
	ParseCronExpiryWindow(cron *interfaces.CronConfig, scheduleWindow time.Duration) (time.Duration, error)
	EvaluateCronSchedule(schedule string, lastEvaluatedAt, evaluatedAt time.Time) (CronScheduleEvaluation, error)
	CronScheduleWindow(schedule string, nominalAt time.Time) (time.Duration, error)
	ParseCronTiming(cron *interfaces.CronConfig, nominalAt time.Time) (CronTiming, error)
	BuildCronTimeMetadata(input CronTimeInput) (CronTimeMetadata, error)
	DeterministicCronJitter(workflowIdentity, workstationName string, nominalAt time.Time, maxJitter time.Duration) time.Duration
	CronTimeWorkID(workflowIdentity, workstationName string, nominalAt time.Time) string
	CronTimeWorkRequest(workflowIdentity string, ws interfaces.FactoryWorkstationConfig, nominalAt time.Time) (work.WorkRequest, CronTimeMetadata, error)
}

// CronTiming contains parsed timing values from a cron workstation config.
type CronTiming struct {
	MaxJitter    time.Duration
	ExpiryWindow time.Duration
}

// CronScheduleEvaluation describes whether a schedule became due between two
// explicit instants and identifies the first matching nominal fire time.
type CronScheduleEvaluation struct {
	Due       bool
	NominalAt time.Time
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
