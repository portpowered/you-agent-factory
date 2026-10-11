package admission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

func sendInput() agentmessages.SendRequest {
	return agentmessages.SendRequest{RequestID: "request", To: &agentmessages.Address{WorkerSessionID: "recipient"}, Body: "hello"}
}

func TestValidatorRejectsInvalidInputWithoutRedaction(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*agentmessages.SendRequest){
		"blank body":                 func(r *agentmessages.SendRequest) { r.Body = " \n" },
		"empty body":                 func(r *agentmessages.SendRequest) { r.Body = "" },
		"invalid body UTF8":          func(r *agentmessages.SendRequest) { r.Body = string([]byte{0xff}) },
		"blank request":              func(r *agentmessages.SendRequest) { r.RequestID = " " },
		"long request":               func(r *agentmessages.SendRequest) { r.RequestID = strings.Repeat("é", 201) },
		"invalid request UTF8":       func(r *agentmessages.SendRequest) { r.RequestID = string([]byte{0xff}) },
		"missing address and parent": func(r *agentmessages.SendRequest) { r.To = nil },
		"blank recipient":            func(r *agentmessages.SendRequest) { r.To.WorkerSessionID = " " },
		"blank parent":               func(r *agentmessages.SendRequest) { r.InReplyTo = " " },
		"invalid work":               func(r *agentmessages.SendRequest) { r.Correlation.WorkID = " " },
		"invalid factory":            func(r *agentmessages.SendRequest) { r.Correlation.FactorySessionID = " " },
		"invalid delivery":           func(r *agentmessages.SendRequest) { r.Delivery = "SEND" },
		"invalid if ended":           func(r *agentmessages.SendRequest) { r.IfEnded = "SEND" },
		"invalid reply policy":       func(r *agentmessages.SendRequest) { r.ReplyIfEnded = "SEND" },
		"short expiry":               func(r *agentmessages.SendRequest) { r.ExpiresInSeconds = intPointer(59) },
		"long expiry":                func(r *agentmessages.SendRequest) { r.ExpiresInSeconds = intPointer(604801) },
		"zero expiry":                func(r *agentmessages.SendRequest) { r.ExpiresInSeconds = intPointer(0) },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := sendInput()
			change(&r)
			r.BodySecret = true
			v := Validator{Redact: func(recordings.RecordingRedactionRequest) (recordings.RecordingRedactionResult, error) {
				t.Fatal("invalid input reached redaction")
				return recordings.RecordingRedactionResult{}, nil
			}}
			got, err := v.Prepare(r, "")
			if !errors.Is(err, agentmessages.ErrBadRequest) || got.Request.Body != "" {
				t.Fatalf("got %v, safe body present=%v", err, got.Request.Body != "")
			}
		})
	}
}

func TestValidatorBodyByteAndExpiryBoundaries(t *testing.T) {
	t.Parallel()
	for _, body := range []string{"a", strings.Repeat("a", 8192), strings.Repeat("é", 4096)} {
		for _, expiry := range []int{60, 86400, 604800} {
			r := sendInput()
			r.Body, r.ExpiresInSeconds = body, intPointer(expiry)
			got, err := (Validator{}).Prepare(r, "")
			if err != nil || *got.Request.ExpiresInSeconds != expiry || got.Request.Body != body {
				t.Fatalf("bytes=%d expiry=%d err=%v", len(body), expiry, err)
			}
		}
	}
	for _, body := range []string{strings.Repeat("a", 8193), strings.Repeat("é", 4096) + "a"} {
		r := sendInput()
		r.Body, r.BodySecret = body, true
		if _, err := (Validator{}).Prepare(r, ""); !errors.Is(err, agentmessages.ErrLimitExceeded) {
			t.Fatalf("oversized secret must fail before redaction: %v", err)
		}
	}
	r := sendInput()
	if _, err := (Validator{BodyBytes: 4}).Prepare(r, ""); !errors.Is(err, agentmessages.ErrLimitExceeded) {
		t.Fatalf("configured byte limit: %v", err)
	}
}

func TestValidatorDefaultsAndDetachedInput(t *testing.T) {
	t.Parallel()
	r := sendInput()
	r.RequestID = strings.Repeat("é", 200)
	got, err := (Validator{}).Prepare(r, "")
	if err != nil || got.Request.Delivery != "QUEUE" || got.Request.IfEnded != "REVIVE" ||
		got.Request.ReplyIfEnded != "REVIVE" || *got.Request.ExpiresInSeconds != 86400 {
		t.Fatalf("defaults: %+v %v", got, err)
	}
	got.Request.To.WorkerSessionID = "other"
	if r.To.WorkerSessionID != "recipient" || r.ExpiresInSeconds != nil || r.Delivery != "" {
		t.Fatal("preparation changed caller-owned input")
	}
	r = sendInput()
	r.To, r.InReplyTo = nil, "parent"
	got, err = (Validator{}).Prepare(r, "")
	if err != nil || got.Request.IfEnded != "" || got.Request.To != nil {
		t.Fatal("reply must retain absent target and policy for parent inheritance")
	}
	r.Delivery = "INTERRUPT"
	if _, err := (Validator{}).Prepare(r, ""); !errors.Is(err, agentmessages.ErrInterruptUnsupported) {
		t.Fatalf("interrupt: %v", err)
	}
}

func TestValidatorRedactsBeforeHashAndFingerprint(t *testing.T) {
	t.Parallel()
	v := Validator{Redact: func(r recordings.RecordingRedactionRequest) (recordings.RecordingRedactionResult, error) {
		if len(r.Secrets) != 1 || r.Secrets[0].JSONPointer != "/body" || r.Secrets[0].Provenance != recordings.RecordingSecretProvenanceDeclared {
			t.Fatal("missing explicit body classification")
		}
		return recordings.RecordingRedactionResult{Payload: json.RawMessage(`{"body":"<redacted>"}`), RedactedCount: 1}, nil
	}}
	var fingerprint string
	for _, body := range []string{"first planted secret", "second planted secret", "prefix caller-token suffix", "<redacted>"} {
		r := sendInput()
		r.Body, r.BodySecret = body, body != "prefix caller-token suffix"
		got, err := v.Prepare(r, "caller-token")
		if err != nil || got.Request.Body != "<redacted>" || got.BodyRedactionCount != 1 || got.Request.BodySecret {
			t.Fatalf("redaction failed: %v", err)
		}
		hash := sha256.Sum256([]byte("<redacted>"))
		if got.BodySHA256 != hex.EncodeToString(hash[:]) {
			t.Fatal("hash was not computed from safe content")
		}
		got.Request.RequestID = "alias"
		digest, err := got.Fingerprint("recipient-factory")
		if err != nil || (fingerprint != "" && fingerprint != digest) {
			t.Fatal("erased secret or alias influenced fingerprint")
		}
		fingerprint = digest
	}
}

func TestPreparedFingerprintIncludesResolvedRecipientScope(t *testing.T) {
	t.Parallel()
	prepared, err := (Validator{}).Prepare(sendInput(), "")
	if err != nil {
		t.Fatal(err)
	}
	first, err := prepared.Fingerprint("first-factory")
	if err != nil {
		t.Fatal(err)
	}
	other, err := prepared.Fingerprint("other-factory")
	if err != nil || first == other {
		t.Fatal("legacy recipient ID collision erased its resolved owner")
	}
	prepared.Request.RequestID = "different-alias"
	retried, err := prepared.Fingerprint("first-factory")
	if err != nil || retried != first {
		t.Fatal("request alias changed normalized admission intent")
	}
	prepared.Request.Correlation.FactorySessionID = "descriptive-factory"
	described, err := prepared.Fingerprint("first-factory")
	if err != nil || described == other {
		t.Fatal("descriptive correlation replaced recipient owner scope")
	}
}

func TestValidatorOmitsSensitiveFailureDiagnostics(t *testing.T) {
	t.Parallel()
	r := sendInput()
	r.Body, r.BodySecret = "planted secret", true
	v := Validator{Redact: func(recordings.RecordingRedactionRequest) (recordings.RecordingRedactionResult, error) {
		return recordings.RecordingRedactionResult{}, errors.New("planted secret caller-token /private/store")
	}}
	got, err := v.Prepare(r, "caller-token")
	if !errors.Is(err, agentmessages.ErrBadRequest) || err.Error() != "BAD_REQUEST" || got.Request.Body != "" {
		t.Fatal("redaction failure leaked content or diagnostics")
	}
	for _, field := range []string{"request", "recipient", "parent", "work", "factory"} {
		r := sendInput()
		switch field {
		case "request":
			r.RequestID = "caller-token"
		case "recipient":
			r.To.WorkerSessionID = "caller-token"
		case "parent":
			r.InReplyTo = "caller-token"
		case "work":
			r.Correlation.WorkID = "caller-token"
		case "factory":
			r.Correlation.FactorySessionID = "caller-token"
		}
		if _, err := (Validator{}).Prepare(r, "caller-token"); !errors.Is(err, agentmessages.ErrBadRequest) {
			t.Fatalf("credential-bearing %s allowed", field)
		}
	}
}

func intPointer(value int) *int { return &value }
