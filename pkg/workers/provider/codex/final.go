package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const ProviderSessionKindSessionID = "session_id"

type FinalResult struct {
	Content         string
	ProviderSession *interfaces.ProviderSessionMetadata
}

// ParseFinalOutput derives the authoritative response independently from any
// streamed decoder observations.
func ParseFinalOutput(stdout []byte) (FinalResult, error) {
	var result FinalResult
	scanner := bufio.NewScanner(bytes.NewReader(stdout))
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)
	for scanner.Scan() {
		var record recordEnvelope
		if json.Unmarshal(scanner.Bytes(), &record) != nil {
			continue
		}
		switch record.Type {
		case "thread.started":
			if id := strings.TrimSpace(record.ThreadID); id != "" {
				result.ProviderSession = &interfaces.ProviderSessionMetadata{Provider: "codex", Kind: ProviderSessionKindSessionID, ID: id}
			}
		case "item.completed":
			var item itemEnvelope
			if json.Unmarshal(record.Item, &item) == nil && item.Type == "agent_message" {
				result.Content = item.Text
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return FinalResult{}, err
	}
	if result.Content == "" {
		return FinalResult{}, errors.New("codex JSONL output did not contain a completed agent message")
	}
	return result, nil
}
