// Package agypty defines the minimal native Go Agy PTY boundary interfaces,
// pure argv/path helpers, and mock seams approved in
// docs/architecture/agy-pty-interface.md.
//
// This package is specification-only for Story 16 (stream-b06-agy-integration-decision).
// Production Agy headless execution remains out of scope until Story 17 consumes
// these interfaces. The package does not invoke or embed the upstream Python bridge
// and does not require an installed Agy binary for unit tests.
//
// Platform PTY allocation (Windows ConPTY, POSIX openpty) is implemented in Story 17.
// Tests substitute MockAllocator for real ConPTY/PTY allocation.
//
// Related documents:
//   - docs/architecture/agy-pty-boundary.md — ADR scope and gating
//   - docs/architecture/agy-pty-threat-review.md — T1/T2 security controls
package agypty
