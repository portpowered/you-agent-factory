package testkit

import "testing"

func TestInMemoryRunnerConformsToCommonContract(t *testing.T) {
	Run(t, NewInMemorySubject())
}
