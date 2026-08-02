// Package acp is the inert Agent Client Protocol (ACP) agent-side
// compatibility boundary. It represents the ACP protocol version,
// capability, configuration-option, and request-identity values understood
// by the pinned github.com/coder/acp-go-sdk v0.13.5 dependency.
//
// Every value in this package is pure and deterministic: construction,
// validation, and serialization perform no IO, start no goroutine or
// process, bind no stdio or network endpoint, and create or invoke no Chat
// Session or Factory Session. Stdio framing, a running JSON-RPC server, and
// session execution are later work; this package only describes what a
// future server would truthfully be allowed to claim.
package acp
