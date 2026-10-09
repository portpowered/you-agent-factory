package workersessions

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestWorkerSessionAddressCandidatesAreDetachedDistinctIdentities(t *testing.T) {
	t.Parallel()
	work := "work-a"
	input := []AddressCandidate{
		{FactorySessionID: "b", WorkerSessionID: "legacy", State: StateRunning},
		{FactorySessionID: "a", WorkerSessionID: "legacy", WorkID: &work, State: StateCompleted},
		{FactorySessionID: "b", WorkerSessionID: "legacy", State: StateCompleted},
	}
	err := (AmbiguousAddressError{Candidates: input}).Clone()
	work = "mutated"
	input[0].FactorySessionID = "mutated"
	if !errors.Is(err, ErrWorkerSessionAmbiguous) || len(err.Candidates) != 2 || err.Candidates[0].FactorySessionID != "a" ||
		*err.Candidates[0].WorkID != "work-a" || err.Candidates[1].State != StateRunning {
		t.Fatalf("detached candidates = %+v", err.Candidates)
	}
	encoded, encodeErr := json.Marshal(err)
	if encodeErr != nil {
		t.Fatal(encodeErr)
	}
	const want = `{"candidates":[{"factorySessionId":"a","workerSessionId":"legacy","workId":"work-a","state":"COMPLETED"},{"factorySessionId":"b","workerSessionId":"legacy","workId":null,"state":"RUNNING"}]}`
	if string(encoded) != want {
		t.Fatalf("JSON = %s, want %s", encoded, want)
	}
}

func TestWorkerSessionAddressCandidatePreservesPrimaryWork(t *testing.T) {
	t.Parallel()
	for _, works := range [][]string{nil, {""}, {"", "peer-work"}, {"primary", "other"}} {
		observation := Observation{WorkerSessionID: "worker", FactorySessionID: "factory", WorkIDs: works, State: StateRunning}
		candidate := observation.AddressCandidate()
		var expected *string
		if len(works) > 0 && works[0] != "" {
			work := works[0]
			expected = &work
		}
		if !reflect.DeepEqual(candidate.WorkID, expected) {
			t.Fatalf("primary Work for %q = %v", works, candidate.WorkID)
		}
		if candidate.WorkID != nil {
			*candidate.WorkID = "mutated"
			if observation.WorkIDs[0] == "mutated" {
				t.Fatal("candidate aliases observation")
			}
		}
	}
}
