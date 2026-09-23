package rawfailure

import (
	"fmt"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/cmd/gocoveragecheck/testdata/rawfailurewitness"
)

func TestRawFailureWitness(t *testing.T) {
	if os.Getenv("FUNCTIONAL_RAW_FAILURE_WITNESS") != "1" {
		for index := 0; index < 100; index++ {
			fmt.Fprintf(os.Stdout, "successful verbose witness output line=%03d\n", index)
		}
		return
	}
	rawfailurewitness.Run(t, "rawfailure", controlledFailureAssertion+" primary")
}
