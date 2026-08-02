// Package acpconformance is the shared, sanitized ACP L1 V0 semantic
// conformance corpus and its test-only reader. Both the Providers-owned
// inbound ACP mapper and the pkg/transports/acp outbound compatibility
// boundary consume it independently through this neutral package, so the two
// protocol directions can be checked against the same expectations without
// either importing the other's private implementation or sharing a
// production mapper.
package acpconformance

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"
)

//go:embed corpus.json
var corpusJSON []byte

// Directionality declares which protocol direction(s) a corpus case is
// expected to support.
type Directionality string

const (
	// DirectionRoundTrip marks a case in the lossless round-trip subset:
	// both the inbound provider mapper and the outbound ACP transport are
	// expected to preserve its declared semantic facts.
	DirectionRoundTrip Directionality = "round_trip"
	// DirectionInboundOnly marks a case only the Providers-owned inbound
	// mapper is expected to honor.
	DirectionInboundOnly Directionality = "inbound_only"
	// DirectionOutboundOnly marks a case only the pkg/transports/acp
	// outbound boundary is expected to honor.
	DirectionOutboundOnly Directionality = "outbound_only"
	// DirectionUnsupported marks a case whose payload must be rejected
	// (an unsupported protocol version or method) rather than mapped.
	DirectionUnsupported Directionality = "unsupported"
	// DirectionNoOutput marks a case that is recognized but intentionally
	// produces no response or text output.
	DirectionNoOutput Directionality = "no_output"
)

var validDirectionality = map[Directionality]bool{
	DirectionRoundTrip:    true,
	DirectionInboundOnly:  true,
	DirectionOutboundOnly: true,
	DirectionUnsupported:  true,
	DirectionNoOutput:     true,
}

// ProtocolRole declares which part of the ACP L1 V0 contract a case
// exercises.
type ProtocolRole string

const (
	RoleInitialize        ProtocolRole = "initialize"
	RoleCapabilities      ProtocolRole = "capabilities"
	RoleFactoryOption     ProtocolRole = "factory_option"
	RoleRequestIdentity   ProtocolRole = "request_identity"
	RolePromptContent     ProtocolRole = "prompt_content"
	RoleSessionUpdate     ProtocolRole = "session_update"
	RoleMalformedInput    ProtocolRole = "malformed_input"
	RoleUnsupportedMethod ProtocolRole = "unsupported_method"
)

var validProtocolRole = map[ProtocolRole]bool{
	RoleInitialize:        true,
	RoleCapabilities:      true,
	RoleFactoryOption:     true,
	RoleRequestIdentity:   true,
	RolePromptContent:     true,
	RoleSessionUpdate:     true,
	RoleMalformedInput:    true,
	RoleUnsupportedMethod: true,
}

// roundTripEligibleKinds are the only Facts.Kind values a DirectionRoundTrip
// case may declare. Every other represented kind (tool calls, plans, and
// other progress-only or lifecycle updates) is lossy in at least one
// direction today and must declare a narrower Directionality instead.
var roundTripEligibleKinds = map[string]bool{
	"message":   true,
	"reasoning": true,
	"usage":     true,
	"session":   true,
}

// Facts is the set of expected semantic facts a corpus case declares.
type Facts struct {
	Kind     string            `json:"kind"`
	ItemID   string            `json:"item_id,omitempty"`
	Phase    string            `json:"phase,omitempty"`
	Text     string            `json:"text,omitempty"`
	Outcome  string            `json:"outcome,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func (facts Facts) clone() Facts {
	cloned := facts
	if facts.Metadata != nil {
		cloned.Metadata = make(map[string]string, len(facts.Metadata))
		maps.Copy(cloned.Metadata, facts.Metadata)
	}
	return cloned
}

// Case is one representative, sanitized ACP protocol example together with
// its declared directionality and expected semantic facts.
type Case struct {
	ID             string          `json:"id"`
	ProtocolRole   ProtocolRole    `json:"protocol_role"`
	Description    string          `json:"description"`
	Directionality Directionality  `json:"directionality"`
	Payload        json.RawMessage `json:"payload"`
	Facts          Facts           `json:"facts"`
	Provenance     string          `json:"provenance"`
}

func (c Case) clone() Case {
	cloned := c
	cloned.Payload = append(json.RawMessage(nil), c.Payload...)
	cloned.Facts = c.Facts.clone()
	return cloned
}

// RoundTripEligible reports whether c's Facts.Kind is one the round-trip
// subset is allowed to declare.
func (c Case) RoundTripEligible() bool {
	return roundTripEligibleKinds[c.Facts.Kind]
}

// SourceProvenance pins the corpus to the ACP SDK version its wire shapes
// were authored against.
type SourceProvenance struct {
	SDKModule  string `json:"sdk_module"`
	SDKVersion string `json:"sdk_version"`
	SDKCommit  string `json:"sdk_commit"`
	SDKLicense string `json:"sdk_license"`
}

// Corpus is the parsed, validated set of shared ACP conformance cases.
type Corpus struct {
	SchemaVersion int              `json:"schema_version"`
	Provenance    SourceProvenance `json:"provenance"`
	// CasesChecksum is a hex-encoded SHA-256 digest over the deterministic
	// JSON encoding of Cases. It never includes itself, so it cannot
	// self-reference. ParseCorpus recomputes it with ChecksumCases and
	// rejects the corpus if the declared and recomputed values differ,
	// which makes any change to case content -- reviewed or not -- require
	// an explicit, matching checksum update.
	CasesChecksum string `json:"cases_checksum"`
	Cases         []Case `json:"cases"`
}

const supportedSchemaVersion = 1

// ChecksumCases returns the hex-encoded SHA-256 digest of cases' canonical
// JSON encoding. It is deterministic for a given slice of cases (field
// order is fixed by the Case struct, and map[string]string keys sort
// alphabetically under encoding/json), so it is reproducible by any caller
// re-deriving it from the same case content, and is the value ParseCorpus
// compares a corpus's declared cases_checksum against.
func ChecksumCases(cases []Case) (string, error) {
	encoded, err := json.Marshal(cases)
	if err != nil {
		return "", fmt.Errorf("acpconformance: encoding cases for checksum: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// ParseCorpus decodes and validates raw corpus JSON. It rejects invalid
// JSON, an unpinned or incomplete source provenance, a missing or mismatched
// cases_checksum, duplicate case identity, an unknown protocol role,
// undeclared or unknown directionality, a case with no declared expected
// facts, and a round-trip declaration for a Facts.Kind known to be lossy or
// unsupported in at least one direction.
func ParseCorpus(data []byte) (Corpus, error) {
	var corpus Corpus
	if err := json.Unmarshal(data, &corpus); err != nil {
		return Corpus{}, fmt.Errorf("acpconformance: invalid corpus JSON: %w", err)
	}
	if corpus.SchemaVersion != supportedSchemaVersion {
		return Corpus{}, fmt.Errorf("acpconformance: unsupported corpus schema_version %d", corpus.SchemaVersion)
	}
	if corpus.Provenance.SDKModule == "" || corpus.Provenance.SDKVersion == "" ||
		corpus.Provenance.SDKCommit == "" || corpus.Provenance.SDKLicense == "" {
		return Corpus{}, fmt.Errorf("acpconformance: corpus source provenance is incomplete")
	}
	if len(corpus.Cases) == 0 {
		return Corpus{}, fmt.Errorf("acpconformance: corpus has no cases")
	}
	if strings.TrimSpace(corpus.CasesChecksum) == "" {
		return Corpus{}, fmt.Errorf("acpconformance: corpus is missing cases_checksum")
	}
	computedChecksum, err := ChecksumCases(corpus.Cases)
	if err != nil {
		return Corpus{}, err
	}
	if !strings.EqualFold(corpus.CasesChecksum, computedChecksum) {
		return Corpus{}, fmt.Errorf(
			"acpconformance: cases_checksum mismatch: corpus declares %q, computed %q from case content -- update cases_checksum after reviewing the case changes",
			corpus.CasesChecksum, computedChecksum,
		)
	}
	seen := make(map[string]bool, len(corpus.Cases))
	for _, c := range corpus.Cases {
		if strings.TrimSpace(c.ID) == "" {
			return Corpus{}, fmt.Errorf("acpconformance: case has empty id")
		}
		if seen[c.ID] {
			return Corpus{}, fmt.Errorf("acpconformance: duplicate case id %q", c.ID)
		}
		seen[c.ID] = true
		if !validProtocolRole[c.ProtocolRole] {
			return Corpus{}, fmt.Errorf("acpconformance: case %q has unknown protocol_role %q", c.ID, c.ProtocolRole)
		}
		if c.Directionality == "" || !validDirectionality[c.Directionality] {
			return Corpus{}, fmt.Errorf("acpconformance: case %q has undeclared directionality %q", c.ID, c.Directionality)
		}
		if len(c.Payload) == 0 || !json.Valid(c.Payload) {
			return Corpus{}, fmt.Errorf("acpconformance: case %q has invalid or empty payload JSON", c.ID)
		}
		if strings.TrimSpace(c.Facts.Kind) == "" {
			return Corpus{}, fmt.Errorf("acpconformance: case %q is missing expected facts", c.ID)
		}
		if strings.TrimSpace(c.Provenance) == "" {
			return Corpus{}, fmt.Errorf("acpconformance: case %q is missing provenance", c.ID)
		}
		if c.Directionality == DirectionRoundTrip && !c.RoundTripEligible() {
			return Corpus{}, fmt.Errorf(
				"acpconformance: case %q declares round_trip for a lossy or unsupported kind %q", c.ID, c.Facts.Kind,
			)
		}
	}
	return corpus, nil
}

// Load parses and validates the embedded shared corpus.
func Load() (Corpus, error) {
	return ParseCorpus(corpusJSON)
}

// MustLoad loads the embedded shared corpus or fails tb. Every returned Case
// is a detached copy: callers may mutate them without affecting other
// callers or the embedded source.
func MustLoad(tb testing.TB) Corpus {
	tb.Helper()
	corpus, err := Load()
	if err != nil {
		tb.Fatalf("acpconformance: %v", err)
	}
	return corpus.clone()
}

func (corpus Corpus) clone() Corpus {
	cloned := corpus
	cloned.Cases = make([]Case, len(corpus.Cases))
	for i, c := range corpus.Cases {
		cloned.Cases[i] = c.clone()
	}
	return cloned
}

// ByRole returns detached copies of every case with the given ProtocolRole.
func (corpus Corpus) ByRole(role ProtocolRole) []Case {
	var out []Case
	for _, c := range corpus.Cases {
		if c.ProtocolRole == role {
			out = append(out, c.clone())
		}
	}
	return out
}

// ByDirectionality returns detached copies of every case with the given
// Directionality.
func (corpus Corpus) ByDirectionality(direction Directionality) []Case {
	var out []Case
	for _, c := range corpus.Cases {
		if c.Directionality == direction {
			out = append(out, c.clone())
		}
	}
	return out
}

// RoundTripCases returns detached copies of every case in the lossless
// round-trip subset.
func (corpus Corpus) RoundTripCases() []Case {
	return corpus.ByDirectionality(DirectionRoundTrip)
}
