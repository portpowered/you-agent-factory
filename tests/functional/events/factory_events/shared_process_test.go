package factory_events

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var factoryEventsCLIProcess support.ApplicationProcess

func TestMain(m *testing.M) {
	process, err := support.BuildProcessWithContext(context.Background(), serviceedges.Edges{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "build Factory Events CLI process: %v\n", err)
		os.Exit(1)
	}
	factoryEventsCLIProcess = process
	exitCode := m.Run()
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		fmt.Fprintf(os.Stderr, "close Factory Events CLI process: %v\n", err)
		exitCode = 1
	}
	os.Exit(exitCode)
}
