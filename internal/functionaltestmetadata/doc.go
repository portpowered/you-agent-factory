// Package functionaltestmetadata inventories top-level Test* declarations under
// a functional-test source tree using the Go AST.
//
// Later foundation cells consume this package for catalog generation, build-tag
// and golden labeling, customer-versus-harness classification, and the
// undocumented-test baseline. This package stays side-effect free aside from
// reading source files.
package functionaltestmetadata
