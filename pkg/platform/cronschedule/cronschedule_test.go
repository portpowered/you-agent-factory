package cronschedule_test

import (
	"errors"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/platform/cronschedule"
)

func TestValidateScheduleAcceptsWellFormedExpressions(t *testing.T) {
	t.Parallel()

	for _, schedule := range []string{
		"* * * * *",
		"0 * * * *",
		"0 9 * * MON-FRI",
		"@daily",
		"CRON_TZ=America/Los_Angeles 0 9 * * *",
		"  0 0 1 * *  ",
	} {
		if err := cronschedule.ValidateSchedule(schedule); err != nil {
			t.Errorf("ValidateSchedule(%q) = %v, want nil", schedule, err)
		}
	}
}

func TestValidateScheduleRejectsMalformedExpressionsAsInvalidSchedule(t *testing.T) {
	t.Parallel()

	for _, schedule := range []string{"", "   ", "not a cron", "* * * *", "99 * * * *"} {
		err := cronschedule.ValidateSchedule(schedule)
		if err == nil {
			t.Errorf("ValidateSchedule(%q) = nil, want an error", schedule)
			continue
		}
		if !errors.Is(err, cronschedule.ErrInvalidSchedule) {
			t.Errorf("ValidateSchedule(%q) error = %v, want ErrInvalidSchedule", schedule, err)
		}
	}
}

// The authoring path surfaces this error text verbatim to the operator as a
// validation finding message, so the quoted expression must survive.
func TestValidateScheduleErrorQuotesTheRejectedExpression(t *testing.T) {
	t.Parallel()

	err := cronschedule.ValidateSchedule("not a cron")
	if err == nil {
		t.Fatal("ValidateSchedule() = nil, want an error")
	}
	const want = `cron: invalid schedule: invalid cron schedule "not a cron": expected exactly 5 fields, found 3: [not a cron]`
	if err.Error() != want {
		t.Fatalf("ValidateSchedule() error = %q, want %q", err, want)
	}
}

func TestNextFireResolvesUnzonedSchedulesInUTCNotHostLocalTime(t *testing.T) {
	t.Parallel()

	// 14:30 UTC is a different wall-clock hour in almost every host zone, so an
	// hourly schedule resolved in local time would not land on 15:00 UTC.
	after := time.Date(2026, time.March, 2, 14, 30, 0, 0, time.UTC)
	next, err := cronschedule.NextFire("0 * * * *", after)
	if err != nil {
		t.Fatalf("NextFire() error = %v", err)
	}
	want := time.Date(2026, time.March, 2, 15, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("NextFire() = %s, want %s", next, want)
	}
	if next.Location() != time.UTC {
		t.Fatalf("NextFire() location = %s, want UTC", next.Location())
	}
}

func TestNextFireHonoursAnExplicitScheduleTimezone(t *testing.T) {
	t.Parallel()

	after := time.Date(2026, time.March, 2, 0, 0, 0, 0, time.UTC)
	next, err := cronschedule.NextFire("CRON_TZ=UTC 30 8 * * *", after)
	if err != nil {
		t.Fatalf("NextFire() error = %v", err)
	}
	want := time.Date(2026, time.March, 2, 8, 30, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("NextFire() = %s, want %s", next, want)
	}
}

func TestNextFireReturnsAnInstantStrictlyAfterTheGivenTime(t *testing.T) {
	t.Parallel()

	onTheHour := time.Date(2026, time.March, 2, 15, 0, 0, 0, time.UTC)
	next, err := cronschedule.NextFire("0 * * * *", onTheHour)
	if err != nil {
		t.Fatalf("NextFire() error = %v", err)
	}
	if !next.After(onTheHour) {
		t.Fatalf("NextFire() = %s, want an instant after %s", next, onTheHour)
	}
}

func TestParseJitterTreatsAnUnsetFieldAsNoJitter(t *testing.T) {
	t.Parallel()

	jitter, err := cronschedule.ParseJitter("")
	if err != nil {
		t.Fatalf("ParseJitter(\"\") error = %v, want nil", err)
	}
	if jitter != 0 {
		t.Fatalf("ParseJitter(\"\") = %s, want 0", jitter)
	}
}

func TestParseJitterAcceptsNonNegativeDurations(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		authored string
		want     time.Duration
	}{
		{authored: "0s", want: 0},
		{authored: "30s", want: 30 * time.Second},
		{authored: "1h30m", want: 90 * time.Minute},
	} {
		jitter, err := cronschedule.ParseJitter(testCase.authored)
		if err != nil {
			t.Errorf("ParseJitter(%q) error = %v", testCase.authored, err)
			continue
		}
		if jitter != testCase.want {
			t.Errorf("ParseJitter(%q) = %s, want %s", testCase.authored, jitter, testCase.want)
		}
	}
}

func TestParseJitterRejectsNegativeAndUnparseableDurations(t *testing.T) {
	t.Parallel()

	for _, authored := range []string{"-1s", "-1h", "soon", "30"} {
		_, err := cronschedule.ParseJitter(authored)
		if err == nil {
			t.Errorf("ParseJitter(%q) = nil error, want ErrInvalidJitter", authored)
			continue
		}
		if !errors.Is(err, cronschedule.ErrInvalidJitter) {
			t.Errorf("ParseJitter(%q) error = %v, want ErrInvalidJitter", authored, err)
		}
	}
}

func TestParseExpiryWindowFallsBackToTheDefaultWhenUnset(t *testing.T) {
	t.Parallel()

	window, err := cronschedule.ParseExpiryWindow("", 5*time.Minute)
	if err != nil {
		t.Fatalf("ParseExpiryWindow() error = %v", err)
	}
	if window != 5*time.Minute {
		t.Fatalf("ParseExpiryWindow() = %s, want 5m", window)
	}
}

func TestParseExpiryWindowRejectsAnUnsetFieldWithANonPositiveDefault(t *testing.T) {
	t.Parallel()

	for _, defaultWindow := range []time.Duration{0, -time.Second} {
		_, err := cronschedule.ParseExpiryWindow("", defaultWindow)
		if err == nil {
			t.Errorf("ParseExpiryWindow(\"\", %s) = nil error, want ErrInvalidExpiryWindow", defaultWindow)
			continue
		}
		if !errors.Is(err, cronschedule.ErrInvalidExpiryWindow) {
			t.Errorf("ParseExpiryWindow(\"\", %s) error = %v, want ErrInvalidExpiryWindow", defaultWindow, err)
		}
	}
}

func TestParseExpiryWindowPrefersTheAuthoredValueOverTheDefault(t *testing.T) {
	t.Parallel()

	window, err := cronschedule.ParseExpiryWindow("10m", time.Hour)
	if err != nil {
		t.Fatalf("ParseExpiryWindow() error = %v", err)
	}
	if window != 10*time.Minute {
		t.Fatalf("ParseExpiryWindow() = %s, want 10m", window)
	}
}

// Jitter tolerates a zero duration and an expiry window does not: a zero-length
// expiry window would expire every tick the instant it was created.
func TestParseExpiryWindowRejectsNonPositiveAndUnparseableDurations(t *testing.T) {
	t.Parallel()

	for _, authored := range []string{"0s", "-1s", "eventually", "10"} {
		_, err := cronschedule.ParseExpiryWindow(authored, time.Hour)
		if err == nil {
			t.Errorf("ParseExpiryWindow(%q) = nil error, want ErrInvalidExpiryWindow", authored)
			continue
		}
		if !errors.Is(err, cronschedule.ErrInvalidExpiryWindow) {
			t.Errorf("ParseExpiryWindow(%q) error = %v, want ErrInvalidExpiryWindow", authored, err)
		}
	}
}
