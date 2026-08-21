package responsestreamremovalgate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReleaseNotesMigrationRelativePath is the packaged migration note for retired
// private CLI response-stream NDJSON record types.
const ReleaseNotesMigrationRelativePath = "docs/release-notes/response-stream-private-ndjson-removal.md"

var releaseNotesRequiredMarkers = []string{
	"no longer emit private NDJSON",
	`"recordType": "progress"`,
	`"recordType": "compaction"`,
	`"recordType": "stream_gap"`,
	`"recordType": "primary_result"`,
	`"recordType": "factory_event"`,
	`"recordType": "invocation_result"`,
	`"event": {`,
	`"response": {`,
	"FactoryEvent",
	"primaryResult",
	"Pre-invocation CLI error envelope",
	`"code":"INVOCATION_OUTPUT_UNSUPPORTED"`,
	"Supported stdout vocabulary",
	"Old → new mapping",
}

var releaseNotesForbiddenMarkers = []string{
	"recordType=response_event",
	`"recordType": "response_event"`,
	"FactoryResponseEvent",
	"event.kind=PROGRESS",
	"event.phase=UPDATED",
}

// AssertReleaseNotesMigrationMapping proves the packaged release note identifies
// retired private NDJSON record types and documents the exact old→new CLI JSON
// mapping required for Batch 09 closeout.
func AssertReleaseNotesMigrationMapping(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	path := filepath.Join(repoRoot, filepath.FromSlash(ReleaseNotesMigrationRelativePath))
	contents, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read release notes %s: %w", ReleaseNotesMigrationRelativePath, err)
	}
	text := string(contents)
	for _, marker := range releaseNotesRequiredMarkers {
		if !strings.Contains(text, marker) {
			return fmt.Errorf("release notes missing migration marker %q", marker)
		}
	}
	for _, marker := range releaseNotesForbiddenMarkers {
		if strings.Contains(text, marker) {
			return fmt.Errorf("release notes contain stale migration marker %q", marker)
		}
	}
	for _, recordType := range PublicNDJSONRecordTypes {
		if !strings.Contains(text, "recordType="+recordType) && !strings.Contains(text, `"recordType": "`+recordType+`"`) {
			return fmt.Errorf("release notes missing supported recordType %q", recordType)
		}
	}
	for _, recordType := range PrivateNDJSONRecordTypes {
		if !strings.Contains(text, `"recordType": "`+recordType+`"`) {
			return fmt.Errorf("release notes missing retired private record example %q", recordType)
		}
	}
	return nil
}
