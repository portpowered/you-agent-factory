package workersessionmcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"
)

type messageInput struct {
	Message          string  `json:"message"`
	RequestID        string  `json:"requestId"`
	WorkerSessionID  *string `json:"workerSessionId"`
	InReplyTo        *string `json:"inReplyTo"`
	FactorySessionID *string `json:"factorySessionId"`
	BodySecret       *bool   `json:"bodySecret"`
	Delivery         *string `json:"delivery"`
	IfEnded          *string `json:"ifEnded"`
	ReplyIfEnded     *string `json:"replyIfEnded"`
	ExpiresInSeconds *int    `json:"expiresInSeconds"`
	Correlation      *struct {
		WorkID           *string `json:"workId"`
		FactorySessionID *string `json:"factorySessionId"`
	} `json:"correlation"`
}

type reviveInput struct {
	WorkerSessionID          string  `json:"workerSessionId"`
	Message                  string  `json:"message"`
	RequestID                string  `json:"requestId"`
	SuccessorWorkerSessionID string  `json:"successorWorkerSessionId"`
	FactorySessionID         *string `json:"factorySessionId"`
}

type messageReadInput struct {
	Messages                    bool      `json:"messages"`
	WorkerSessionID             *string   `json:"workerSessionId"`
	FactorySessionID            *string   `json:"factorySessionId"`
	ThreadID                    *string   `json:"threadId"`
	Status                      *[]string `json:"status"`
	MaxResults                  *int      `json:"maxResults"`
	NextToken                   *string   `json:"nextToken"`
	MarkRead                    *bool     `json:"markRead"`
	FromWorkerSessionID         *string   `json:"fromWorkerSessionId"`
	CorrelationWorkID           *string   `json:"correlationWorkId"`
	CorrelationFactorySessionID *string   `json:"correlationFactorySessionId"`
}

// Inspect members recursively before the Go decoder can discard duplicate
// properties, accept case aliases, or replace malformed UTF-8.
func validateMessageObject(raw json.RawMessage, allowed []string) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("arguments must be valid UTF-8")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return fmt.Errorf("arguments must be one JSON object")
	}
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		key, ok := token.(string)
		if !validMessageMember(err, ok, seen[key], key, allowed) {
			return fmt.Errorf("arguments contain duplicate or unknown properties")
		}
		seen[key] = true
		var value json.RawMessage
		if decoder.Decode(&value) != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return fmt.Errorf("arguments contain an invalid value")
		}
		if key == "correlation" {
			if err := validateMessageObject(value, []string{"workId", "factorySessionId"}); err != nil {
				return err
			}
		}
	}
	if token, err := decoder.Token(); err != nil || token != json.Delim('}') || decoder.Decode(new(any)) != io.EOF {
		return fmt.Errorf("arguments must be one JSON object")
	}
	return nil
}

func nonemptyMessageStrings(values ...*string) error {
	for _, value := range values {
		if value != nil && (!utf8.ValidString(*value) || strings.TrimSpace(*value) == "") {
			return fmt.Errorf("explicit strings must be nonempty UTF-8")
		}
	}
	return nil
}

func decodeMessage(raw json.RawMessage) (messageInput, error) {
	var input messageInput
	if err := validateMessageObject(raw, []string{"message", "requestId", "workerSessionId", "inReplyTo", "factorySessionId", "bodySecret", "delivery", "ifEnded", "replyIfEnded", "expiresInSeconds", "correlation"}); err != nil {
		return input, err
	}
	if _, err := decodeInput(raw, &input); err != nil {
		return input, err
	}
	if strings.TrimSpace(input.Message) == "" || len(input.Message) > 8192 || strings.TrimSpace(input.RequestID) == "" || utf8.RuneCountInString(input.RequestID) > 200 {
		return input, fmt.Errorf("MESSAGE requires message (at most 8192 bytes) and requestId (at most 200 characters)")
	}
	if (input.WorkerSessionID == nil) == (input.InReplyTo == nil) {
		return input, fmt.Errorf("MESSAGE requires exactly one workerSessionId or inReplyTo")
	}
	if err := nonemptyMessageStrings(input.WorkerSessionID, input.InReplyTo, input.FactorySessionID); err != nil {
		return input, err
	}
	if input.Correlation != nil {
		if err := nonemptyMessageStrings(input.Correlation.WorkID, input.Correlation.FactorySessionID); err != nil {
			return input, err
		}
	}
	return input, validateMessageOptions(input)
}

func validateMessageOptions(input messageInput) error {
	if input.Delivery != nil && !slices.Contains([]string{"QUEUE", "INTERRUPT"}, *input.Delivery) {
		return fmt.Errorf("delivery must be QUEUE or INTERRUPT")
	}
	for _, policy := range []*string{input.IfEnded, input.ReplyIfEnded} {
		if policy != nil && !slices.Contains([]string{"HOLD", "REVIVE"}, *policy) {
			return fmt.Errorf("ended policy must be HOLD or REVIVE")
		}
	}
	if input.ExpiresInSeconds != nil && (*input.ExpiresInSeconds < 60 || *input.ExpiresInSeconds > 604800) {
		return fmt.Errorf("expiresInSeconds must be between 60 and 604800")
	}
	return nil
}

func decodeRevive(raw json.RawMessage) (reviveInput, error) {
	var input reviveInput
	if err := validateMessageObject(raw, []string{"workerSessionId", "message", "requestId", "successorWorkerSessionId", "factorySessionId"}); err != nil {
		return input, err
	}
	if _, err := decodeInput(raw, &input); err != nil {
		return input, err
	}
	if err := nonemptyMessageStrings(&input.WorkerSessionID, &input.Message, &input.RequestID, &input.SuccessorWorkerSessionID, input.FactorySessionID); err != nil {
		return input, err
	}
	return input, nil
}

func decodeMessageRead(raw json.RawMessage, action string) (messageReadInput, error) {
	var input messageReadInput
	allowed := []string{"messages", "factorySessionId", "threadId", "status", "maxResults", "nextToken", "markRead"}
	if action == ActionRead {
		allowed = append(allowed, "workerSessionId")
	} else {
		allowed = append(allowed, "fromWorkerSessionId", "correlationWorkId", "correlationFactorySessionId")
	}
	if err := validateMessageObject(raw, allowed); err != nil {
		return input, err
	}
	if _, err := decodeInput(raw, &input); err != nil {
		return input, err
	}
	if !input.Messages || (action == ActionRead && input.WorkerSessionID == nil) {
		return input, fmt.Errorf("message inspection requires messages=true and READ requires workerSessionId")
	}
	if err := nonemptyMessageStrings(input.WorkerSessionID, input.FactorySessionID, input.ThreadID, input.NextToken, input.FromWorkerSessionID, input.CorrelationWorkID, input.CorrelationFactorySessionID); err != nil {
		return input, err
	}
	if input.MaxResults != nil && (*input.MaxResults < 1 || *input.MaxResults > 500) {
		return input, fmt.Errorf("maxResults must be between 1 and 500")
	}
	if input.Status != nil {
		for _, status := range *input.Status {
			if !slices.Contains([]string{"QUEUED", "READ", "DELIVERED_REVIVE", "REPLIED", "EXPIRED"}, status) {
				return input, fmt.Errorf("status must be a public Agent Message status")
			}
		}
	}
	return input, nil
}

func validMessageMember(err error, isString, duplicate bool, key string, allowed []string) bool {
	return err == nil && isString && !duplicate && slices.Contains(allowed, key)
}
