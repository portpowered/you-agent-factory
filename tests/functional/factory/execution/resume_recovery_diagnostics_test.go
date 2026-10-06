package execution_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	definitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The lane's safety contract explicitly requires mock workers for every resume.
// This is the scoped TEST-SAFETY-001 exception; both execution edges also deny
// fallback. All cases reuse one process and own their profile and session.
func TestResumeRecovery(t *testing.T) {
	t.Parallel()
	var routes sync.Map
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: deniedRunner{}, ScriptCommandRunner: deniedRunner{},
		APIServerStarter: func(ctx context.Context, request platformhttpserver.StartRequest) error {
			if route, ok := routes.Load(request.Port); ok {
				server := httptest.NewServer(request.Handler)
				defer server.Close()
				if request.OnBound != nil {
					request.OnBound(platformhttpserver.Binding{Port: server.Listener.Addr().(*net.TCPAddr).Port})
				}
				route.(chan string) <- server.URL
				<-ctx.Done()
				return nil
			}
			// Transport binding precedes Runtime readiness. Keep that independent
			// edge alive until startup cancels it so it cannot mask restore errors.
			if request.OnBound != nil {
				request.OnBound(platformhttpserver.Binding{Port: request.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	t.Run("F01/consumed-accepted-cron", func(t *testing.T) {
		t.Parallel()
		assertAcceptedCronBoard(t, process, &routes)
	})
	t.Run("F06/missing-cron-identity", func(t *testing.T) {
		t.Parallel()
		assertRejectedResume(t, process, false, missingCronIdentityPayload, []string{"historical-cron", "no current place occupancy"})
	})
	t.Run("F02/consumed-failed-cron", func(t *testing.T) {
		t.Parallel()
		assertFailedCronBoard(t, process, &routes)
	})
	t.Run("F07/successor", func(t *testing.T) {
		t.Parallel()
		assertFailedCronSuccessor(t, process, &routes)
	})
	t.Run("F03/interrupted-cron", func(t *testing.T) {
		t.Parallel()
		assertInterruptedCron(t, process, &routes)
	})
	t.Run("F04/operator-move", func(t *testing.T) {
		t.Parallel()
		assertMoveRoundTrip(t, process, &routes)
	})
	for _, debug := range []bool{false, true} {
		t.Run(fmt.Sprintf("F05/debug=%t", debug), func(t *testing.T) {
			t.Parallel()
			assertCorruptResumeDiagnostic(t, process, debug)
		})
	}
}

func assertCorruptResumeDiagnostic(t *testing.T, process support.Process, debug bool) {
	assertRejectedResume(t, process, debug, corruptResumePayload, []string{"work-corrupt", "task:missing", "not present in the current Factory topology"})
}

func missingCronIdentityPayload(t *testing.T, session string) []byte {
	var artifact definitions.ReplayArtifact
	if err := json.Unmarshal(cronPayload(t, session, workers.OutcomeFailed), &artifact); err != nil {
		t.Fatal(err)
	}
	var admission work.WorkRequestEventPayload
	if err := json.Unmarshal(artifact.Events[1].Payload, &admission); err != nil {
		t.Fatal(err)
	}
	admission.Works[0].Tags = nil
	artifact.Events[1].Payload, _ = json.Marshal(admission)
	data, err := json.Marshal(artifact)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertRejectedResume(t *testing.T, process support.Process, debug bool, fixture func(*testing.T, string) []byte, wants []string) {
	t.Helper()
	dir, home := support.ScaffoldFactory(t, corruptResumeFactory()), t.TempDir()
	inputPath, successor := filepath.Join(dir, "corrupt.json"), filepath.Join(dir, "successor.jsonl")
	sessionID := uuid.NewString()
	payload := fixture(t, sessionID)
	if err := os.WriteFile(inputPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"you", "run", "--continuously", "--session", sessionID, "--resume", inputPath, "--dir", dir, "--record", successor,
		"--with-mock-workers", "--with-server", "--listen", fmt.Sprintf("127.0.0.1:%d", port), "--quiet"}
	if debug {
		args = append(args, "--debug")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Stdin = strings.NewReader("")
	inputs.Input.Env = append(os.Environ(), "HOME="+home, "USERPROFILE="+home, "HOMEDRIVE=", "HOMEPATH=")
	if err := process.Execute(inputs.Input); err == nil {
		t.Fatal("corrupt resume succeeded")
	}
	response := requireStartupCLIDiagnostic(t, inputs.Stderr())
	if string(response.Code) != "SERVER_START_FAILED" || response.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("response = %#v, want SERVER_START_FAILED/internal", response)
	}
	for _, want := range wants {
		if !strings.Contains(response.Message, want) {
			t.Fatalf("resume diagnostic %q missing %q", response.Message, want)
		}
	}
	if strings.Contains(inputs.Stderr()+inputs.Stdout(), "PRIVATE-PROMPT") {
		t.Fatal("resume output leaked synthetic prompt")
	}
	if after, err := os.ReadFile(inputPath); err != nil || !bytes.Equal(after, payload) {
		t.Fatalf("resume changed source recording: %v", err)
	}
	if inputs.Stdout() != "" {
		t.Fatalf("corrupt resume published readiness/output: %q", inputs.Stdout())
	}
	if released, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port)); err != nil {
		t.Fatalf("resume did not release scenario port: %v", err)
	} else {
		_ = released.Close()
	}
}

func corruptResumeFactory() map[string]any {
	return map[string]any{
		"name": "synthetic-corrupt-resume",
		"workTypes": []map[string]any{{"name": "task", "states": []map[string]string{
			{"name": "ready", "type": "INITIAL"}, {"name": "complete", "type": "TERMINAL"},
		}}},
		"workers": []any{}, "workstations": []any{},
	}
}

func corruptResumePayload(t *testing.T, sessionID string) []byte {
	t.Helper()
	snapshot, err := definitions.NewFactorySnapshot(corruptResumeFactory())
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	values := []struct {
		kind    definitions.FactoryEventType
		payload any
	}{
		{definitions.FactoryEventTypeRunRequest, definitions.RunRequestEventPayload{Factory: snapshot, RecordedAt: base}},
		{definitions.FactoryEventTypeWorkRequest, work.WorkRequestEventPayload{
			Type: work.WorkRequestTypeFactoryRequestBatch,
			Works: []work.WorkRequestEventWork{{WorkID: "work-corrupt", RequestID: "request-corrupt", Name: "synthetic", WorkTypeID: "task",
				State:   &work.WorkEventState{Name: "missing", Type: "PROCESSING"},
				Content: []work.WorkContentPart{{Type: "text", Text: "PRIVATE-PROMPT"}},
			}},
		}},
	}
	events := make([]definitions.FactoryEvent, 0, len(values))
	for i, value := range values {
		data, err := json.Marshal(value.payload)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, definitions.FactoryEvent{Id: fmt.Sprintf("synthetic/%d", i), Type: value.kind,
			SchemaVersion: definitions.FactoryEventSchemaVersionV1, Payload: data,
			Context: definitions.FactoryEventContext{EventTime: base.Add(time.Duration(i) * time.Second),
				Sequence: i, Tick: i, SessionID: &sessionID},
		})
	}
	payload, err := json.Marshal(definitions.ReplayArtifact{SchemaVersion: definitions.ReplayV1SourceFormat, RecordedAt: base, Events: events})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

type deniedRunner struct{}

func (deniedRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, fmt.Errorf("resume diagnostics may not execute a worker or script")
}

// requireStartupCLIDiagnostic accepts one envelope followed only by the bounded
// local startup cause contract. Callers retain scenario-specific privacy checks.
// Ordinary and remote diagnostics should continue using strict JSON decoding.
func requireStartupCLIDiagnostic(t testing.TB, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(stderr), "\n")
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(lines[0]), &response); err != nil {
		t.Fatalf("decode startup envelope: %v; stderr=%q", err, stderr)
	}
	if response.Code == "" || response.Family == "" || response.Message == "" {
		t.Fatalf("incomplete startup envelope: %#v", response)
	}
	if len(lines) > 18 {
		t.Fatalf("startup causes exceed 16 nodes plus truncation: %q", stderr)
	}
	for index, line := range lines[1:] {
		prefix := fmt.Sprintf("cause[%d]=", index)
		if !strings.HasPrefix(line, prefix) {
			t.Fatalf("unexpected trailing startup diagnostic: %q", line)
		}
		cause := strings.TrimPrefix(line, prefix)
		if cause == "" || len(cause) > 515 || unsafeStartupCause.MatchString(cause) {
			t.Fatalf("unbounded or unsafe startup cause: %q", line)
		}
	}
	return response
}

var unsafeStartupCause = regexp.MustCompile(`(?i)(?:^|[\s=("'])(?:[A-Za-z]:[\\/]|\\\\|\.\.?[\\/]|~/|/)[^\s]+|https?://[^\s]*[?@#]|\b(?:password|secret|token|prompt|payload|body|authorization)\s*[:=]\s*(?:[^<\s]|<(?:[^r]|r[^e]))`)
