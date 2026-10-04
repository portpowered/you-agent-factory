package mp

import (
	"os"
	osexec "os/exec" // want `mapping-process-behavior: pkg/transports/mapping/mp -> os/exec`
	"time"
)

func Map() {
	_, _ = os.ReadFile("x") // want `mapping-filesystem-behavior: pkg/transports/mapping/mp -> os.ReadFile`
	_ = os.Getenv("x")
	time.Sleep(1)           // want `mapping-timer-behavior: pkg/transports/mapping/mp -> time.Sleep`
	_ = osexec.Command("x") // want `mapping-process-behavior: pkg/transports/mapping/mp -> os/exec.Command`
	go func() {}()          // want `mapping-goroutine: pkg/transports/mapping/mp -> go statement`
}
