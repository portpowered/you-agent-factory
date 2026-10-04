package registeredgetterparity

import owner "m/pkg/registeredgetters"

func Named(s *owner.Service) {
	_ = s.NamedParenthesized()    // want "unresolved-service-getter-locator.*Service.*NamedParenthesized"
	_ = s.NamedChain()            // want "unresolved-service-getter-locator.*Service.*NamedChain"
	_ = s.NamedDeclared()         // want "unresolved-service-getter-locator.*Service.*NamedDeclared"
	_ = s.NamedGrouped()          // want "unresolved-service-getter-locator.*Service.*NamedGrouped"
	_ = s.NamedLaterAlias()       // want "unresolved-service-getter-locator.*Service.*NamedLaterAlias"
	_ = s.NamedHelper()           // want "unresolved-service-getter-locator.*Service.*NamedHelper"
	_, _ = s.NamedCrossResult()   // want "unresolved-service-getter-locator.*Service.*NamedCrossResult"
	_ = s.NamedClosure()          // want "unresolved-service-getter-locator.*Service.*NamedClosure"
	_ = s.NamedCycle()            // want "unresolved-service-getter-locator.*Service.*NamedCycle"
	_, _ = s.NamedGroupedResult() // want "unresolved-service-getter-locator.*Service.*NamedGroupedResult"
	_, _ = s.NamedParallel()      // want "unresolved-service-getter-locator.*Service.*NamedParallel"
	_, _ = s.NamedTuple()         // want "unresolved-service-getter-locator.*Service.*NamedTuple"
	_ = s.NamedConflict()         // want "unresolved-service-getter-locator.*Service.*NamedConflict"
	_, _ = s.DomainSlot()
	_, _ = s.NamedDomainSlot()
	_ = s.NamedOptional()
	_ = s.NamedShadow()
	_ = s.NamedUnused()
	_ = s.NamedZero()
}

type Empty interface{ any }
type Failure interface{ error }
type Inline interface{ interface{ Observe() } }
type Alias = Inline
type Defined Inline
type Nested interface{ Inline }
type EmptyView struct {
	*owner.Service
	Empty
}
type FailureView struct {
	*owner.Service
	Failure
}
type InlineView struct {
	*owner.Service
	Inline
}
type AliasView struct {
	*owner.Service
	Alias
}
type DefinedView struct {
	*owner.Service
	Defined
}
type NestedView struct {
	*owner.Service
	Nested
}
type Inner struct{ *owner.Service }
type DeeperView struct {
	Inner
	Empty
}
type Getter interface{ Lookup() owner.Port }
type ShadowedView struct {
	Inner
	Getter
}
type FieldView struct {
	*owner.Service
	Lookup func() owner.Port
}

func EmptySibling(empty *EmptyView)       { _ = empty.Lookup() }   // want "service-getter-locator.*Service.*Lookup"
func FailureSibling(failure *FailureView) { _ = failure.Lookup() } // want "service-getter-locator.*Service.*Lookup"
func InlineSibling(inline *InlineView)    { _ = inline.Lookup() }  // want "service-getter-locator.*Service.*Lookup"
func AliasSibling(alias *AliasView)       { _ = alias.Lookup() }   // want "service-getter-locator.*Service.*Lookup"
func DefinedSibling(defined *DefinedView) { _ = defined.Lookup() } // want "service-getter-locator.*Service.*Lookup"
func NestedSibling(nested *NestedView)    { _ = nested.Lookup() }  // want "service-getter-locator.*Service.*Lookup"
func DeeperSibling(deeper *DeeperView)    { _ = deeper.Lookup() }  // want "service-getter-locator.*Service.*Lookup"
func Shadowed(shadowed *ShadowedView, field *FieldView, contract Getter) {
	_ = shadowed.Lookup()
	_ = field.Lookup()
	_ = contract.Lookup()
}
func MethodValue(view *EmptyView) {
	lookup := view.Lookup
	_ = lookup() // want "service-getter-locator.*Service.*Lookup"
}
func MethodExpression(view *FailureView) {
	_ = (*FailureView).Lookup(view) // want "service-getter-locator.*Service.*Lookup"
}
