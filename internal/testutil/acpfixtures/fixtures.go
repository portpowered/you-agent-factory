// Package acpfixtures defines the sanitized L1 V0 ACP conformance-fixture
// shape and its committed corpus: a closed set of named Cases, each
// recording the compatibility surface (Role) and interaction Direction it
// exercises, the expected Classification, a raw input protocol value, and
// the raw expected semantic output or safe error. The shape carries no
// acp-go-sdk or transport-package types — only strings and
// encoding/json.RawMessage — so a fixture corpus is a plain, portable JSON
// document.
//
// This package lives under the repo-root internal/testutil tree, not under
// pkg/transports/acp/internal, specifically so both the outbound
// pkg/transports/acp compatibility boundary and the inbound
// pkg/services/providers ACP mapper can import it directly: Go's
// internal/ visibility rule only allows a package rooted at
// pkg/transports/acp/internal to be imported from within the
// pkg/transports/acp tree, which would make a fixture corpus placed there
// unreachable from pkg/services/providers. Both directions decode the same
// committed Cases and independently assert their own semantics against the
// same Input; neither imports the other's production package.
//
// Role is a closed, documented set matching the L1 V0 JSON-RPC method
// surface plus the two derived, non-request compatibility checks this
// transport also verifies. For every JSON-RPC method Role, Input is a
// complete JSON-RPC 2.0 message -- {"jsonrpc":"2.0","id":...,"method":...,
// "params":...} for a request method, or the same shape with no "id" member
// for a notification method -- not a bare params object: this lets a
// consumer decode Input through the same envelope-level identity binding
// (method + request/connection identity + params, all bound together) the
// real compatibility boundary uses, proving the combined round trip rather
// than only the isolated params-to-value mapping.
//
//   - "initialize"                    — a request; params decode as acpsdk.InitializeRequest
//   - "session/new"                   — a request; params decode as acpsdk.NewSessionRequest
//   - "session/load"                  — a request; params decode as acpsdk.LoadSessionRequest
//   - "session/resume"                — a request; params decode as acpsdk.ResumeSessionRequest
//   - "session/cancel"                — a notification (no "id"); params decode as acpsdk.CancelNotification
//   - "session/set_config_option"     — a request; params decode as acpsdk.SetSessionConfigOptionRequest
//   - "session/prompt"                — a request; params decode as acpsdk.PromptRequest
//   - "session/update"                — a notification (no "id"); params decode as acpsdk.SessionNotification
//   - "session/request_permission"    — a request; params decode as acpsdk.RequestPermissionRequest
//   - "stop_reason"                   — Input is {"outcome": "<protocol.TerminalOutcome>"}, not a JSON-RPC message; Direction is always "outbound"
//   - "unsupported_method"            — Input is {"method": "<name>"}, not a JSON-RPC message; the method is always outside SupportedMethods
//
// For every request/notification Role, Classification "accepted" means
// Expected is the JSON-marshaled semantic value the owning
// Validate/Negotiate function returns; Classification "rejected" means
// Expected is the JSON-marshaled *acpsdk.RequestError the owning
// compatibility boundary returns instead.
package acpfixtures

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
)

//go:embed testdata/*.json
var embeddedCorpora embed.FS

// LoadAll parses every committed fixture corpus under testdata/. It fails
// clearly if no corpus is found or any committed corpus does not match the
// Case shape; it never asserts anything about which files exist beyond the
// embed glob itself.
func LoadAll() ([]Corpus, error) {
	entries, err := fs.Glob(embeddedCorpora, "testdata/*.json")
	if err != nil {
		return nil, fmt.Errorf("acpfixtures: glob embedded corpora: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("acpfixtures: no fixture corpora found under testdata")
	}
	corpora := make([]Corpus, 0, len(entries))
	for _, path := range entries {
		data, err := embeddedCorpora.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("acpfixtures: read %s: %w", path, err)
		}
		corpus, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("acpfixtures: parse %s: %w", path, err)
		}
		corpora = append(corpora, corpus)
	}
	return corpora, nil
}

// CasesByRole returns every committed Case across every corpus whose Role
// matches role, preserving corpus order.
func CasesByRole(role Role) ([]Case, error) {
	corpora, err := LoadAll()
	if err != nil {
		return nil, err
	}
	var out []Case
	for _, corpus := range corpora {
		for _, c := range corpus.Cases {
			if c.Role == role {
				out = append(out, c)
			}
		}
	}
	return out, nil
}

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
