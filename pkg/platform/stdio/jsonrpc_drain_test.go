package stdio

import (
	"bytes"
	"context"
	"encoding/json"
	"sync"
	"testing"
)

func TestDrainWriterSerializesConcurrentJSONRPCResponses(t *testing.T) {
	var output bytes.Buffer
	drain := &responseDrain{ctx: context.Background(), changed: make(chan struct{})}
	writer := &drainWriter{Writer: &output, drain: drain}

	const count = 64
	var wait sync.WaitGroup
	for id := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()
			line, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
			if err != nil {
				t.Errorf("Marshal() error = %v", err)
				return
			}
			line = append(line, '\n')
			if _, err := writer.Write(line); err != nil {
				t.Errorf("Write() error = %v", err)
			}
		}()
	}
	wait.Wait()

	seen := make(map[int]bool, count)
	for _, line := range bytes.Split(bytes.TrimSpace(output.Bytes()), []byte("\n")) {
		var response struct {
			ID int `json:"id"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("output contains malformed or interleaved JSON-RPC: %q: %v", line, err)
		}
		seen[response.ID] = true
	}
	if len(seen) != count {
		t.Fatalf("received %d complete responses, want %d", len(seen), count)
	}
}
