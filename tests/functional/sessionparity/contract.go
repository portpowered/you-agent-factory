// Package sessionparity declares test-only Factory Session parity facts.
//
// It intentionally contains only data contracts. Customer-boundary scenarios
// capture observations and pass them to the adapters added in later stories;
// this package never constructs a Factory Session runtime or service graph.
package sessionparity

// Projection is the stable, customer-visible representation used by Factory
// Session parity tests. Fields marked required in contract.md must be present
// in every projection. Optional facts are represented by pointers or empty
// slices as documented there.
type Projection struct {
	Identity     FactorySessionIdentity `json:"identity"`
	Lifecycle    LifecycleFacts         `json:"lifecycle"`
	Hashes       HashFacts              `json:"hashes"`
	Progress     ProgressFacts          `json:"progress"`
	Dispatches   []DispatchFact         `json:"dispatches"`
	Artifacts    []ArtifactFact         `json:"artifacts"`
	Results      []ResultFact           `json:"results"`
	Failures     []FailureFact          `json:"failures"`
	EventCursors []FactoryEventCursor   `json:"eventCursors"`
}

// FactorySessionIdentity identifies the one Factory Session observed by every
// retained fact in a Projection.
type FactorySessionIdentity struct {
	SessionID string `json:"sessionId"`
}

// LifecycleFacts records the stable Factory Session lifecycle state. Phase is
// absent when the customer interface does not declare a current phase.
type LifecycleFacts struct {
	Status string  `json:"status"`
	Phase  *string `json:"phase,omitempty"`
}

// HashFacts records source and policy facts that identify the session's
// declared execution inputs. Requested and effective policy hashes are absent
// when the corresponding policy was not declared by the customer interface.
type HashFacts struct {
	SourceHash          string  `json:"sourceHash"`
	RequestedPolicyHash *string `json:"requestedPolicyHash,omitempty"`
	EffectivePolicyHash *string `json:"effectivePolicyHash,omitempty"`
}

// ProgressFacts records the declared dispatch counts at observation time.
type ProgressFacts struct {
	TotalDispatches     int `json:"totalDispatches"`
	CompletedDispatches int `json:"completedDispatches"`
	FailedDispatches    int `json:"failedDispatches"`
	InFlightDispatches  int `json:"inFlightDispatches"`
}

// DispatchFact records one customer-visible dispatch. Dispatches retain their
// customer-visible list order; Order is the one-based position in that list.
type DispatchFact struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
	Order     int    `json:"order"`
	Status    string `json:"status"`
	Kind      string `json:"kind"`
}

// ArtifactFact records one customer-visible artifact. Artifacts retain their
// customer-visible list order; Order is the one-based position in that list.
type ArtifactFact struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
	Order     int    `json:"order"`
	Kind      string `json:"kind"`
}

// ResultFact records one customer-visible session result. Results retain their
// customer-visible list order; Order is the one-based position in that list.
type ResultFact struct {
	SessionID string `json:"sessionId"`
	ID        string `json:"id"`
	Order     int    `json:"order"`
	Status    string `json:"status"`
	Value     string `json:"value"`
}

// FailureFact records one stable failure declared for the session. DispatchID
// is absent for a session-level failure. Failures retain their customer-visible
// list order; Order is the one-based position in that list.
type FailureFact struct {
	SessionID  string  `json:"sessionId"`
	ID         string  `json:"id"`
	Order      int     `json:"order"`
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	DispatchID *string `json:"dispatchId,omitempty"`
}

// FactoryEventCursor identifies an ordered canonical Factory Event. Cursors
// are stored in strictly increasing canonical sequence order, not transport
// delivery order.
type FactoryEventCursor struct {
	SessionID string `json:"sessionId"`
	Cursor    string `json:"cursor"`
	Sequence  int64  `json:"sequence"`
	EventType string `json:"eventType"`
}
