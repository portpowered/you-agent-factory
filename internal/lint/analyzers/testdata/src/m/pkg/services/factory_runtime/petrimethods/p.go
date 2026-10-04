package petrimethods

import "m/pkg/services/factory_runtime/internal/orchestrators/petri"

type Recursive struct { // want "exact key `petri-public\\|.*\\|Recursive\\|.*petri.Marking`"
	Next   *Recursive
	First  petri.Marking
	Second []petri.Marking
}
type Pointer struct{}                // want "exact key `petri-public\\|.*\\|Pointer\\|.*petri.Token`"
func (*Pointer) Observe(petri.Token) {}

type Value struct{}                 // want "exact key `petri-public\\|.*\\|Value\\|.*petri.Marking`"
func (Value) Observe(petri.Marking) {}

type hidden struct{}

func (*hidden) Observe(petri.Token) {}

type Promoted struct{ *hidden } // want "exact key `petri-public\\|.*\\|Promoted\\|.*petri.Token`"
type callback interface{ Observe(petri.Marking) }
type Embedded interface { // want "exact key `petri-public\\|.*\\|Embedded\\|.*petri.Marking`"
	callback
}
type SecondRoot = Recursive // want "exact key `petri-public\\|.*\\|SecondRoot\\|.*petri.Marking`"
type Harmless struct{}

func (Harmless) private(petri.Token) {}
