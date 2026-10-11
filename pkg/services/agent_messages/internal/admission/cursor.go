package admission

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"

	agentmessages "github.com/portpowered/infinite-you/pkg/services/agent_messages"
	"github.com/portpowered/infinite-you/pkg/services/agent_messages/internal/store"
)

const (
	minCursorKeyBytes = 32
	maxCursorBytes    = 2048
)

type pagePosition struct {
	Query   string `json:"query"`
	After   uint64 `json:"after"`
	Through uint64 `json:"through"`
}

func (e *Engine) pageCursor(r agentmessages.ListRequest, identity Identity, sequence uint64) (pagePosition, error) {
	if len(e.cursorKey) < minCursorKeyBytes {
		return pagePosition{}, store.ErrUnavailable
	}
	token := r.NextToken
	r.NextToken = ""
	// Credentials never enter a token or its hash. Exact caller, verified
	// chain/Work and normalized query bind it to this authorized observation.
	worker := ""
	if r.Caller != nil {
		worker = r.Caller.WorkerSessionID
	}
	r.Caller = nil
	encoded, err := json.Marshal(struct {
		Request agentmessages.ListRequest
		Worker  string
		Chain   string
		Work    string
	}{r, worker, identity.Chain, identity.Work})
	if err != nil {
		return pagePosition{}, agentmessages.ErrBadRequest
	}
	query := digest(encoded)
	if token == "" {
		return pagePosition{Query: query, Through: sequence}, nil
	}
	cursor, err := e.decodeCursor(token)
	if err != nil || cursor.Query != query || cursor.Through > sequence || cursor.After == 0 || cursor.After >= cursor.Through {
		return pagePosition{}, agentmessages.ErrCursorInvalid
	}
	return cursor, nil
}

func (e *Engine) encodeCursor(cursor pagePosition) (string, error) {
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", agentmessages.ErrCursorInvalid
	}
	signature := e.cursorSignature(encoded)
	return base64.RawURLEncoding.EncodeToString(encoded) + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func (e *Engine) decodeCursor(token string) (pagePosition, error) {
	if len(token) > maxCursorBytes {
		return pagePosition{}, agentmessages.ErrCursorInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return pagePosition{}, agentmessages.ErrCursorInvalid
	}
	encoded, err := base64.RawURLEncoding.Strict().DecodeString(parts[0])
	if err != nil {
		return pagePosition{}, agentmessages.ErrCursorInvalid
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, e.cursorSignature(encoded)) {
		return pagePosition{}, agentmessages.ErrCursorInvalid
	}
	var cursor pagePosition
	if json.Unmarshal(encoded, &cursor) != nil {
		return pagePosition{}, agentmessages.ErrCursorInvalid
	}
	return cursor, nil
}

func (e *Engine) cursorSignature(encoded []byte) []byte {
	mac := hmac.New(sha256.New, e.cursorKey)
	_, _ = mac.Write(encoded)
	return mac.Sum(nil)
}
