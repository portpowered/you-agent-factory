package acp_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpfixtures"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/envelope"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// fixtureConnectionID is the synthetic connection identity every fixture
// case decodes under. Fixture cases prove method-specific semantics and the
// combined method/request/identity round trip; cross-connection identity
// distinctness is proven directly by envelope and protocol package tests,
// not by this corpus.
const fixtureConnectionID = "fixture-connection"

// TestACPConformanceFixtures decodes every committed sanitized fixture
// corpus from the shared internal/testutil/acpfixtures corpus and asserts
// each case's declared semantic behavior against the L1 V0 compatibility
// functions those cases exercise. Every JSON-RPC method-role case's Input
// is a complete JSON-RPC message, decoded through envelope.Decode (and, for
// every role whose rejections use the shared three-way RejectionKind
// classification, dispatched through protocol.GuardEnvelope) so the
// combined method + request + method-specific identity round trip is
// proven, not just the isolated params-to-value mapping. It proves
// behavior only through parsed protocol outcomes: it never scans the
// testdata directory's file inventory, and it never asserts anything about
// which files exist. The same committed corpus is independently consumed
// by the inbound Providers-owned ACP mapper (see
// pkg/services/providers/internal/services/acp/internal/service), so both
// protocol directions are checked against the same semantic inputs without
// either importing the other's production package.
func TestACPConformanceFixtures(t *testing.T) {
	corpora, err := acpfixtures.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}

	for _, corpus := range corpora {
		for _, c := range corpus.Cases {
			t.Run(string(c.Role)+"/"+c.Name, func(t *testing.T) {
				assertCaseSemantics(t, c)
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

func assertCaseSemantics(t *testing.T, c acpfixtures.Case) {
	t.Helper()
	switch c.Role {
	case acpfixtures.RoleInitialize:
		// initialize's own error payload (e.g. reason:
		// unsupported_protocol_version, with requested/supported version
		// numbers) is richer than the shared three-way RejectionKind
		// SafeReject produces, so this case decodes the envelope for its
		// identity/params binding but calls NegotiateInitialization
		// directly rather than through protocol.Guard's generic rejection
		// wrapping.
		env, err := envelope.Decode(fixtureConnectionID, c.Input)
		if err != nil {
			t.Fatalf("%s: envelope.Decode() unexpected error: %v", c.Name, err)
		}
		if env.IsNotification {
			t.Fatalf("%s: initialize decoded as a notification, want a request", c.Name)
		}
		var req acpsdk.InitializeRequest
		mustUnmarshal(t, env.Params, &req)
		resp, err := acp.NegotiateInitialization(req)
		assertOutcome(t, c, resp, err)

	case acpfixtures.RoleSessionNew:
		var got session.NewSessionParams
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				v, verr := session.ValidateNewSession(env.Params)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionLoad:
		var got session.LoadSessionParams
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				v, verr := session.ValidateLoadSession(env.Params)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionResume:
		var got session.LoadSessionParams
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				v, verr := session.ValidateResumeSession(env.Params)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionCancel:
		var got session.CancelParams
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				var req acpsdk.CancelNotification
				if uerr := json.Unmarshal(env.Params, &req); uerr != nil {
					return uerr
				}
				v, verr := session.ValidateCancel(req)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionSetConfigOption:
		var got session.ConfigOptionValue
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				v, verr := session.ValidateSetConfigOption(env.Params)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionPrompt:
		var got session.PromptTurn
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				var req acpsdk.PromptRequest
				if uerr := json.Unmarshal(env.Params, &req); uerr != nil {
					return uerr
				}
				v, verr := session.ValidatePrompt(req)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionUpdate:
		var got session.TextUpdate
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				var notif acpsdk.SessionNotification
				if uerr := json.Unmarshal(env.Params, &notif); uerr != nil {
					return uerr
				}
				v, verr := session.ValidateSessionUpdate(notif)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotification(t, c, env)
		assertOutcome(t, c, got, err)

	case acpfixtures.RoleSessionRequestPermission:
		var got session.PermissionCorrelation
		env, err := protocol.GuardEnvelope(fixtureConnectionID, c.Input,
			func(env envelope.Envelope) error {
				var req acpsdk.RequestPermissionRequest
				if uerr := json.Unmarshal(env.Params, &req); uerr != nil {
					return uerr
				}
				v, verr := session.ValidatePermissionCorrelation(req)
				got = v
				return verr
			},
			func() error { return nil },
		)
		requireNotNotification(t, c, env)
		assertOutcome(t, c, got, err)

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
		t.Fatalf("no compatibility check wired for role %q", c.Role)
	}
}

// requireNotNotification asserts a request-role case decoded as an
// ordinary, id-correlated request, proving the envelope layer's
// request/notification split lines up with this corpus's declared roles.
func requireNotNotification(t *testing.T, c acpfixtures.Case, env envelope.Envelope) {
	t.Helper()
	if env.IsNotification {
		t.Fatalf("%s: decoded as a notification, want a request (IsNotification=false)", c.Name)
	}
}

// requireNotification asserts a notification-role case (session/cancel,
// session/update) decoded with a minted, connection-scoped identity and no
// JSON-RPC id, proving the envelope layer's request/notification split
// lines up with this corpus's declared roles.
func requireNotification(t *testing.T, c acpfixtures.Case, env envelope.Envelope) {
	t.Helper()
	if !env.IsNotification {
		t.Fatalf("%s: decoded as a request, want a notification (IsNotification=true)", c.Name)
	}
	if !env.Identity.IsMinted() {
		t.Fatalf("%s: notification identity is not minted", c.Name)
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
