package registeredbagchild

type Child struct{ injected *Child }
type Effect struct{}
type Domain struct{ injected *Child }
type State struct{ injected *Child }
type Resource struct{ injected *Child }
type Derived Child        // want Derived:"construction-kind=behavior"
type DerivedAgain Derived // want DerivedAgain:"construction-kind=behavior"
type DomainDerived Domain // want DomainDerived:"construction-kind=domain-data"
type ChildAlias = Child
type Bag struct{ child *DerivedAgain }
type BagAlias = Bag
type BagDefined Bag
type Slice []*Child
