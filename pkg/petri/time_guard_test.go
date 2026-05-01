package petri

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestCronTimeWindowGuard_EvaluateAt(t *testing.T) {
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	dueAt := base.Add(2 * time.Minute)
	expiresAt := base.Add(7 * time.Minute)

	tests := []struct {
		name      string
		now       time.Time
		token     interfaces.Token
		wantOK    bool
		wantToken string
	}{
		{
			name:      "due token matches",
			now:       dueAt,
			token:     cronTimeToken("time-due", "refresh", dueAt, expiresAt),
			wantOK:    true,
			wantToken: "time-due",
		},
		{
			name:  "not yet due token fails",
			now:   dueAt.Add(-time.Nanosecond),
			token: cronTimeToken("time-early", "refresh", dueAt, expiresAt),
		},
		{
			name:  "expired token fails",
			now:   expiresAt,
			token: cronTimeToken("time-expired", "refresh", dueAt, expiresAt),
		},
		{
			name: "malformed due timestamp fails",
			now:  dueAt,
			token: cronTimeTokenWithTags("time-bad-due", map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: "refresh",
				interfaces.TimeWorkTagKeyDueAt:           "not-a-time",
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			}),
		},
		{
			name:  "wrong workstation fails",
			now:   dueAt,
			token: cronTimeToken("time-other", "other-cron", dueAt, expiresAt),
		},
		{
			name: "missing required tag fails",
			now:  dueAt,
			token: cronTimeTokenWithTags("time-missing", map[string]string{
				interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
				interfaces.TimeWorkTagKeyCronWorkstation: "refresh",
				interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guard := &CronTimeWindowGuard{Workstation: "refresh"}
			matched, ok := guard.EvaluateAt(tt.now, []interfaces.Token{tt.token}, nil, nil)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				if len(matched) != 0 {
					t.Fatalf("matched = %d, want 0", len(matched))
				}
				return
			}
			if len(matched) != 1 || matched[0].ID != tt.wantToken {
				t.Fatalf("matched = %#v, want token %q", matched, tt.wantToken)
			}
		})
	}
}

func TestExpiredTimeWorkGuard_EvaluateAt(t *testing.T) {
	base := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	expiresAt := base.Add(5 * time.Minute)
	guard := &ExpiredTimeWorkGuard{}

	candidates := []interfaces.Token{
		cronTimeToken("time-expired", "refresh", base, expiresAt),
		cronTimeToken("time-pending", "refresh", base, expiresAt.Add(time.Hour)),
		cronTimeTokenWithTags("time-bad-expiry", map[string]string{
			interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
			interfaces.TimeWorkTagKeyCronWorkstation: "refresh",
			interfaces.TimeWorkTagKeyDueAt:           base.Format(time.RFC3339Nano),
			interfaces.TimeWorkTagKeyExpiresAt:       "not-a-time",
		}),
	}

	matched, ok := guard.EvaluateAt(expiresAt, candidates, nil, nil)
	if !ok {
		t.Fatal("expected expiry guard to pass")
	}
	if len(matched) != 1 || matched[0].ID != "time-expired" {
		t.Fatalf("matched = %#v, want only time-expired", matched)
	}
}

func TestClockedTimeGuards_FailClosedWithoutRuntimeClock(t *testing.T) {
	now := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	token := cronTimeToken("time-ready", "refresh", now, now.Add(time.Minute))

	windowGuard := &CronTimeWindowGuard{Workstation: "refresh"}
	if matched, ok := windowGuard.Evaluate([]interfaces.Token{token}, nil, nil); ok || len(matched) != 0 {
		t.Fatalf("window guard direct Evaluate = (%#v, %v), want fail closed", matched, ok)
	}

	expiryGuard := &ExpiredTimeWorkGuard{}
	if matched, ok := expiryGuard.Evaluate([]interfaces.Token{token}, nil, nil); ok || len(matched) != 0 {
		t.Fatalf("expiry guard direct Evaluate = (%#v, %v), want fail closed", matched, ok)
	}
}

func cronTimeToken(id string, workstation string, dueAt time.Time, expiresAt time.Time) interfaces.Token {
	return cronTimeTokenWithTags(id, map[string]string{
		interfaces.TimeWorkTagKeySource:          interfaces.TimeWorkSourceCron,
		interfaces.TimeWorkTagKeyCronWorkstation: workstation,
		interfaces.TimeWorkTagKeyDueAt:           dueAt.Format(time.RFC3339Nano),
		interfaces.TimeWorkTagKeyExpiresAt:       expiresAt.Format(time.RFC3339Nano),
	})
}

func cronTimeTokenWithTags(id string, tags map[string]string) interfaces.Token {
	return interfaces.Token{
		ID: id,
		Color: interfaces.TokenColor{
			WorkID:     id,
			WorkTypeID: interfaces.SystemTimeWorkTypeID,
			DataType:   interfaces.DataTypeWork,
			Tags:       tags,
		},
	}
}
