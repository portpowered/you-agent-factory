//go:build windows || linux

package process

import (
	"os"
	"testing"
)

func TestIncarnationUsesStableOSCreationIdentity(t *testing.T) {
	t.Parallel()
	probe := IncarnationProbe{}
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
