package workersessions

import (
	"errors"
	"sort"
)

// ErrWorkerSessionAmbiguous distinguishes a retained identity collision from
// an unknown Worker Session. Callers must select a Factory Session before use.
var ErrWorkerSessionAmbiguous = errors.New("worker session: ambiguous identity")

// AddressCandidate contains only the public identity and lifecycle facts
// needed to select an owner. A missing primary Work identity remains null.
type AddressCandidate struct {
	FactorySessionID string  `json:"factorySessionId"`
	WorkerSessionID  string  `json:"workerSessionId"`
	WorkID           *string `json:"workId"`
	State            State   `json:"state"`
}

// AddressCandidate returns detached identity facts without provider content.
func (o Observation) AddressCandidate() AddressCandidate {
	candidate := AddressCandidate{FactorySessionID: o.FactorySessionID, WorkerSessionID: o.WorkerSessionID, State: o.State}
	if len(o.WorkIDs) > 0 && o.WorkIDs[0] != "" {
		workID := o.WorkIDs[0]
		candidate.WorkID = &workID
	}
	return candidate
}

// AmbiguousAddressError carries the complete distinct owner set. Candidates
// are ordered by Factory Session and Worker Session for stable diagnostics.
type AmbiguousAddressError struct {
	Candidates []AddressCandidate `json:"candidates"`
}

func (err *AmbiguousAddressError) Error() string {
	return "Worker Session ID is ambiguous. Select a Factory Session with --session."
}

func (err *AmbiguousAddressError) Unwrap() error { return ErrWorkerSessionAmbiguous }

// NewAmbiguousAddressError freezes candidates and deduplicates representations
// of the same identity. The first representation retains authority for facts.
func NewAmbiguousAddressError(candidates []AddressCandidate) *AmbiguousAddressError {
	result := &AmbiguousAddressError{}
	type identity struct{ factory, worker string }
	seen := make(map[identity]bool, len(candidates))
	for _, candidate := range candidates {
		key := identity{candidate.FactorySessionID, candidate.WorkerSessionID}
		if seen[key] {
			continue
		}
		seen[key] = true
		if candidate.WorkID != nil {
			workID := *candidate.WorkID
			candidate.WorkID = &workID
		}
		result.Candidates = append(result.Candidates, candidate)
	}
	sort.Slice(result.Candidates, func(i, j int) bool {
		left, right := result.Candidates[i], result.Candidates[j]
		if left.FactorySessionID == right.FactorySessionID {
			return left.WorkerSessionID < right.WorkerSessionID
		}
		return left.FactorySessionID < right.FactorySessionID
	})
	return result
}
