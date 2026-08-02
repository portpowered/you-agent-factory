// Package factorysessionsshim is the Chat Sessions-owned adapter over the
// public factory_sessions.Service root. It exposes only the Factory-target
// start, invoke, cancel, and close capabilities Chat Sessions needs and
// delegates each call exactly once, unmodified, to the injected root. It is
// registered under "Shims registered for deletion" in
// docs/internal/projects/root-consolidation/proposal.md, retired at L3
// Factory Sessions sealing; it must not grow into a second session
// authority.
package factorysessionsshim
