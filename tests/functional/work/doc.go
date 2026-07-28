// Package work owns functional root.BuildProcess evidence for packaged Work
// submission, routing, relationships, recovery, recordings-read, CLI submit
// contracts, and visualization/dependency-graph surfaces: construction stays
// inert until runtime lifecycle starts, then activates through public Work
// protocol surfaces composed only via root.BuildProcess and edges.Edges.
// Proofs import only published Work contracts plus shared functional support;
// peer_import_boundary_test.go seals forbidden work/internal, deleted
// transitional Work packages, and legacy pkg/work* consumer imports.
package work
