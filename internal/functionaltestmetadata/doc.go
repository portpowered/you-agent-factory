// Package functionaltestmetadata inventories top-level Test* declarations under
// a functional-test source tree using the Go AST.
//
// Records include file-level build-constraint expressions and explicit golden
// fixture/manifest references when declared. Golden references are read from:
//   - a //golden: <path> directive in the test's doc comment, or
//   - a test-owned const/var string named golden, goldenManifest, goldenFixture,
//     Golden, GoldenManifest, or GoldenFixture inside the Test* body.
//
// Later foundation cells consume this package for catalog generation, build-tag
// and golden labeling, customer-versus-harness classification, and the
// undocumented-test baseline. This package stays side-effect free aside from
// reading source files.
package functionaltestmetadata
