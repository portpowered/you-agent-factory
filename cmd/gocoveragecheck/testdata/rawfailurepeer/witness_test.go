package rawfailurepeer

import (
	"os"
	"testing"

	"github.com/portpowered/infinite-you/cmd/gocoveragecheck/testdata/rawfailurewitness"
)

func TestRawFailurePeerWitness(t *testing.T) {
	if os.Getenv("FUNCTIONAL_RAW_FAILURE_WITNESS") != "1" {
		return
	}
	rawfailurewitness.Run(t, "rawfailurepeer", "controlled raw failure assertion peer")
}
