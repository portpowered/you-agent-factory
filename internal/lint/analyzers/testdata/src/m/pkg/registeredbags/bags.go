package registeredbags

import child "m/pkg/registeredbagchild"

type Service struct{}
type Record struct{ secretInput *child.Child }
type RecordAlias = Record
type RecordDefined Record
type Embedded struct{ *child.Child }
type Nested struct{ inner Record }
type Recursive struct {
	next  *Recursive
	value int
}
type RecursiveBag struct {
	next  *RecursiveBag
	child *child.Child
}
type Classified struct{ child *child.Child }
type Alias = child.Child
type Defined child.Child // want Defined:"construction-kind=behavior"
type ChildContainer struct{ children child.Slice }
type DefinedContainer struct{ child *Defined }
type Multiple struct{ first, second *child.Child }
type DomainContainer struct{ value child.DomainDerived }

func NewRecord(input Record) *Service                                      { return nil } // want "required-dependency-bag.*NewRecord->.*NewRecord"
func NewAlias(input RecordAlias) *Service                                  { return nil } // want "required-dependency-bag.*NewAlias->.*NewAlias"
func NewDefined(input RecordDefined) *Service                              { return nil } // want "required-dependency-bag.*NewDefined->.*NewDefined"
func NewPointer(input *Record) *Service                                    { return nil } // want "required-dependency-bag.*NewPointer->.*NewPointer"
func NewEmbedded(input Embedded) *Service                                  { return nil } // want "required-dependency-bag.*NewEmbedded->.*NewEmbedded"
func NewNested(input Nested) *Service                                      { return nil } // want "required-dependency-bag.*NewNested->.*NewNested"
func NewRecursiveBag(input RecursiveBag) *Service                          { return nil } // want "required-dependency-bag.*NewRecursiveBag->.*NewRecursiveBag"
func NewSlice(input []Record) *Service                                     { return nil } // want "required-dependency-bag.*NewSlice->.*NewSlice"
func NewArray(input [2]Record) *Service                                    { return nil } // want "required-dependency-bag.*NewArray->.*NewArray"
func NewMap(input map[string]Record) *Service                              { return nil } // want "required-dependency-bag.*NewMap->.*NewMap"
func NewMapKey(input map[*Record]string) *Service                          { return nil } // want "required-dependency-bag.*NewMapKey->.*NewMapKey"
func NewChannel(input chan Record) *Service                                { return nil } // want "required-dependency-bag.*NewChannel->.*NewChannel"
func NewVariadic(input ...Record) *Service                                 { return nil } // want "required-dependency-bag.*NewVariadic->.*NewVariadic"
func NewImported(input child.Bag) *Service                                 { return nil } // want "required-dependency-bag.*NewImported->.*NewImported"
func NewImportedAlias(input child.BagAlias) *Service                       { return nil } // want "required-dependency-bag.*NewImportedAlias->.*NewImportedAlias"
func NewImportedDefined(input child.BagDefined) *Service                   { return nil } // want "required-dependency-bag.*NewImportedDefined->.*NewImportedDefined"
func NewDefinedCollaborator(input DefinedContainer) *Service               { return nil } // want "required-dependency-bag.*NewDefinedCollaborator->.*NewDefinedCollaborator"
func NewContainer(input ChildContainer) *Service                           { return nil } // want "required-dependency-bag.*NewContainer->.*NewContainer"
func NewMultiple(input Multiple) *Service                                  { return nil } // want "required-dependency-bag.*NewMultiple->.*NewMultiple"
func NewInline(input struct{ dep *child.Child }) *Service                  { return nil } // want "required-dependency-bag.*NewInline->.*NewInline"
func NewEffect(input struct{ effect child.Effect }) *Service               { return nil } // want "required-dependency-bag.*NewEffect->.*NewEffect"
func NewFieldMapKey(input struct{ deps map[*child.Child]string }) *Service { return nil } // want "required-dependency-bag.*NewFieldMapKey->.*NewFieldMapKey"
func NewFieldChannel(input struct{ deps chan *child.Child }) *Service      { return nil } // want "required-dependency-bag.*NewFieldChannel->.*NewFieldChannel"
func NewFieldArray(input struct{ deps [2]*child.Child }) *Service          { return nil } // want "required-dependency-bag.*NewFieldArray->.*NewFieldArray"
func NewGrouped(optional int, first, second Record) *Service               { return nil } // want "required-dependency-bag.*NewGrouped->.*NewGrouped"
func NewUnnamed(Record) *Service                                           { return nil } // want "required-dependency-bag.*NewUnnamed->.*NewUnnamed"

func NewDomain(input child.Domain) *Service               { return nil }
func NewState(input child.State) *Service                 { return nil }
func NewResource(input child.Resource) *Service           { return nil }
func NewDomainContainer(input DomainContainer) *Service   { return nil }
func NewClassified(input Classified) *Service             { return nil }
func NewRecursive(input Recursive) *Service               { return nil }
func NewDirect(input *child.Child) *Service               { return nil }
func NewDirectAlias(input *Alias) *Service                { return nil }
func NewDirectDefined(input *child.DerivedAgain) *Service { return nil }
func NewDirectSlice(input child.Slice) *Service           { return nil }
func NewOptional(input Record) *Service                   { return nil }
