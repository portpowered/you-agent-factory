package support

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// DecodeInvocationResponseJSON accepts either the legacy single-response JSON
// presentation or the terminal invocation_result from the public NDJSON
// response stream. Packaged-factory tests use it so their assertions remain
// about invocation behavior instead of duplicating CLI presentation parsing.
func DecodeInvocationResponseJSON(t testing.TB, stdout string) factoryapi.InvocationResponse {
	t.Helper()

	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		t.Fatal("invocation stdout is empty")
	}

	var direct factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(trimmed), &direct); err == nil && direct.Status != "" {
		return direct
	}

	type invocationResultRecord struct {
		RecordType string                         `json:"recordType"`
		Response   *factoryapi.InvocationResponse `json:"response"`
	}

	var response *factoryapi.InvocationResponse
	scanner := bufio.NewScanner(bytes.NewBufferString(trimmed))
	line := 0
	for scanner.Scan() {
		line++
		payload := bytes.TrimSpace(scanner.Bytes())
		if len(payload) == 0 {
			continue
		}
		var record invocationResultRecord
		if err := json.Unmarshal(payload, &record); err != nil {
			t.Fatalf("decode invocation NDJSON line %d: %v\nline: %s\nstdout:\n%s", line, err, payload, stdout)
		}
		if record.RecordType == "invocation_result" {
			if record.Response == nil {
				t.Fatalf("invocation_result line %d has no response\nstdout:\n%s", line, stdout)
			}
			response = record.Response
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan invocation NDJSON: %v", err)
	}
	if response == nil {
		t.Fatalf("invocation stdout has no terminal invocation_result\nstdout:\n%s", stdout)
	}
	return *response
}
