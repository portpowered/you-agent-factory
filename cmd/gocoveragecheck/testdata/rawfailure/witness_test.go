package rawfailure

import (
	"fmt"
	"os"
	"testing"
)

func TestRawFailureWitness(t *testing.T) {
	if os.Getenv("FUNCTIONAL_RAW_FAILURE_WITNESS") != "1" {
		for index := 0; index < 100; index++ {
			fmt.Fprintf(os.Stdout, "successful verbose witness output line=%03d\n", index)
		}
		return
	}
	t.Log("Factory Event timeline: sequence=1 kind=work.accepted")
	t.Log("Factory Event timeline: sequence=2 kind=worker.completed")
	t.Errorf("%s", controlledFailureAssertion)
}
