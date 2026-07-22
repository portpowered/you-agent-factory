package petri

// Place represents a location in the net where tokens reside.
// In factory terms, a Place is a (TypeID, State) pair where TypeID groups
// related places (e.g. a work type or any other logical grouping).
// e.g. Place{TypeID: "page", State: "init"} or Place{TypeID: "gpu", State: "available"}
type Place struct {
	ID     string `json:"id"`
	TypeID string `json:"type_id"` // logical grouping ID (e.g. work-type name)
	State  string `json:"state"`   // state value within that type
}
