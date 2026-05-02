package petri

import "github.com/portpowered/infinite-you/pkg/interfaces"

// Arc connects a Place to a Transition (input) or a Transition to a Place (output).
type Arc struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"` // binding name — guards on other arcs reference this (e.g., "work", "design")
	PlaceID     string             `json:"place_id"`
	Direction   ArcDirection       `json:"direction"`   // INPUT or OUTPUT
	Mode        interfaces.ArcMode `json:"mode"`        // CONSUME or OBSERVE (input arcs only; ignored for output arcs)
	Guard       Guard              `json:"-"`           // nil for unconditional arcs; receives bindings from other named arcs
	Cardinality ArcCardinality     `json:"cardinality"` // ONE, ALL, or a specific count
	// Input/output
	TransitionID string `json:"transition_id"`
}

// ArcDirection indicates whether an arc is an input (place → transition) or output (transition → place).
type ArcDirection int

const (
	ArcInput  ArcDirection = iota // place → transition
	ArcOutput                     // transition → place
)

// ArcCardinality defines how many tokens an arc consumes/produces.
type ArcCardinality struct {
	Mode  CardinalityMode `json:"mode"`  // ONE, ALL, ALL_TERMINAL, or N
	Count int             `json:"count"` // only used when Mode == CardinalityN
}

// CardinalityMode specifies the cardinality strategy for an arc.
type CardinalityMode int

const (
	CardinalityOne         CardinalityMode = iota // consumes exactly one token
	CardinalityAll                                // consumes all tokens matching the guard in a single place
	CardinalityAllTerminal                        // consumes all tokens across all terminal-category places for the work type
	CardinalityN                                  // consumes exactly N tokens
	CardinalityZeroOrMore                         // consumes zero or more tokens — used by dynamic fanout where count is validated by guard
)
