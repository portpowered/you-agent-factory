// Package association owns functional post-lifecycle Provider Session ref
// correlation on public Factory Session dispatch projections. Proofs construct
// only through support.StartFunctionalAPIServer (root.BuildProcess +
// Process.Execute) with serviceedges.Edges effect replacement, wait for runtime
// lifecycle readiness, then assert providerSessionRefs on public dispatch list
// and detail surfaces join the owning dispatch id and Factory Session sessionId.
// association_test.go covers present-ref, absent-ref non-fabrication, and
// distinct multi-dispatch correlation; response_exec_metadata_test.go covers
// golden metadata survival across CLI, API response events, and replay.
package association
