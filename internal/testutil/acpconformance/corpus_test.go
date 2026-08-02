package acpconformance_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpconformance"
)

// corpusFixture mirrors acpconformance.Corpus's JSON shape for building test
// fixtures with an explicit, independently-computed cases_checksum.
type corpusFixture struct {
	SchemaVersion int                             `json:"schema_version"`
	Provenance    acpconformance.SourceProvenance `json:"provenance"`
	CasesChecksum string                          `json:"cases_checksum"`
	Cases         []acpconformance.Case           `json:"cases"`
}

// marshalFixture builds a corpus fixture with a correct cases_checksum
// computed from cases via the same acpconformance.ChecksumCases function
// ParseCorpus itself uses, so every fixture below stays valid on that axis
// unless a test deliberately corrupts the checksum field afterward.
func marshalFixture(t testing.TB, provenance acpconformance.SourceProvenance, cases []acpconformance.Case) []byte {
	t.Helper()
	checksum, err := acpconformance.ChecksumCases(cases)
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	data, err := json.Marshal(corpusFixture{
		SchemaVersion: 1,
		Provenance:    provenance,
		CasesChecksum: checksum,
		Cases:         cases,
	})
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return data
}

func validMinimalCorpusJSON(t testing.TB) string {
	t.Helper()
	corpus := acpconformance.MustLoad(t)
	if len(corpus.Cases) == 0 {
		t.Fatal("embedded corpus has no cases to derive a minimal fixture from")
	}
	one := corpus.Cases[0]
	return string(marshalFixture(t, corpus.Provenance, []acpconformance.Case{one}))
}

func TestLoadEmbeddedCorpusIsValid(t *testing.T) {
	corpus, err := acpconformance.Load()
	if err != nil {
		t.Fatalf("Load() unexpected error: %v", err)
	}
	if corpus.SchemaVersion != 1 {
		t.Fatalf("SchemaVersion = %d, want 1", corpus.SchemaVersion)
	}
	if corpus.Provenance.SDKModule != "github.com/coder/acp-go-sdk" || corpus.Provenance.SDKVersion != "v0.13.5" {
		t.Fatalf("Provenance = %+v, want the pinned acp-go-sdk v0.13.5 source", corpus.Provenance)
	}
	if len(corpus.Cases) == 0 {
		t.Fatal("Load() returned a corpus with no cases")
	}
}

func TestParseCorpusRejectsInvalidJSON(t *testing.T) {
	_, err := acpconformance.ParseCorpus([]byte("{not valid json"))
	if err == nil {
		t.Fatal("ParseCorpus(invalid JSON) expected error, got nil")
	}
}

func TestParseCorpusRejectsDuplicateCaseID(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	dup := corpus.Cases[0]
	data := marshalFixture(t, corpus.Provenance, []acpconformance.Case{dup, dup})
	_, err := acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(duplicate case id) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate case id") {
		t.Fatalf("ParseCorpus(duplicate case id) error = %v, want it to mention duplicate case id", err)
	}
}

func TestParseCorpusRejectsMissingFacts(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	broken := corpus.Cases[0]
	broken.Facts = acpconformance.Facts{}
	data := marshalFixture(t, corpus.Provenance, []acpconformance.Case{broken})
	_, err := acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(missing facts) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing expected facts") {
		t.Fatalf("ParseCorpus(missing facts) error = %v, want it to mention missing expected facts", err)
	}
}

func TestParseCorpusRejectsUndeclaredDirectionality(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	broken := corpus.Cases[0]
	broken.Directionality = ""
	data := marshalFixture(t, corpus.Provenance, []acpconformance.Case{broken})
	_, err := acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(undeclared directionality) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "undeclared directionality") {
		t.Fatalf("ParseCorpus(undeclared directionality) error = %v, want it to mention undeclared directionality", err)
	}
}

func TestParseCorpusRejectsUnknownDirectionality(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	broken := corpus.Cases[0]
	broken.Directionality = "sideways"
	data := marshalFixture(t, corpus.Provenance, []acpconformance.Case{broken})
	if _, err := acpconformance.ParseCorpus(data); err == nil {
		t.Fatal("ParseCorpus(unknown directionality) expected error, got nil")
	}
}

func TestParseCorpusRejectsRoundTripForLossyKind(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	var toolCase acpconformance.Case
	found := false
	for _, c := range corpus.Cases {
		if c.Facts.Kind == "tool" {
			toolCase = c
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected embedded corpus to contain a case with Facts.Kind == \"tool\" to mutate")
	}
	toolCase.Directionality = acpconformance.DirectionRoundTrip
	data := marshalFixture(t, corpus.Provenance, []acpconformance.Case{toolCase})
	_, err := acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(round_trip declared for lossy kind) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "lossy or unsupported kind") {
		t.Fatalf("ParseCorpus(round_trip for lossy kind) error = %v, want it to mention lossy or unsupported kind", err)
	}
}

func TestParseCorpusRejectsEmptyCorpus(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	data := marshalFixture(t, corpus.Provenance, nil)
	if _, err := acpconformance.ParseCorpus(data); err == nil {
		t.Fatal("ParseCorpus(no cases) expected error, got nil")
	}
}

func TestParseCorpusRejectsIncompleteProvenance(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	data := marshalFixture(t, acpconformance.SourceProvenance{}, corpus.Cases[:1])
	if _, err := acpconformance.ParseCorpus(data); err == nil {
		t.Fatal("ParseCorpus(incomplete provenance) expected error, got nil")
	}
}

func TestParseCorpusRejectsMissingChecksum(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	fixture := corpusFixture{
		SchemaVersion: 1,
		Provenance:    corpus.Provenance,
		CasesChecksum: "",
		Cases:         corpus.Cases[:1],
	}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	_, err = acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(missing cases_checksum) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "missing cases_checksum") {
		t.Fatalf("ParseCorpus(missing cases_checksum) error = %v, want it to mention missing cases_checksum", err)
	}
}

func TestParseCorpusRejectsMismatchedChecksum(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	correct, err := acpconformance.ChecksumCases(corpus.Cases[:1])
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	fixture := corpusFixture{
		SchemaVersion: 1,
		Provenance:    corpus.Provenance,
		CasesChecksum: correct[:len(correct)-1] + "0",
		Cases:         corpus.Cases[:1],
	}
	if fixture.CasesChecksum == correct {
		t.Fatal("test setup bug: corrupted checksum equals the correct checksum")
	}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	_, err = acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(mismatched cases_checksum) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cases_checksum mismatch") {
		t.Fatalf("ParseCorpus(mismatched cases_checksum) error = %v, want it to mention cases_checksum mismatch", err)
	}
}

// TestParseCorpusRejectsUnknownCaseField proves ParseCorpus cannot be
// fooled by the exact loophole a prior review found: cases_checksum is
// computed over the *typed* []Case value, so if unknown fields were merely
// dropped by json.Unmarshal (rather than rejected), adding an unrecognized
// field to a case's raw JSON would leave the previously-computed checksum
// still valid, silently defeating the "checksum makes every case-content
// change explicit" guarantee. The checksum below is deliberately computed
// from the clean (no extra field) case -- exactly what would happen if
// ParseCorpus only checksummed the post-decode typed value -- to prove that
// is not enough: the unknown field itself must cause ParseCorpus to fail.
func TestParseCorpusRejectsUnknownCaseField(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	one := corpus.Cases[0]
	checksum, err := acpconformance.ChecksumCases([]acpconformance.Case{one})
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	caseJSON, err := json.Marshal(one)
	if err != nil {
		t.Fatalf("marshal case: %v", err)
	}
	var caseFields map[string]json.RawMessage
	if err := json.Unmarshal(caseJSON, &caseFields); err != nil {
		t.Fatalf("unmarshal case: %v", err)
	}
	caseFields["unexpected_extra_field"] = json.RawMessage(`"snuck in"`)
	mutatedCase, err := json.Marshal(caseFields)
	if err != nil {
		t.Fatalf("marshal mutated case: %v", err)
	}
	provenanceJSON, err := json.Marshal(corpus.Provenance)
	if err != nil {
		t.Fatalf("marshal provenance: %v", err)
	}
	data := fmt.Appendf(nil,
		`{"schema_version":1,"provenance":%s,"cases_checksum":%q,"cases":[%s]}`,
		provenanceJSON, checksum, mutatedCase,
	)
	_, err = acpconformance.ParseCorpus(data)
	if err == nil {
		t.Fatal("ParseCorpus(case with unknown field, checksum computed as if the field were absent) expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("ParseCorpus(unknown case field) error = %v, want it to mention an unknown field", err)
	}
}

func TestChecksumCasesIsDeterministicAndContentSensitive(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	first, err := acpconformance.ChecksumCases(corpus.Cases)
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	second, err := acpconformance.ChecksumCases(corpus.Cases)
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	if first != second {
		t.Fatalf("ChecksumCases(same cases) = %q then %q, want identical results", first, second)
	}

	mutated := corpus.Cases
	mutated[0].Description = mutated[0].Description + " (mutated)"
	changed, err := acpconformance.ChecksumCases(mutated)
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	if changed == first {
		t.Fatal("ChecksumCases did not change after mutating a case's content")
	}
}

func TestEmbeddedCorpusChecksumIsSelfConsistent(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	if strings.TrimSpace(corpus.CasesChecksum) == "" {
		t.Fatal("embedded corpus has no cases_checksum")
	}
	recomputed, err := acpconformance.ChecksumCases(corpus.Cases)
	if err != nil {
		t.Fatalf("ChecksumCases: %v", err)
	}
	if recomputed != corpus.CasesChecksum {
		t.Fatalf("recomputed cases checksum %q != declared cases_checksum %q", recomputed, corpus.CasesChecksum)
	}
}

func TestMustLoadReturnsDetachedCases(t *testing.T) {
	first := acpconformance.MustLoad(t)
	if len(first.Cases) == 0 {
		t.Fatal("MustLoad() returned no cases")
	}
	original := string(first.Cases[0].Payload)
	first.Cases[0].Payload = json.RawMessage(`{"mutated":true}`)
	first.Cases[0].Facts.Kind = "mutated"

	second := acpconformance.MustLoad(t)
	if string(second.Cases[0].Payload) != original {
		t.Fatalf("mutating one MustLoad() result affected another: got %s, want %s", second.Cases[0].Payload, original)
	}
	if second.Cases[0].Facts.Kind == "mutated" {
		t.Fatal("mutating one MustLoad() result's Facts affected another")
	}
}

func TestCorpusCoversRequiredProtocolRoles(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	required := []acpconformance.ProtocolRole{
		acpconformance.RoleInitialize,
		acpconformance.RoleCapabilities,
		acpconformance.RoleFactoryOption,
		acpconformance.RoleRequestIdentity,
		acpconformance.RolePromptContent,
		acpconformance.RoleSessionUpdate,
		acpconformance.RoleMalformedInput,
		acpconformance.RoleUnsupportedMethod,
	}
	for _, role := range required {
		if len(corpus.ByRole(role)) == 0 {
			t.Errorf("corpus has no case with protocol_role %q", role)
		}
	}
}

func TestCorpusRepresentsEveryDirectionality(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	directions := []acpconformance.Directionality{
		acpconformance.DirectionRoundTrip,
		acpconformance.DirectionInboundOnly,
		acpconformance.DirectionOutboundOnly,
		acpconformance.DirectionUnsupported,
		acpconformance.DirectionNoOutput,
	}
	for _, direction := range directions {
		if len(corpus.ByDirectionality(direction)) == 0 {
			t.Errorf("corpus has no case with directionality %q", direction)
		}
	}
}

func TestRoundTripCasesOnlyDeclareLosslessKinds(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	roundTrip := corpus.RoundTripCases()
	if len(roundTrip) == 0 {
		t.Fatal("expected at least one round_trip case")
	}
	allowed := map[string]bool{"message": true, "reasoning": true, "usage": true, "session": true}
	for _, c := range roundTrip {
		if !allowed[c.Facts.Kind] {
			t.Errorf("round_trip case %q declares disallowed kind %q", c.ID, c.Facts.Kind)
		}
		if !c.RoundTripEligible() {
			t.Errorf("round_trip case %q reports RoundTripEligible() == false", c.ID)
		}
	}
}

func TestLossyKindsAreExcludedFromRoundTripSubset(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	lossy := map[string]bool{"tool": false, "plan": false}
	for _, c := range corpus.Cases {
		if _, tracked := lossy[c.Facts.Kind]; tracked {
			lossy[c.Facts.Kind] = true
			if c.Directionality == acpconformance.DirectionRoundTrip {
				t.Errorf("lossy kind %q (case %q) must not declare round_trip directionality", c.Facts.Kind, c.ID)
			}
		}
	}
	for kind, seen := range lossy {
		if !seen {
			t.Errorf("expected the corpus to contain a %q case to prove it is excluded from the round-trip subset", kind)
		}
	}
}

// TestSessionUpdateCasesParseAsACPSDKSessionUpdate proves every session_update
// case's payload is a real, parseable acp-go-sdk v0.13.5 SessionUpdate, and
// that the declared facts are actually derivable from the decoded value —
// not merely asserted independently of the payload.
func TestSessionUpdateCasesParseAsACPSDKSessionUpdate(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	for _, c := range corpus.ByRole(acpconformance.RoleSessionUpdate) {
		t.Run(c.ID, func(t *testing.T) {
			var update acpsdk.SessionUpdate
			if err := json.Unmarshal(c.Payload, &update); err != nil {
				t.Fatalf("payload does not parse as acpsdk.SessionUpdate: %v", err)
			}
			kind, itemID, text := describeSessionUpdate(t, update)
			if kind != c.Facts.Kind {
				t.Errorf("decoded kind = %q, want Facts.Kind = %q", kind, c.Facts.Kind)
			}
			if c.Facts.ItemID != "" && itemID != c.Facts.ItemID {
				t.Errorf("decoded item id = %q, want Facts.ItemID = %q", itemID, c.Facts.ItemID)
			}
			if c.Facts.Text != "" && text != c.Facts.Text {
				t.Errorf("decoded text = %q, want Facts.Text = %q", text, c.Facts.Text)
			}

			// Round-trip the payload through the SDK type a second time and
			// confirm the same facts are still derivable, proving the
			// declared round-trip subset is stable under marshal/unmarshal.
			reMarshaled, err := json.Marshal(update)
			if err != nil {
				t.Fatalf("re-marshal SessionUpdate: %v", err)
			}
			var reDecoded acpsdk.SessionUpdate
			if err := json.Unmarshal(reMarshaled, &reDecoded); err != nil {
				t.Fatalf("re-unmarshal SessionUpdate: %v", err)
			}
			reKind, reItemID, reText := describeSessionUpdate(t, reDecoded)
			if c.Directionality == acpconformance.DirectionRoundTrip {
				if reKind != kind || reItemID != itemID || reText != text {
					t.Errorf(
						"round_trip case %q lost facts across a second marshal/unmarshal: got (%q,%q,%q), want (%q,%q,%q)",
						c.ID, reKind, reItemID, reText, kind, itemID, text,
					)
				}
			}
		})
	}
}

// describeSessionUpdate extracts a minimal (kind, itemID, text) tuple
// directly from a decoded acpsdk.SessionUpdate. It intentionally duplicates
// none of the Providers-owned mapper's policy (metadata shaping, progress
// phases beyond delta/started/updated); it exists only to prove this
// corpus's own declared facts are self-consistent with its payload.
func describeSessionUpdate(t testing.TB, update acpsdk.SessionUpdate) (kind, itemID, text string) {
	t.Helper()
	switch {
	case update.AgentMessageChunk != nil:
		id := ""
		if update.AgentMessageChunk.MessageId != nil {
			id = *update.AgentMessageChunk.MessageId
		}
		txt := ""
		if update.AgentMessageChunk.Content.Text != nil {
			txt = update.AgentMessageChunk.Content.Text.Text
		}
		return "message", id, txt
	case update.AgentThoughtChunk != nil:
		id := ""
		if update.AgentThoughtChunk.MessageId != nil {
			id = *update.AgentThoughtChunk.MessageId
		}
		txt := ""
		if update.AgentThoughtChunk.Content.Text != nil {
			txt = update.AgentThoughtChunk.Content.Text.Text
		}
		return "reasoning", id, txt
	case update.UsageUpdate != nil:
		return "usage", "usage", ""
	case update.SessionInfoUpdate != nil:
		txt := ""
		if update.SessionInfoUpdate.Title != nil {
			txt = *update.SessionInfoUpdate.Title
		}
		return "session", "session", txt
	case update.ToolCall != nil:
		return "tool", string(update.ToolCall.ToolCallId), ""
	case update.Plan != nil:
		return "plan", "plan", ""
	default:
		t.Fatal("describeSessionUpdate: no recognized SessionUpdate variant is set")
		return "", "", ""
	}
}

func TestInitializeCasesParseAsACPSDKInitializeRequest(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	for _, c := range corpus.ByRole(acpconformance.RoleInitialize) {
		var req acpsdk.InitializeRequest
		if err := json.Unmarshal(c.Payload, &req); err != nil {
			t.Errorf("case %q payload does not parse as acpsdk.InitializeRequest: %v", c.ID, err)
		}
	}
}

func TestCapabilitiesCasesParseAsACPSDKAgentCapabilities(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	for _, c := range corpus.ByRole(acpconformance.RoleCapabilities) {
		var capabilities acpsdk.AgentCapabilities
		if err := json.Unmarshal(c.Payload, &capabilities); err != nil {
			t.Errorf("case %q payload does not parse as acpsdk.AgentCapabilities: %v", c.ID, err)
		}
		if capabilities.PromptCapabilities.Image || capabilities.PromptCapabilities.Audio || capabilities.PromptCapabilities.EmbeddedContext {
			t.Errorf("case %q decoded non-text prompt capabilities as enabled: %+v", c.ID, capabilities.PromptCapabilities)
		}
	}
}

func TestFactoryOptionCasesParseAsACPSDKSelectSessionConfigOption(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	for _, c := range corpus.ByRole(acpconformance.RoleFactoryOption) {
		var option acpsdk.SessionConfigOption
		if err := json.Unmarshal(c.Payload, &option); err != nil {
			t.Fatalf("case %q payload does not parse as acpsdk.SessionConfigOption: %v", c.ID, err)
		}
		if option.Select == nil {
			t.Errorf("case %q decoded with no select variant, want type=select", c.ID)
		}
		if option.Boolean != nil {
			t.Errorf("case %q decoded a boolean variant, want only the select variant", c.ID)
		}
	}
}

func TestPromptContentCasesParseAsACPSDKPromptRequest(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	for _, c := range corpus.ByRole(acpconformance.RolePromptContent) {
		var req acpsdk.PromptRequest
		if err := json.Unmarshal(c.Payload, &req); err != nil {
			t.Fatalf("case %q payload does not parse as acpsdk.PromptRequest: %v", c.ID, err)
		}
		for _, block := range req.Prompt {
			if block.Image != nil || block.Audio != nil || block.Resource != nil || block.ResourceLink != nil {
				t.Errorf("case %q prompt content included a non-text block, want text-only content", c.ID)
			}
		}
	}
}

func TestRequestIdentityCasesCarryASupportedWireIDShape(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	for _, c := range corpus.ByRole(acpconformance.RoleRequestIdentity) {
		var envelope struct {
			ID *acpsdk.RequestId `json:"id"`
		}
		if err := json.Unmarshal(c.Payload, &envelope); err != nil {
			t.Fatalf("case %q payload id does not parse as acpsdk.RequestId: %v", c.ID, err)
		}
		switch c.Facts.Outcome {
		case "string_id":
			if envelope.ID == nil || envelope.ID.Str == nil {
				t.Errorf("case %q declares string_id but decoded id = %+v", c.ID, envelope.ID)
			}
		case "number_id":
			if envelope.ID == nil || envelope.ID.Number == nil {
				t.Errorf("case %q declares number_id but decoded id = %+v", c.ID, envelope.ID)
			}
		case "minted":
			if envelope.ID != nil {
				t.Errorf("case %q declares minted (no wire id) but payload carried an id", c.ID)
			}
		}
	}
}

func TestMalformedInputInvalidRequestIDShapeFailsToDecode(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	var target acpconformance.Case
	found := false
	for _, c := range corpus.ByRole(acpconformance.RoleMalformedInput) {
		if c.Facts.Outcome == "invalid_request" {
			target = c
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected a malformed_input case declaring outcome invalid_request")
	}
	var envelope struct {
		ID acpsdk.RequestId `json:"id"`
	}
	if err := json.Unmarshal(target.Payload, &envelope); err == nil {
		t.Fatal("expected the invalid_request case's id to fail acpsdk.RequestId decoding, got nil error")
	}
}

// TestCorpusContentIsSanitized guards the corpus artifact's own sanitization
// invariant: no credential-shaped values, machine-specific absolute paths,
// or raw provider commands. This tests the shipped corpus.json contract
// itself (an explicit acceptance criterion), not source-file inventory.
func TestCorpusContentIsSanitized(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	forbidden := []string{
		"/home/", "/Users/", `C:\Users`, "password", "secret", "api_key", "apikey",
		"Bearer ", "AKIA", "-----BEGIN",
	}
	for _, c := range corpus.Cases {
		haystack := strings.ToLower(c.Description + " " + string(c.Payload) + " " + c.Facts.Text + " " + c.Provenance)
		for _, term := range forbidden {
			if strings.Contains(haystack, strings.ToLower(term)) {
				t.Errorf("case %q contains forbidden sanitization term %q", c.ID, term)
			}
		}
	}
}

func TestUnsupportedMethodCasesDeclareMethodNotFoundOrNoResponse(t *testing.T) {
	corpus := acpconformance.MustLoad(t)
	cases := corpus.ByRole(acpconformance.RoleUnsupportedMethod)
	if len(cases) == 0 {
		t.Fatal("expected at least one unsupported_method case")
	}
	sawRequest, sawNotification := false, false
	for _, c := range cases {
		var envelope struct {
			ID *acpsdk.RequestId `json:"id"`
		}
		if err := json.Unmarshal(c.Payload, &envelope); err != nil {
			t.Fatalf("case %q payload id does not parse: %v", c.ID, err)
		}
		if envelope.ID != nil {
			sawRequest = true
			if c.Facts.Outcome != "method_not_found" {
				t.Errorf("case %q has an id but Facts.Outcome = %q, want method_not_found", c.ID, c.Facts.Outcome)
			}
			if c.Directionality != acpconformance.DirectionUnsupported {
				t.Errorf("case %q has an id but directionality = %q, want unsupported", c.ID, c.Directionality)
			}
		} else {
			sawNotification = true
			if c.Facts.Outcome != "no_response_emitted" {
				t.Errorf("case %q has no id but Facts.Outcome = %q, want no_response_emitted", c.ID, c.Facts.Outcome)
			}
			if c.Directionality != acpconformance.DirectionNoOutput {
				t.Errorf("case %q has no id but directionality = %q, want no_output", c.ID, c.Directionality)
			}
		}
	}
	if !sawRequest || !sawNotification {
		t.Fatalf("expected unsupported_method cases to cover both a request (sawRequest=%v) and a notification (sawNotification=%v)", sawRequest, sawNotification)
	}
}

func TestParseCorpusIsUsableFromASyntheticMinimalFixture(t *testing.T) {
	// Confirms ParseCorpus works over an arbitrary, well-formed corpus, not
	// only the embedded one, per its role as a general reusable reader.
	minimal := validMinimalCorpusJSON(t)
	corpus, err := acpconformance.ParseCorpus([]byte(minimal))
	if err != nil {
		t.Fatalf("ParseCorpus(minimal valid fixture) unexpected error: %v", err)
	}
	if len(corpus.Cases) != 1 {
		t.Fatalf("len(corpus.Cases) = %d, want 1", len(corpus.Cases))
	}
}
