//go:build linux

package process

import (
	"errors"
	"strings"
	"testing"
)

func TestIncarnationLinuxCreationTokenRetainsBootAndRejectsDeadProcesses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, boot, state, start, want string
		gone, invalid                  bool
	}{
		{name: "alive", boot: "boot-one\n", state: "S", start: "123", want: "boot-one:123"},
		{name: "pid-reused", boot: "boot-one", state: "S", start: "456", want: "boot-one:456"},
		{name: "reboot", boot: "boot-two", state: "S", start: "123", want: "boot-two:123"},
		{name: "zombie", boot: "boot-one", state: "Z", start: "123", gone: true},
		{name: "dead", boot: "boot-one", state: "X", start: "123", gone: true},
		{name: "missing-boot", state: "S", start: "123", invalid: true},
		{name: "malformed-start", boot: "boot-one", state: "S", start: "unknown", invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			stat := "123 (name with ) spaces) " + test.state + strings.Repeat(" 0", 18) + " " + test.start
			got, err := processCreationToken(test.boot, stat)
			if test.gone {
				if !errors.Is(err, ErrProcessGone) {
					t.Fatalf("dead query = %v", err)
				}
				return
			}
			if test.invalid {
				if err == nil || errors.Is(err, ErrProcessGone) {
					t.Fatalf("invalid query = %v", err)
				}
				return
			}
			if err != nil || got != test.want {
				t.Fatalf("token = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
