package petrifixture

import "m/pkg/services/factory_runtime/internal/orchestrators/petri"

type Alias = petri.Marking // want "exact key `petri-public\\|.*\\|Alias\\|.*petri.Marking`"
type Nested struct {       // want "exact key `petri-public\\|.*\\|Nested\\|.*petri.Marking`" "exact key `petri-public\\|.*\\|Nested\\|.*petri.Token`"
	Items [2][]map[*petri.Token]chan func(petri.Marking) *petri.Token
}

func Function(petri.Marking) []petri.Token { return nil } // want "exact key `petri-public\\|.*\\|Function\\|.*petri.Marking`" "exact key `petri-public\\|.*\\|Function\\|.*petri.Token`"
var Variable func() *petri.Net             // want "exact key `petri-public\\|.*\\|Variable\\|.*petri.Net`"
const Constant petri.Color = "red"         // want "exact key `petri-public\\|.*\\|Constant\\|.*petri.Color`"
type Constraint interface {                // want "exact key `petri-public\\|.*\\|Constraint\\|.*petri.Color`"
	petri.Color | int
}
type Generic[T Constraint] struct{ Value T } // want "exact key `petri-public\\|.*\\|Generic\\|.*petri.Color`"
func GenericFunction[T Constraint](v T) T    { return v } // want "exact key `petri-public\\|.*\\|GenericFunction\\|.*petri.Color`"
type box[T any] struct{ Value T }

type phantom[T any] struct{}
type ArgumentOnly = phantom[petri.Token] // want "exact key `petri-public\\|.*\\|ArgumentOnly\\|.*petri.Token`"
type Instance = box[petri.Token]         // want "exact key `petri-public\\|.*\\|Instance\\|.*petri.Token`"
type GenericAlias[T Constraint] = box[T] // want "exact key `petri-public\\|.*\\|GenericAlias\\|.*petri.Color`"
type private struct{ hidden petri.Marking }

var HiddenField private // want "exact key `petri-public\\|.*\\|HiddenField\\|.*petri.Marking`"
