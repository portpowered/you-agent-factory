// Package acp defines the L1 V0 ACP JSON-RPC compatibility boundary: the
// pinned protocol version, the exact P0 text-first capability profile,
// collision-safe request identity, and the supported session and prompt
// compatibility values that later stdio framing, session mapping, and
// projection work builds on.
//
// Protocol implementation stays internal to this package tree. Only inert,
// transport-facing operations are exported; they perform no I/O, start no
// server, and invoke no downstream service or effect.
package acp

import (
	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/negotiation"
)

// NegotiateInitialization evaluates a client's ACP initialize request
// against the implemented P0 text-first capability profile and returns the
// negotiated result, or a protocol-safe rejection.
//
// A successful result always carries the pinned protocol version and
// advertises exactly the capabilities this transport implements; it never
// claims plans, filesystem callbacks, terminals, fork, authentication,
// client-supplied MCP servers, or other deferred behavior, regardless of
// what the client offers.
func NegotiateInitialization(req acpsdk.InitializeRequest) (acpsdk.InitializeResponse, error) {
	return negotiation.Negotiate(req)
}
