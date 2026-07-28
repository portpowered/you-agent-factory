// Package details owns functional post-lifecycle Provider Session detail inspection
// through the public GET /provider-sessions/detail HTTP surface. Proofs construct
// only through support.StartFunctionalAPIServer (root.BuildProcess +
// Process.Execute) with serviceedges.Edges effect replacement (for example
// ProviderSessionResolveHomeDirectory), wait for runtime lifecycle readiness, then
// assert identity/provider/kind and inspectable transcript or adverse outcomes on
// the public detail response. codex_details_test.go covers Codex golden success,
// missing-transcript not-found, and corrupt-transcript diagnostics;
// cursor_details_test.go covers Cursor golden success, unavailable-blob inspection,
// and missing-session not-found; http_test.go covers HTTP/API golden success,
// raw filesystem path rejection, and unsupported-kind validation.
package details
