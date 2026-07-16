package pi

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

// FinalResult is the authoritative Pi terminal response.
type FinalResult struct {
	Content         string
	ProviderSession *workerexecution.ProviderSessionMetadata
}

type nativeRecord struct {
	Type     string          `json:"type"`
	ID       string          `json:"id"`
	Message  *nativeMessage  `json:"message"`
	Messages []nativeMessage `json:"messages"`
}

// parseFinalOutput derives the authoritative response independently from streamed observations.
func parseFinalOutput(stdout []byte) (FinalResult, error) {
	var result FinalResult
	forEachRecord(stdout, func(raw []byte) {
		var record nativeRecord
		if json.Unmarshal(raw, &record) != nil {
			return
		}
		switch record.Type {
		case "session":
			if session := providerSession(record.ID); session != nil {
				result.ProviderSession = session
			}
		case "message_end":
			if record.Message != nil && strings.EqualFold(strings.TrimSpace(record.Message.Role), "assistant") {
				if text := assistantText(*record.Message); text != "" {
					result.Content = text
				}
			}
		case "agent_end":
			if text := lastAssistantText(record.Messages); text != "" {
				result.Content = text
			}
		}
	})
	if strings.TrimSpace(result.Content) == "" {
		return FinalResult{}, errors.New("Pi JSONL output did not contain a completed assistant message")
	}
	return result, nil
}

func parseTerminalFailure(stdout []byte) error {
	var failureMessage string
	forEachRecord(stdout, func(raw []byte) {
		if failureMessage != "" {
			return
		}
		var record nativeRecord
		if json.Unmarshal(raw, &record) != nil {
			return
		}
		if record.Type != "message_end" || record.Message == nil {
			return
		}
		stopReason := strings.TrimSpace(record.Message.StopReason)
		if stopReason != "error" && stopReason != "aborted" {
			return
		}
		if text := strings.TrimSpace(record.Message.ErrorMessage); text != "" {
			failureMessage = text
			return
		}
		failureMessage = "Pi returned a terminal assistant failure"
	})
	if failureMessage == "" {
		return nil
	}
	return &piTerminalError{message: failureMessage}
}

type piTerminalError struct{ message string }

func (e *piTerminalError) Error() string { return e.message }

func forEachRecord(output []byte, visit func([]byte)) {
	normalized := bytes.ReplaceAll(output, []byte("\r\n"), []byte("\n"))
	for len(normalized) > 0 {
		newline := bytes.IndexByte(normalized, '\n')
		if newline < 0 {
			if trimmed := bytes.TrimSpace(normalized); len(trimmed) > 0 && len(trimmed) <= maximumBufferedRecordBytes {
				visit(trimmed)
			}
			return
		}
		if trimmed := bytes.TrimSpace(normalized[:newline]); len(trimmed) > 0 && len(trimmed) <= maximumBufferedRecordBytes {
			visit(trimmed)
		}
		normalized = normalized[newline+1:]
	}
}

func lastAssistantText(messages []nativeMessage) string {
	for index := len(messages) - 1; index >= 0; index-- {
		if !strings.EqualFold(strings.TrimSpace(messages[index].Role), "assistant") {
			continue
		}
		if text := assistantText(messages[index]); text != "" {
			return text
		}
	}
	return ""
}
