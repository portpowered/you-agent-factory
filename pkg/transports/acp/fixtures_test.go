package acp_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpfixtures"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/identity"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// fixtureConnectionID is the synthetic connection identity every fixture
// case decodes under. Fixture cases prove method-specific semantics and the
// combined method/request/identity round trip; cross-connection identity
// distinctness is proven directly by envelope and protocol package tests,
// not by this corpus.
const fixtureConnectionID = "fixture-connection"

// methodDispatch is a test-only adapter over this package's real
// compatibility mapping: for every session/* method this transport
// supports, it decodes the method-specific SDK params from an already
// identity-bound envelope.Envelope and calls the exact Validate* function
// that owns that method. assertDispatchedCase looks this map up by the
// envelope's own decoded Method -- the same key protocol.GuardEnvelope
// dispatches on -- rather than by a fixture's declared Role, so a fixture
// whose Role does not actually match its Input's wire "method" is caught as
// a test failure instead of silently routed to whichever validator the
// Role happens to name.
//
// "initialize" is deliberately not in this map: NegotiateInitialization
// returns its own richer *acpsdk.RequestError (see assertInitializeCase),
// which protocol.Guard's generic SafeReject classification would
// overwrite.
var methodDispatch = map[string]func(env envelope.Envelope) (any, error){
	"session/new": func(env envelope.Envelope) (any, error) {
		return session.ValidateNewSession(env.Params)
	},
	"session/load": func(env envelope.Envelope) (any, error) {
		return session.ValidateLoadSession(env.Params)
	},
	"session/resume": func(env envelope.Envelope) (any, error) {
		return session.ValidateResumeSession(env.Params)
	},
	"session/cancel": func(env envelope.Envelope) (any, error) {
		var req acpsdk.CancelNotification
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return nil, err
		}
		return session.ValidateCancel(req)
	},
	"session/set_config_option": func(env envelope.Envelope) (any, error) {
		return session.ValidateSetConfigOption(env.Params)
	},
	"session/prompt": func(env envelope.Envelope) (any, error) {
		var req acpsdk.PromptRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return nil, err
		}
		return session.ValidatePrompt(req)
	},
	"session/update": func(env envelope.Envelope) (any, error) {
		var notif acpsdk.SessionNotification
		if err := json.Unmarshal(env.Params, &notif); err != nil {
			return nil, err
		}
		return session.ValidateSessionUpdate(notif)
	},
	"session/request_permission": func(env envelope.Envelope) (any, error) {
		var req acpsdk.RequestPermissionRequest
		if err := json.Unmarshal(env.Params, &req); err != nil {
			return nil, err
		}
		return session.ValidatePermissionCorrelation(req)
	},
}

// TestACPConformanceFixtures decodes every committed sanitized fixture
// corpus from the shared internal/testutil/acpfixtures corpus and asserts
// each case's declared semantic behavior against the L1 V0 compatibility
// functions those cases exercise. Every JSON-RPC method-role case's Input
// is a complete JSON-RPC message, decoded through envelope.Decode and
// dispatched by the envelope's own decoded Method (via methodDispatch, or
// directly to NegotiateInitialization for "initialize") rather than by the
// fixture's declared Role, so the combined method + request/notification
// identity + method-specific identity round trip is proven end to end, not
// just the isolated params-to-value mapping. Every accepted case's semantic
// value and every case's request identity are additionally round-tripped
// through encoding/json and compared against the pre-round-trip value, so a
// content or correlation field that failed to survive encode/decode would
// fail here even if the raw Expected comparison happened to still match.
// It proves behavior only through parsed protocol outcomes: it never scans
// the testdata directory's file inventory, and it never asserts anything
// about which files exist. The same committed corpus is independently
// consumed by the inbound Providers-owned ACP mapper (see
// pkg/services/providers/internal/services/acp/internal/service), so both
// protocol directions are checked against the same semantic inputs without
// either importing the other's production package.
func TestACPConformanceFixtures(t *testing.T) {
	corpora, err := acpfixtures.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	var seq uint64
	for _, corpus := range corpora {
		for _, c := range corpus.Cases {
			seq++
			caseSeq := seq
			t.Run(string(c.Role)+"/"+c.Name, func(t *testing.T) {
				assertCaseSemantics(t, c, caseSeq)
			})
		}
	}
}

// TestACPConformanceFixtureShapeRejectsInvalidCorpus proves the fixture
// parser used by TestACPConformanceFixtures fails clearly, rather than
// silently accepting a structurally invalid corpus.
func TestACPConformanceFixtureShapeRejectsInvalidCorpus(t *testing.T) {
	_, err := acpfixtures.Parse([]byte(`{"cases":[{"name":"missing role fields","input":{},"expected":{}}]}`))
	if err == nil {
		t.Fatal("Parse() error = nil, want a clear error for a missing role/direction/classification")
	}
}

func assertCaseSemantics(t *testing.T, c acpfixtures.Case, seq uint64) {
	t.Helper()
	switch c.Role {
	case acpfixtures.RoleInitialize:
		// initialize's own error payload (e.g. reason:
		// unsupported_protocol_version, with requested/supported version
		// numbers) is richer than the shared three-way RejectionKind
		// SafeReject produces, so this case decodes the envelope for its
		// identity/method binding but calls NegotiateInitialization
		// directly rather than through protocol.Guard's generic rejection
		// wrapping.
		assertInitializeCase(t, c, seq)

	case acpfixtures.RoleStopReason:
		var in struct {
			Outcome string `json:"outcome"`
		}
		mustUnmarshal(t, c.Input, &in)
		cause := errors.New("synthetic internal cause that must never be serialized")
		result := protocol.MapStopReason(protocol.TerminalOutcome(in.Outcome), cause)
		assertOutcome(t, c, result, nil)

	case acpfixtures.RoleUnsupportedMethod:
		var in struct {
			Method string `json:"method"`
		}
		mustUnmarshal(t, c.Input, &in)
		err := protocol.Guard(in.Method,
			func() error { t.Fatal("validate must not run for an unsupported method"); return nil },
			func() error { t.Fatal("effect must not run for an unsupported method"); return nil },
		)
		assertOutcome(t, c, nil, err)

	default:
		assertDispatchedCase(t, c, seq)
	}
}

// assertInitializeCase decodes the envelope to prove the fixture's declared
// Role matches its own Input's wire method and that the resulting request
// identity round-trips, then asserts NegotiateInitialization's outcome
// directly (see the RoleInitialize case in assertCaseSemantics for why it
// bypasses protocol.Guard/GuardEnvelope).
func assertInitializeCase(t *testing.T, c acpfixtures.Case, seq uint64) {
	t.Helper()
	env, err := envelope.Decode(fixtureConnectionID, seq, c.Input)
	if err != nil {
		t.Fatalf("%s: envelope.Decode() unexpected error: %v", c.Name, err)
	}
	if env.IsNotification {
		t.Fatalf("%s: initialize decoded as a notification, want a request", c.Name)
	}
	if env.Method != string(c.Role) {
		t.Fatalf("%s: envelope method = %q, want fixture role %q", c.Name, env.Method, c.Role)
	}
	assertIdentityRoundTrips(t, c.Name, env.Identity)

	var req acpsdk.InitializeRequest
	mustUnmarshal(t, env.Params, &req)
	resp, err := acp.NegotiateInitialization(req)
	assertOutcome(t, c, resp, err)
}

// assertDispatchedCase decodes and dispatches a session/* fixture case
// through protocol.GuardEnvelope exactly as production dispatch would,
// selecting the validator to call from the envelope's own decoded Method
// via methodDispatch rather than from the fixture's declared Role. It then
// proves: the decoded method matches the fixture's declared Role, the
// decoded notification/request classification matches the production
// envelope.NotificationMethods set, the request identity round-trips
// through JSON, the case's declared Classification and Expected value hold,
// and -- for an accepted case -- the resulting semantic value (carrying
// session/content/correlation fields such as SessionID, ToolCallID, or
// OptionIDs) itself round-trips through JSON without loss.
func assertDispatchedCase(t *testing.T, c acpfixtures.Case, seq uint64) {
	t.Helper()

	var got any
	env, err := protocol.GuardEnvelope(fixtureConnectionID, seq, c.Input,
		func(env envelope.Envelope) error {
			dispatch, ok := methodDispatch[env.Method]
			if !ok {
				return fmt.Errorf("no compatibility dispatch wired for method %q", env.Method)
			}
			v, verr := dispatch(env)
			got = v
			return verr
		},
		func() error { return nil },
	)

	if env.Method != string(c.Role) {
		t.Fatalf("%s: envelope method = %q, want fixture role %q", c.Name, env.Method, c.Role)
	}
	if wantNotification := envelope.NotificationMethods[env.Method]; env.IsNotification != wantNotification {
		t.Fatalf("%s: IsNotification = %v, want %v for method %q", c.Name, env.IsNotification, wantNotification, env.Method)
	}
	assertIdentityRoundTrips(t, c.Name, env.Identity)

	assertOutcome(t, c, got, err)
	if c.Classification == acpfixtures.ClassificationAccepted {
		assertValueRoundTrips(t, c.Name, got)
	}
}

// assertIdentityRoundTrips proves a request identity produced by decoding a
// real fixture's wire message survives an encode/decode cycle unchanged.
func assertIdentityRoundTrips(t *testing.T, name string, id identity.RequestIdentity) {
	t.Helper()
	encoded, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("%s: marshal request identity: %v", name, err)
	}
	var decoded identity.RequestIdentity
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("%s: unmarshal request identity: %v", name, err)
	}
	if !decoded.Equal(id) {
		t.Fatalf("%s: request identity did not round-trip: got %+v, want %+v", name, decoded, id)
	}
}

// assertValueRoundTrips proves an accepted case's semantic value survives an
// encode/decode cycle unchanged, so a lossy reduction (e.g. a dropped
// session or correlation field) fails here even if it happened to still
// satisfy the fixture's Expected comparison.
func assertValueRoundTrips(t *testing.T, name string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("%s: marshal semantic value: %v", name, err)
	}
	roundTripped := reflect.New(reflect.TypeOf(value))
	if err := json.Unmarshal(data, roundTripped.Interface()); err != nil {
		t.Fatalf("%s: unmarshal semantic value: %v", name, err)
	}
	if !reflect.DeepEqual(value, roundTripped.Elem().Interface()) {
		t.Fatalf("%s: semantic value did not round-trip: got %+v, want %+v", name, roundTripped.Elem().Interface(), value)
	}
}

// assertOutcome checks a case's declared Classification against what the
// compatibility function under test actually produced: an accepted case
// must succeed and match Expected against its semantic value; a rejected
// case must fail and match Expected against its safe protocol error.
func assertOutcome(t *testing.T, c acpfixtures.Case, value any, err error) {
	t.Helper()
	switch c.Classification {
	case acpfixtures.ClassificationAccepted:
		if err != nil {
			t.Fatalf("%s: unexpected rejection: %v", c.Name, err)
		}
		assertJSONEqual(t, c.Name, c.Expected, value)
	case acpfixtures.ClassificationRejected:
		if err == nil {
			t.Fatalf("%s: expected a rejection, got none", c.Name)
		}
		assertJSONEqual(t, c.Name, c.Expected, err)
	default:
		t.Fatalf("%s: unknown classification %q", c.Name, c.Classification)
	}
}

func mustUnmarshal(t *testing.T, data json.RawMessage, target any) {
	t.Helper()
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode fixture input into %T: %v", target, err)
	}
}

// assertJSONEqual compares got (marshaled the same way it would cross the
// wire) against a fixture's raw expected JSON, structurally rather than
// byte-for-byte, so struct field ordering and tag mechanics never cause a
// spurious mismatch.
func assertJSONEqual(t *testing.T, name string, want json.RawMessage, got any) {
	t.Helper()
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("%s: marshal actual value: %v", name, err)
	}
	var wantAny, gotAny any
	if err := json.Unmarshal(want, &wantAny); err != nil {
		t.Fatalf("%s: decode expected fixture value: %v", name, err)
	}
	if err := json.Unmarshal(gotBytes, &gotAny); err != nil {
		t.Fatalf("%s: decode actual value: %v", name, err)
	}
	if !reflect.DeepEqual(wantAny, gotAny) {
		t.Fatalf("%s: got %s, want %s", name, gotBytes, want)
	}
}
