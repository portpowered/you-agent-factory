package cli_rest_journeys_test

import (
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

func addProviderSessionReadRoutes(t *testing.T, home string, routes map[string]platformprocess.CommandResult, codexStdout []byte) {
	t.Helper()
	for _, test := range providerSessionReadCases {
		stdout := bytesReplaceAll(codexStdout, workerSessionsCodexSuccessID, test.id)
		switch {
		case strings.Contains(test.name, "truncated"):
			writeCodexRollout(t, home, test.id, []byte(
				"{\"type\":\"event_msg\",\"payload\":{\"type\":\"agent_message\",\"message\":\"retained fixture answer\"}}\n"+
					"{\"type\":\"response_item\",\"payload\":{\"type\":\"message\",\"content\":[{\"text\":\"partial"))
		case !strings.Contains(test.name, "missing"):
			writeCodexRollout(t, home, test.id, []byte("{}\n"))
		}
		routes[test.id] = platformprocess.CommandResult{Stdout: stdout}
	}
}
