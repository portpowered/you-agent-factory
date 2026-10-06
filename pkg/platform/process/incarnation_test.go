//go:build windows || linux

package process

import (
	"errors"
	"os"
	"testing"
)

func TestIncarnationUsesStableOSCreationIdentity(t *testing.T) {
	t.Parallel()
	probe := IncarnationProbe{ReadFile: os.ReadFile}
	first, err := probe.CurrentProcess()
	if err != nil {
		t.Fatal(err)
	}
	second, err := probe.CurrentProcess()
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.PID != os.Getpid() || first.Host == "" || first.Start == "" {
		t.Fatalf("current process identity = %+v, second = %+v", first, second)
	}
}

func TestIncarnationLookupDistinguishesLiveAbsentAndInvalidProcesses(t *testing.T) {
	t.Parallel()
	probe := IncarnationProbe{ReadFile: os.ReadFile}
	current, err := probe.CurrentProcess()
	if err != nil {
		t.Fatal(err)
	}
	queried, err := probe.LookupProcess(os.Getpid())
	if err != nil || queried != current {
		t.Fatalf("live lookup = %+v, %v; want %+v", queried, err, current)
	}
	// Both supported OSes bound PIDs below this positive signed 32-bit value.
	if _, err := probe.LookupProcess(2147483647); !errors.Is(err, ErrProcessGone) {
		t.Fatalf("absent lookup = %v; want affirmative absence", err)
	}
	if _, err := probe.LookupProcess(0); err == nil || errors.Is(err, ErrProcessGone) {
		t.Fatalf("invalid lookup = %v; invalid input cannot establish death", err)
	}
}
