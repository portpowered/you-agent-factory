package acp_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/internal/testutil/acpfixtures"
	"github.com/portpowered/infinite-you/pkg/transports/acp"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/protocol"
	"github.com/portpowered/infinite-you/pkg/transports/acp/internal/session"
)

// TestACPConformanceFixtures decodes every committed sanitized fixture
// corpus from the shared internal/testutil/acpfixtures corpus and asserts
// each case's declared semantic behavior against the L1 V0 compatibility
// functions those cases exercise. It proves behavior only through parsed
// protocol outcomes: it never scans the testdata directory's file
// inventory, and it never asserts anything about which files exist. The
// same committed corpus is independently consumed by the inbound
// Providers-owned ACP mapper (see
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
		var req acpsdk.InitializeRequest
		mustUnmarshal(t, c.Input, &req)
		resp, err := acp.NegotiateInitialization(req)
		assertOutcome(t, c, resp, err)

	case acpfixtures.RoleSessionNew:
		got, err := session.ValidateNewSession(c.Input)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionLoad:
		got, err := session.ValidateLoadSession(c.Input)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionResume:
		got, err := session.ValidateResumeSession(c.Input)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionCancel:
		var req acpsdk.CancelNotification
		mustUnmarshal(t, c.Input, &req)
		got, err := session.ValidateCancel(req)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionSetConfigOption:
		got, err := session.ValidateSetConfigOption(c.Input)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionPrompt:
		var req acpsdk.PromptRequest
		mustUnmarshal(t, c.Input, &req)
		got, err := session.ValidatePrompt(req)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionUpdate:
		var upd acpsdk.SessionUpdate
		mustUnmarshal(t, c.Input, &upd)
		got, err := session.ValidateSessionUpdate(upd)
		assertOutcome(t, c, got, rejectionOf(err))

	case acpfixtures.RoleSessionRequestPermission:
		var req acpsdk.RequestPermissionRequest
		mustUnmarshal(t, c.Input, &req)
		got, err := session.ValidatePermissionCorrelation(req)
		assertOutcome(t, c, got, rejectionOf(err))

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

// rejectionOf converts an internal validation cause into the same bounded,
// protocol-safe error the L1 V0 boundary would actually return to a client.
func rejectionOf(cause error) error {
	if cause == nil {
		return nil
	}
	return protocol.SafeReject(cause)
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
