package acp

import (
	"bytes"
	"encoding/json"

	acpsdk "github.com/coder/acp-go-sdk"
)

// MethodDisposition classifies a JSON-RPC method name against this V0
// boundary's method inventory. Every disposition other than a method this
// boundary implements resolves identically on the wire -- method not found
// -- because stdio serving and session execution are later work; the
// distinction between "deferred" and "unknown" exists for inventory and test
// coverage, not differing wire behavior.
type MethodDisposition string

const (
	// MethodDispositionDeferred marks a method the ACP protocol/SDK defines
	// that this V0 boundary does not yet implement.
	MethodDispositionDeferred MethodDisposition = "deferred"
	// MethodDispositionUnknown marks a method this V0 boundary does not
	// recognize at all.
	MethodDispositionUnknown MethodDisposition = "unknown"
)

// deferredMethods lists ACP methods defined by the protocol/SDK that this
// inert V0 boundary does not implement: stdio serving and session execution
// are later work.
var deferredMethods = map[string]struct{}{
	"initialize":                 {},
	"session/new":                {},
	"session/load":               {},
	"session/prompt":             {},
	"session/cancel":             {},
	"session/set_config_option":  {},
	"session/request_permission": {},
	"session/set_mode":           {},
	"authenticate":               {},
}

// ClassifyMethod reports whether method is a V0-deferred ACP operation or
// genuinely unknown to the protocol. It performs no IO and has no side
// effect.
func ClassifyMethod(method string) MethodDisposition {
	if _, ok := deferredMethods[method]; ok {
		return MethodDispositionDeferred
	}
	return MethodDispositionUnknown
}

// UnsupportedMethodOutcome is the correlated, side-effect-free result of
// dispatching a JSON-RPC request whose method this V0 boundary does not
// implement, whether genuinely unknown or explicitly deferred.
type UnsupportedMethodOutcome struct {
	ResponseID acpsdk.RequestId
	Error      *acpsdk.RequestError
}

// UnsupportedMethodResponse builds the method-not-found outcome for a
// method this V0 boundary does not implement. wireID is the original
// decoded JSON-RPC request id; a nil wireID represents a notification (no
// id present), for which UnsupportedMethodResponse returns nil and the
// caller must emit no response at all. UnsupportedMethodResponse invokes no
// session, provider, process, filesystem, network, or persistence
// collaborator.
func UnsupportedMethodResponse(method string, wireID *acpsdk.RequestId) *UnsupportedMethodOutcome {
	if wireID == nil {
		return nil
	}
	return &UnsupportedMethodOutcome{
		ResponseID: *wireID,
		Error:      acpsdk.NewMethodNotFound(method),
	}
}

// DecodeMethodParams unmarshals raw JSON-RPC params into T and returns a
// typed, sensitive-safe invalid-params outcome on missing or malformed
// input, so parameter validation never produces a partially decoded value.
// The returned *acpsdk.RequestError never includes the raw params content.
func DecodeMethodParams[T any](raw json.RawMessage) (T, *acpsdk.RequestError) {
	var zero T
	if len(bytes.TrimSpace(raw)) == 0 {
		return zero, acpsdk.NewInvalidParams(map[string]any{"reason": "missing params"})
	}
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return zero, acpsdk.NewInvalidParams(map[string]any{"reason": "malformed params"})
	}
	return v, nil
}
