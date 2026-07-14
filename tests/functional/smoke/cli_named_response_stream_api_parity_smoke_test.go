package smoke

import (
	"fmt"
	"testing"
	"time"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/packages/subagent"
)

func TestNamedGoalResponseStream_APIInvocationMatchesCLIResponseStreamTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/goal response-stream terminal parity smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	goalText := fmt.Sprintf("functional-smoke-goal-api-cli-stream-parity-%d", time.Now().UnixNano())
	apiResponse := postNamedGoalRoutingInvocationOnServer(t, server, goalText)

	streamStdout, streamStderr, err := runNamedGoalResponseStreamInvocationCLI(t, mockWorkersPath, true, goalText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	streamTerminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", err, streamStdout)
	}

	assertNamedGoalInvocationTerminalOutcomeParity(t, apiResponse, streamTerminal)
}

func TestNamedSubagentResponseStream_APIInvocationMatchesCLIResponseStreamTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI/API named @you/subagent response-stream terminal parity smoke")
	}

	factoryDir := materializeNamedSubagentFactoryForSmoke(t)
	mockWorkersPath := writePackagedSubagentMockWorkers(t)
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	requestText := fmt.Sprintf("functional-smoke-subagent-api-cli-stream-parity-%d", time.Now().UnixNano())
	apiResponse := postNamedGoalRoutingInvocationOnServer(t, server, requestText)

	streamStdout, streamStderr, err := runNamedSubagentResponseStreamInvocationCLI(t, mockWorkersPath, true, requestText)
	if err != nil {
		t.Fatalf("CLI JSON response-stream invocation: %v\nstdout:\n%s\nstderr:\n%s", err, streamStdout, streamStderr)
	}
	records, err := parseNamedGoalResponseStreamNDJSONRecords(streamStdout)
	if err != nil {
		t.Fatalf("parse response-stream NDJSON: %v\nstdout:\n%s", err, streamStdout)
	}
	streamTerminal, err := namedGoalResponseStreamTerminalInvocation(records)
	if err != nil {
		t.Fatalf("response-stream terminal invocation: %v\nstdout:\n%s", err, streamStdout)
	}

	assertNamedGoalInvocationTerminalOutcomeParity(t, apiResponse, streamTerminal)
}

func materializeNamedSubagentFactoryForSmoke(t *testing.T) string {
	t.Helper()

	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), subagent.PackagedFactoryName, subagent.BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory(@you/subagent): %v", err)
	}
	return dir
}
