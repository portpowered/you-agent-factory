// Package fixtures defines the sanitized L1 V0 ACP conformance-fixture
// shape: a closed corpus of named Cases, each recording the compatibility
// surface (Role) and interaction Direction it exercises, the expected
// Classification, a raw input protocol value, and the raw expected semantic
// output or safe error. The shape carries no acp-go-sdk or transport-package
// types — only strings and encoding/json.RawMessage — so a fixture corpus is
// a plain, portable JSON document: any independent consumer (this package's
// own compatibility tests, or a future provider-side counterpart) can decode
// a Case without importing this package tree's internal implementation, as
// long as it interprets Role/Direction/Classification the same way this file
// documents.
//
// Role is a closed, documented set matching the L1 V0 JSON-RPC method
// surface plus the two derived, non-request compatibility checks this
// transport also verifies:
//
//   - "initialize"                    — an initialize request (Input decodes as acpsdk.InitializeRequest)
//   - "session/new"                   — Input decodes as acpsdk.NewSessionRequest
//   - "session/load"                  — Input decodes as acpsdk.LoadSessionRequest
//   - "session/resume"                — Input decodes as acpsdk.ResumeSessionRequest
//   - "session/cancel"                — Input decodes as acpsdk.CancelNotification
//   - "session/set_config_option"     — Input decodes as acpsdk.SetSessionConfigOptionRequest
//   - "session/prompt"                — Input decodes as acpsdk.PromptRequest
//   - "session/update"                — Input decodes as acpsdk.SessionUpdate
//   - "session/request_permission"    — Input decodes as acpsdk.RequestPermissionRequest
//   - "stop_reason"                   — Input is {"outcome": "<protocol.TerminalOutcome>"}; Direction is always "outbound"
//   - "unsupported_method"            — Input is {"method": "<name>"}; the method is always outside SupportedMethods
//
// For every request Role, Classification "accepted" means Expected is the
// JSON-marshaled semantic value the owning Validate/Negotiate function
// returns; Classification "rejected" means Expected is the JSON-marshaled
// *acpsdk.RequestError the owning compatibility boundary returns instead.
package fixtures

import (
	"encoding/json"
	"fmt"
)

// Direction records which side of the boundary a Case exercises: an inbound
// case is a request this transport receives and validates; an outbound case
// is a result this transport produces on its own (currently only stop-reason
// mapping).
type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

// Classification records whether a Case's input is expected to be accepted
// (producing a semantic value) or rejected (producing a safe protocol
// error).
type Classification string

const (
	ClassificationAccepted Classification = "accepted"
	ClassificationRejected Classification = "rejected"
)

// Role is the closed set of compatibility surfaces a Case can exercise. See
// the package doc comment for the full table and each Role's Input shape.
type Role string

const (
	RoleInitialize               Role = "initialize"
	RoleSessionNew               Role = "session/new"
	RoleSessionLoad              Role = "session/load"
	RoleSessionResume            Role = "session/resume"
	RoleSessionCancel            Role = "session/cancel"
	RoleSessionSetConfigOption   Role = "session/set_config_option"
	RoleSessionPrompt            Role = "session/prompt"
	RoleSessionUpdate            Role = "session/update"
	RoleSessionRequestPermission Role = "session/request_permission"
	RoleStopReason               Role = "stop_reason"
	RoleUnsupportedMethod        Role = "unsupported_method"
)

var validRoles = map[Role]bool{
	RoleInitialize:               true,
	RoleSessionNew:               true,
	RoleSessionLoad:              true,
	RoleSessionResume:            true,
	RoleSessionCancel:            true,
	RoleSessionSetConfigOption:   true,
	RoleSessionPrompt:            true,
	RoleSessionUpdate:            true,
	RoleSessionRequestPermission: true,
	RoleStopReason:               true,
	RoleUnsupportedMethod:        true,
}

var validDirections = map[Direction]bool{
	DirectionInbound:  true,
	DirectionOutbound: true,
}

var validClassifications = map[Classification]bool{
	ClassificationAccepted: true,
	ClassificationRejected: true,
}

// Case is one sanitized L1 V0 ACP conformance fixture.
type Case struct {
	// Name identifies the case within its corpus; it must be unique within
	// that corpus so a failing assertion is unambiguous to locate.
	Name string `json:"name"`
	// Role is the compatibility surface this case exercises.
	Role Role `json:"role"`
	// Direction is which side of the boundary this case exercises.
	Direction Direction `json:"direction"`
	// Classification is the expected outcome: accepted or rejected.
	Classification Classification `json:"classification"`
	// Input is the raw protocol-shaped input value for Role.
	Input json.RawMessage `json:"input"`
	// Expected is the raw expected semantic output (Classification
	// "accepted") or safe protocol error (Classification "rejected").
	Expected json.RawMessage `json:"expected"`
}

// Corpus is a named, committed collection of Cases sharing this package's
// shape.
type Corpus struct {
	Cases []Case `json:"cases"`
}

// Parse decodes and validates a fixture corpus's shape. It fails clearly,
// naming the offending case and field, for any structurally invalid
// fixture — an unparseable document, an empty corpus, a missing or
// duplicate case name, an unrecognized Role/Direction/Classification, or a
// missing/invalid-JSON Input or Expected value — rather than silently
// accepting a partial or malformed shape. It does not evaluate a case's
// semantic behavior; that is the owning compatibility test's job.
func Parse(data []byte) (Corpus, error) {
	var corpus Corpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return Corpus{}, fmt.Errorf("fixtures: decode corpus: %w", err)
	}
	if len(corpus.Cases) == 0 {
		return Corpus{}, fmt.Errorf("fixtures: corpus has no cases")
	}

	seen := make(map[string]bool, len(corpus.Cases))
	for i, c := range corpus.Cases {
		if c.Name == "" {
			return Corpus{}, fmt.Errorf("fixtures: case %d: name is required", i)
		}
		if seen[c.Name] {
			return Corpus{}, fmt.Errorf("fixtures: duplicate case name %q", c.Name)
		}
		seen[c.Name] = true
		if !validRoles[c.Role] {
			return Corpus{}, fmt.Errorf("fixtures: case %q: unknown role %q", c.Name, c.Role)
		}
		if !validDirections[c.Direction] {
			return Corpus{}, fmt.Errorf("fixtures: case %q: unknown direction %q", c.Name, c.Direction)
		}
		if !validClassifications[c.Classification] {
			return Corpus{}, fmt.Errorf("fixtures: case %q: unknown classification %q", c.Name, c.Classification)
		}
		if len(c.Input) == 0 || !json.Valid(c.Input) {
			return Corpus{}, fmt.Errorf("fixtures: case %q: input must be non-empty valid JSON", c.Name)
		}
		if len(c.Expected) == 0 || !json.Valid(c.Expected) {
			return Corpus{}, fmt.Errorf("fixtures: case %q: expected must be non-empty valid JSON", c.Name)
		}
	}
	return corpus, nil
}
