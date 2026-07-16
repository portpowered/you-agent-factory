package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

const ProviderSessionKindSessionID = "session_id"

type FinalResult struct {
	Content         string
	ProviderSession *workerexecution.ProviderSessionMetadata
}

// ParseFinalOutput derives the authoritative response independently from any
// streamed decoder observations.
func ParseFinalOutput(stdout []byte) (FinalResult, error) {
	var result FinalResult
	forEachBoundedRecord(stdout, func(raw []byte) {
		var record recordEnvelope
		if json.Unmarshal(raw, &record) != nil {
			return
		}
		switch record.Type {
		case "thread.started":
			if id := strings.TrimSpace(record.ThreadID); id != "" {
				result.ProviderSession = &workerexecution.ProviderSessionMetadata{Provider: "codex", Kind: ProviderSessionKindSessionID, ID: id}
			}
		case "item.completed":
			var item itemEnvelope
			if json.Unmarshal(record.Item, &item) == nil && item.Type == "agent_message" {
				result.Content = item.Text
			}
		}
	})
	if result.Content == "" {
		return FinalResult{}, errors.New("codex JSONL output did not contain a completed agent message")
	}
	return result, nil
}

func forEachBoundedRecord(output []byte, visit func([]byte)) {
	for len(output) > 0 {
		newline := bytes.IndexByte(output, '\n')
		if newline < 0 {
			newline = len(output)
		}
		if newline <= maxJSONLRecordBytes {
			visit(output[:newline])
		}
		if newline == len(output) {
			return
		}
		output = output[newline+1:]
	}
}
