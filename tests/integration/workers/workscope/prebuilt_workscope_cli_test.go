package workscope_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func assertPrebuiltWorkscopeCLIParity(
	t *testing.T,
	ctx context.Context,
	binaryPath, workspace string,
	environment []string,
	serverURL string,
	journey prebuiltWorkscopeJourney,
) (string, int) {
	t.Helper()
	commands := 0
	listOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, workspace, environment,
		"worker-sessions", "list", "--server", serverURL,
		"--session", journey.factorySessionSelector, "--work-id", prebuiltWorkscopeWorkID, "--output", "json",
	)
	commands++
	var cliList factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal(listOutput, &cliList); err != nil {
		t.Fatalf("decode prebuilt CLI Work-scoped list: %v\nstdout=%s", err, listOutput)
	}
	if !reflect.DeepEqual(cliList, journey.list) {
		t.Fatalf("prebuilt CLI Work-scoped rows differ from HTTP:\nCLI=%#v\nHTTP=%#v", cliList, journey.list)
	}
	showOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, workspace, environment,
		"worker-sessions", "show", "--server", serverURL,
		"--session", journey.factorySessionSelector, "--worker-session-id", prebuiltWorkscopeWorkerSession, "--json",
	)
	commands++
	var cliDetail factoryapi.WorkerSessionObservation
	if err := json.Unmarshal(showOutput, &cliDetail); err != nil {
		t.Fatalf("decode prebuilt CLI stable-ID show: %v\nstdout=%s", err, showOutput)
	}
	if journey.detail.FactorySessionId == nil || *journey.detail.FactorySessionId != journey.factorySessionSelector {
		t.Fatalf("prebuilt HTTP stable-ID observation Factory Session selector = %#v, want %q", journey.detail.FactorySessionId, journey.factorySessionSelector)
	}
	if cliDetail.FactorySessionId == nil || strings.TrimSpace(*cliDetail.FactorySessionId) == "" {
		t.Fatalf("prebuilt CLI stable-ID observation omitted resolved Factory Session ID: %#v", cliDetail.FactorySessionId)
	}
	// The HTTP route preserves the public ~default selector in this field;
	// the CLI resolves that selector and prints its concrete runtime identity.
	// Compare every other response field and require the resolved identity to
	// remain stable across the copied-ledger restart below.
	httpDetail := journey.detail
	httpDetail.FactorySessionId = cliDetail.FactorySessionId
	if !reflect.DeepEqual(cliDetail, httpDetail) {
		cliJSON, _ := json.Marshal(cliDetail)
		httpJSON, _ := json.Marshal(httpDetail)
		t.Fatalf("prebuilt CLI stable-ID observation differs from HTTP:\nCLI=%s\nHTTP=%s", cliJSON, httpJSON)
	}
	readOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, workspace, environment,
		"worker-sessions", "read", "--server", serverURL,
		"--session", journey.factorySessionSelector, "--worker-session-id", prebuiltWorkscopeWorkerSession, "--json",
	)
	commands++
	var cliTranscript factoryapi.WorkerSessionTranscriptResponse
	if err := json.Unmarshal(readOutput, &cliTranscript); err != nil {
		t.Fatalf("decode prebuilt CLI stable-ID transcript: %v\nstdout=%s", err, readOutput)
	}
	httpTranscript := journey.transcript
	if cliTranscript.FactorySessionId == nil || strings.TrimSpace(*cliTranscript.FactorySessionId) == "" {
		t.Fatalf("prebuilt CLI transcript omitted resolved Factory Session ID: %#v", cliTranscript)
	}
	// As with the observation, the HTTP route keeps the public selector while
	// the CLI reports its concrete resolved Factory Session identity.
	httpTranscript.FactorySessionId = cliTranscript.FactorySessionId
	if !reflect.DeepEqual(cliTranscript, httpTranscript) {
		cliJSON, _ := json.Marshal(cliTranscript)
		httpJSON, _ := json.Marshal(httpTranscript)
		t.Fatalf("prebuilt CLI stable-ID transcript differs from HTTP:\nCLI=%s\nHTTP=%s", cliJSON, httpJSON)
	}
	streamOutput := runPrebuiltWorkscopeCLI(t, ctx, binaryPath, workspace, environment,
		"worker-sessions", "stream", "--server", serverURL,
		"--session", journey.factorySessionSelector, "--worker-session-id", prebuiltWorkscopeWorkerSession,
		"--replay-only", "--json",
	)
	commands++
	cliEvents := decodePrebuiltWorkscopeCLIEvents(t, streamOutput)
	assertPrebuiltWorkscopeEventParity(t, cliEvents, journey.events)
	return *cliDetail.FactorySessionId, commands
}

type prebuiltWorkscopeCLIEvent struct {
	Position       uint64          `json:"position"`
	SourceType     string          `json:"sourceType"`
	SourceID       string          `json:"sourceId"`
	SourceSequence uint64          `json:"sourceSequence"`
	SourceEventID  string          `json:"sourceEventId"`
	SchemaID       string          `json:"schemaId"`
	Payload        json.RawMessage `json:"payload"`
}

type prebuiltWorkscopeCLIFrame struct {
	WorkerSessionID  string                                      `json:"workerSessionId"`
	FactorySessionID string                                      `json:"factorySessionId"`
	ProviderSession  *factoryapi.WorkerSessionProviderSessionRef `json:"providerSession"`
	WorkIDs          []string                                    `json:"workIds"`
	Event            *prebuiltWorkscopeCLIEvent                  `json:"event"`
	ReplaySummary    *prebuiltWorkscopeCLIReplaySummary          `json:"replaySummary,omitempty"`
}

type prebuiltWorkscopeCLIReplaySummary struct {
	Kind          string `json:"kind"`
	Complete      bool   `json:"complete"`
	EventsEmitted int64  `json:"eventsEmitted"`
}

func runPrebuiltWorkscopeCLI(
	t *testing.T,
	ctx context.Context,
	binaryPath, workspace string,
	environment []string,
	arguments ...string,
) []byte {
	t.Helper()
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	command := exec.CommandContext(commandCtx, binaryPath, arguments...)
	command.Dir = workspace
	command.Env = append([]string(nil), environment...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("prebuilt Work Session CLI %q: %v\nstdout=%s\nstderr=%s", arguments, err, stdout.String(), stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("prebuilt Work Session CLI %q wrote stderr: %s", arguments, stderr.String())
	}
	return append([]byte(nil), stdout.Bytes()...)
}

func decodePrebuiltWorkscopeCLIEvents(t *testing.T, output []byte) []prebuiltWorkscopeCLIFrame {
	t.Helper()
	frames := make([]prebuiltWorkscopeCLIFrame, 0)
	complete := false
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var frame prebuiltWorkscopeCLIFrame
		if err := json.Unmarshal([]byte(line), &frame); err == nil && frame.Event != nil {
			frames = append(frames, frame)
			if frame.ReplaySummary != nil && frame.ReplaySummary.Complete {
				complete = true
			}
			continue
		}
		var summary struct {
			Kind     string `json:"kind"`
			Complete bool   `json:"complete"`
		}
		if err := json.Unmarshal([]byte(line), &summary); err != nil || summary.Kind != "replay-summary" {
			t.Fatalf("decode prebuilt CLI stream record: %v\nline=%s", err, line)
		}
		complete = summary.Complete
	}
	if len(frames) == 0 || !complete {
		t.Fatalf("prebuilt CLI Worker Session stream has %d events and complete=%t:\n%s", len(frames), complete, output)
	}
	return frames
}

func assertPrebuiltWorkscopeEventParity(
	t *testing.T,
	cliEvents []prebuiltWorkscopeCLIFrame,
	httpEvents []factoryapi.WorkerSessionEvent,
) {
	t.Helper()
	if len(cliEvents) != len(httpEvents) {
		t.Fatalf("prebuilt CLI/HTTP retained event count differs: CLI=%d HTTP=%d", len(cliEvents), len(httpEvents))
	}
	for index, cli := range cliEvents {
		httpEvent := httpEvents[index]
		var cliPayload, httpPayload any
		if err := json.Unmarshal(cli.Event.Payload, &cliPayload); err != nil {
			t.Fatalf("decode prebuilt CLI event payload[%d]: %v", index, err)
		}
		httpPayload = httpEvent.Event.Payload
		if cli.WorkerSessionID != httpEvent.WorkerSessionId || cli.FactorySessionID != stringValue(httpEvent.FactorySessionId) ||
			cli.ProviderSession == nil || !reflect.DeepEqual(*cli.ProviderSession, httpEvent.ProviderSession) ||
			!reflect.DeepEqual(cli.WorkIDs, httpEvent.WorkIds) || cli.Event.Position != uint64(httpEvent.Event.Position) ||
			cli.Event.SourceType != httpEvent.Event.SourceType || cli.Event.SourceID != httpEvent.Event.SourceId ||
			cli.Event.SourceSequence != uint64(httpEvent.Event.SourceSequence) || cli.Event.SourceEventID != httpEvent.Event.SourceEventId ||
			cli.Event.SchemaID != httpEvent.Event.SchemaId || !reflect.DeepEqual(cliPayload, httpPayload) {
			t.Fatalf("prebuilt CLI/HTTP retained event[%d] differs: CLI=%#v HTTP=%#v", index, cli, httpEvent)
		}
	}
}

func assertPrebuiltWorkscopeRestartEqual(t *testing.T, before, after prebuiltWorkscopeJourney) {
	t.Helper()
	if !reflect.DeepEqual(before.list, after.list) {
		t.Fatalf("prebuilt Work-scoped rows changed after copied restart:\nbefore=%#v\nafter=%#v", before.list, after.list)
	}
	if !reflect.DeepEqual(before.detail, after.detail) {
		beforeJSON, _ := json.Marshal(before.detail)
		afterJSON, _ := json.Marshal(after.detail)
		t.Fatalf("prebuilt stable-ID observation changed after copied restart:\nbefore=%s\nafter=%s", beforeJSON, afterJSON)
	}
	if !reflect.DeepEqual(before.transcript, after.transcript) {
		t.Fatalf("prebuilt Provider Session transcript changed after copied restart:\nbefore=%#v\nafter=%#v", before.transcript, after.transcript)
	}
	if len(before.events) != len(after.events) {
		t.Fatalf("prebuilt retained event count changed after copied restart: before=%d after=%d", len(before.events), len(after.events))
	}
	for index := range before.events {
		left, right := before.events[index], after.events[index]
		left.Event.Cursor.StreamGenerationId = nil
		right.Event.Cursor.StreamGenerationId = nil
		left.ReplaySummary, right.ReplaySummary = nil, nil
		if !reflect.DeepEqual(left, right) {
			t.Fatalf("prebuilt retained event[%d] changed after copied restart:\nbefore=%#v\nafter=%#v", index, left, right)
		}
	}
}

func normalizePrebuiltWorkscopeFactorySession(journey *prebuiltWorkscopeJourney, factorySessionID string) {
	if journey == nil {
		return
	}
	for index := range journey.list.Sessions {
		journey.list.Sessions[index].FactorySessionId = &factorySessionID
	}
	journey.detail.FactorySessionId = &factorySessionID
	journey.transcript.FactorySessionId = &factorySessionID
	for index := range journey.events {
		journey.events[index].FactorySessionId = &factorySessionID
	}
}
