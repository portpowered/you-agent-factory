// Package admission owns privacy-normalized message input and admission policy.
package admission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"unicode/utf8"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

const (
	DefaultBodyBytes     = 8192
	DefaultExpirySeconds = 86400
	MinExpirySeconds     = 60
	MaxExpirySeconds     = 604800
	MaxHop               = 3
	maxRequestRunes      = 200
)

// Validator consumes the public declared-secret capability injected by Wire.
// BodyBytes is the effective configured limit, bounded by the public contract.
type Validator struct {
	BodyBytes int
	Redact    func(recordings.RecordingRedactionRequest) (recordings.RecordingRedactionResult, error)
}

// Prepared contains only safe content. Fingerprints omit request IDs so the
// admission owner can compare durable aliases without fingerprinting secrets.
type Prepared struct {
	Request            agentmessages.SendRequest
	BodySHA256         string
	BodyRedactionCount int
}

// Prepare validates raw bytes before redaction. Credential-bearing identifiers
// are refused; credential-bearing bodies are conservatively classified whole.
// The caller must authenticate independently before consuming this result.
func (v Validator) Prepare(request agentmessages.SendRequest, token string) (Prepared, error) {
	if !validRequest(request) || containsCredential(request, token) {
		return Prepared{}, agentmessages.ErrBadRequest
	}
	limit := v.BodyBytes
	if limit == 0 {
		limit = DefaultBodyBytes
	}
	if limit < 1 || limit > DefaultBodyBytes || len(request.Body) > limit {
		return Prepared{}, agentmessages.ErrLimitExceeded
	}
	if request.Delivery == "INTERRUPT" {
		return Prepared{}, agentmessages.ErrInterruptUnsupported
	}
	request = detachedDefaults(request)
	count := 0
	if request.BodySecret || (token != "" && strings.Contains(request.Body, token)) {
		var err error
		request.Body, count, err = v.redactBody(request.Body)
		if err != nil {
			return Prepared{}, err
		}
	}
	request.BodySecret = false
	return Prepared{Request: request, BodySHA256: digest([]byte(request.Body)), BodyRedactionCount: count}, nil
}

func validRequest(r agentmessages.SendRequest) bool {
	if !validID(r.RequestID) || utf8.RuneCountInString(r.RequestID) > maxRequestRunes ||
		!utf8.ValidString(r.Body) || strings.TrimSpace(r.Body) == "" {
		return false
	}
	if r.To == nil && r.InReplyTo == "" {
		return false
	}
	if r.To != nil && !validID(r.To.WorkerSessionID) {
		return false
	}
	if !optionalID(r.InReplyTo) || !optionalID(r.Correlation.WorkID) || !optionalID(r.Correlation.FactorySessionID) {
		return false
	}
	return validDeliveryOptions(r)
}

func validDeliveryOptions(r agentmessages.SendRequest) bool {
	if r.Delivery != "" && r.Delivery != "QUEUE" && r.Delivery != "INTERRUPT" {
		return false
	}
	if !validEndedPolicy(r.IfEnded) || !validEndedPolicy(r.ReplyIfEnded) {
		return false
	}
	return r.ExpiresInSeconds == nil || (*r.ExpiresInSeconds >= MinExpirySeconds && *r.ExpiresInSeconds <= MaxExpirySeconds)
}

func validID(id string) bool             { return utf8.ValidString(id) && strings.TrimSpace(id) != "" }
func optionalID(id string) bool          { return id == "" || validID(id) }
func validEndedPolicy(value string) bool { return value == "" || value == "REVIVE" || value == "HOLD" }

func containsCredential(r agentmessages.SendRequest, token string) bool {
	if token == "" {
		return false
	}
	fields := []string{r.RequestID, r.InReplyTo, r.Correlation.WorkID, r.Correlation.FactorySessionID}
	if r.To != nil {
		fields = append(fields, r.To.WorkerSessionID)
	}
	for _, field := range fields {
		if strings.Contains(field, token) {
			return true
		}
	}
	return false
}

func detachedDefaults(r agentmessages.SendRequest) agentmessages.SendRequest {
	if r.To != nil {
		to := *r.To
		r.To = &to
	}
	expiry := DefaultExpirySeconds
	if r.ExpiresInSeconds != nil {
		expiry = *r.ExpiresInSeconds
	}
	r.ExpiresInSeconds = &expiry
	if r.Delivery == "" {
		r.Delivery = "QUEUE"
	}
	// Leave reply IfEnded absent: parent.ReplyIfEnded supplies its default.
	if r.IfEnded == "" && r.InReplyTo == "" {
		r.IfEnded = "REVIVE"
	}
	if r.ReplyIfEnded == "" {
		r.ReplyIfEnded = "REVIVE"
	}
	return r
}

func (v Validator) redactBody(body string) (string, int, error) {
	if v.Redact == nil {
		return "", 0, agentmessages.ErrBadRequest
	}
	encoded, err := json.Marshal(struct {
		Body string `json:"body"`
	}{body})
	if err != nil {
		return "", 0, agentmessages.ErrBadRequest
	}
	result, err := v.Redact(recordings.RecordingRedactionRequest{
		Payload: encoded,
		Secrets: []recordings.RecordingSecret{{JSONPointer: "/body", Provenance: recordings.RecordingSecretProvenanceDeclared}},
	})
	if err != nil || result.RedactedCount != 1 {
		return "", 0, agentmessages.ErrBadRequest
	}
	var safe struct {
		Body string `json:"body"`
	}
	if json.Unmarshal(result.Payload, &safe) != nil || !validID(safe.Body) {
		return "", 0, agentmessages.ErrBadRequest
	}
	return safe.Body, result.RedactedCount, nil
}

// Fingerprint compares normalized admission intent. Recipient resolution and
// inherited reply defaults must be applied by the owner before this call.
func (p Prepared) Fingerprint() (string, error) {
	r := p.Request
	r.RequestID = ""
	encoded, err := json.Marshal(r)
	if err != nil {
		return "", agentmessages.ErrBadRequest
	}
	return digest(encoded), nil
}

func digest(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}
