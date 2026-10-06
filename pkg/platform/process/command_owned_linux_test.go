//go:build linux

package process

import "testing"

func TestOwnedLinuxGroupStat(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		stat string
		live bool
		bad  bool
	}{
		{name: "owned root", stat: "42 (provider) S 1 42 42", live: true},
		{name: "owned grandchild", stat: "46 (child (with spaces)) R 45 42 42", live: true},
		{name: "unrelated live group", stat: "52 (sibling) S 1 52 52"},
		{name: "exited zombie", stat: "46 (child) Z 1 42 42"},
		{name: "exited dead", stat: "46 (child) X 1 42 42"},
		{name: "missing command", stat: "42 S 1 42 42", bad: true},
		{name: "missing group", stat: "42 (provider) S 1", bad: true},
		{name: "invalid group", stat: "42 (provider) S 1 invalid", bad: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			live, err := linuxStatGroupRunning(test.stat, 42)
			if live != test.live || (err != nil) != test.bad {
				t.Fatalf("group liveness = %t, error = %v; want live=%t bad=%t", live, err, test.live, test.bad)
			}
		})
	}
}
