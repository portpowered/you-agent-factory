package petri

import (
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/token"
)

// CronTimeWindowGuard matches internal cron time tokens for one workstation
// when the runtime clock is within [due_at, expires_at).
type CronTimeWindowGuard struct {
	Workstation string
}

var _ ClockedGuard = (*CronTimeWindowGuard)(nil)

// Evaluate fails closed unless called through EvaluateAt with a runtime clock.
func (g *CronTimeWindowGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	return g.EvaluateAt(time.Time{}, candidates, bindings, marking)
}

// EvaluateAt returns candidates with valid cron time metadata for the target
// workstation where due_at <= now < expires_at.
func (g *CronTimeWindowGuard) EvaluateAt(now time.Time, candidates []factorytoken.Token, _ map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	if now.IsZero() || g.Workstation == "" {
		return nil, false
	}
	now = now.UTC()

	matched := make([]factorytoken.Token, 0, len(candidates))
	for _, candidate := range candidates {
		if cronTimeWindowMatches(candidate, g.Workstation, now) {
			matched = append(matched, candidate)
		}
	}
	return matched, len(matched) > 0
}

// ExpiredTimeWorkGuard matches internal cron time tokens whose expires_at has
// passed according to the runtime clock. It is used by the system expiry path.
type ExpiredTimeWorkGuard struct{}

var _ ClockedGuard = (*ExpiredTimeWorkGuard)(nil)

// Evaluate fails closed unless called through EvaluateAt with a runtime clock.
func (g *ExpiredTimeWorkGuard) Evaluate(candidates []factorytoken.Token, bindings map[string]*factorytoken.Token, marking *MarkingSnapshot) ([]factorytoken.Token, bool) {
	return g.EvaluateAt(time.Time{}, candidates, bindings, marking)
}

// EvaluateAt returns candidates with valid cron time metadata where now >= expires_at.
func (g *ExpiredTimeWorkGuard) EvaluateAt(now time.Time, candidates []factorytoken.Token, _ map[string]*factorytoken.Token, _ *MarkingSnapshot) ([]factorytoken.Token, bool) {
	if now.IsZero() {
		return nil, false
	}
	now = now.UTC()

	matched := make([]factorytoken.Token, 0, len(candidates))
	for _, candidate := range candidates {
		if expiredTimeWorkMatches(candidate, now) {
			matched = append(matched, candidate)
		}
	}
	return matched, len(matched) > 0
}

func cronTimeWindowMatches(token factorytoken.Token, workstation string, now time.Time) bool {
	if !isCronTimeToken(token, workstation) {
		return false
	}

	dueAt, ok := parseTimeWorkTag(token, interfaces.TimeWorkTagKeyDueAt)
	if !ok {
		return false
	}
	expiresAt, ok := parseTimeWorkTag(token, interfaces.TimeWorkTagKeyExpiresAt)
	if !ok {
		return false
	}

	return !now.Before(dueAt) && now.Before(expiresAt)
}

func expiredTimeWorkMatches(token factorytoken.Token, now time.Time) bool {
	if !isCronTimeToken(token, "") {
		return false
	}
	expiresAt, ok := parseTimeWorkTag(token, interfaces.TimeWorkTagKeyExpiresAt)
	if !ok {
		return false
	}
	return !now.Before(expiresAt)
}

func isCronTimeToken(token factorytoken.Token, workstation string) bool {
	if token.Color.WorkTypeID != interfaces.SystemTimeWorkTypeID {
		return false
	}
	if token.Color.Tags[interfaces.TimeWorkTagKeySource] != interfaces.TimeWorkSourceCron {
		return false
	}
	cronWorkstation, ok := token.Color.Tags[interfaces.TimeWorkTagKeyCronWorkstation]
	if !ok || cronWorkstation == "" {
		return false
	}
	return workstation == "" || cronWorkstation == workstation
}

func parseTimeWorkTag(token factorytoken.Token, key string) (time.Time, bool) {
	value, ok := token.Color.Tags[key]
	if !ok || value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed.UTC(), true
}
