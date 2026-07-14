// Package agypty defines the minimal native Go Agy PTY boundary interfaces,
// pure argv/path helpers, and mock seams approved in
// docs/architecture/agy-pty-interface.md.
//
// Platform PTY allocation uses WindowsConPTYAllocator (ConPTY pseudo-console) and
// POSIXPTYAllocator (openpty master/slave). Unsupported OS builds return
// ErrUnsupportedPlatform and never fall back to pipe IO. Story 002+ implement
// PTYSession.Run capture, timeout, and cleanup. Tests substitute MockAllocator or
// inject mock openers for hermetic coverage without an installed Agy binary.
//
// The package does not invoke or embed the upstream Python bridge.
//
// Related documents:
//   - docs/architecture/agy-pty-boundary.md — ADR scope and gating
//   - docs/architecture/agy-pty-threat-review.md — T1/T2 security controls
package agypty
