package batch_contract_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/internal/functionalevidence"
)

// TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation proves you submit batch
// --dry-run prints a readable summary of the parsed batch and does not upsert
// Work: an instrumented Factory Session server receives no HTTP traffic while
// output includes request identity, batch source, and dry-run markers.
func TestCLISubmitBatchDryRunEmitsSummaryWithoutMutation(t *testing.T) {
	serverURL, requests := newInstrumentedSubmitBatchServer(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{})
	process := buildBatchContractProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})

	stdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", serverURL,
		"submit", "batch", "--dry-run", dryRunInlineBatchJSON(),
	})

	for _, marker := range []string{
		"requestId: " + dryRunSubmitBatchRequestID,
		"batchSource: inline",
		"work count: 1",
		"works: " + dryRunSubmitBatchWorkName,
		"dry-run: no request sent",
	} {
		if !strings.Contains(stdout, marker) {
			t.Fatalf("submit batch dry-run output missing %q: %q", marker, stdout)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("submit batch dry-run sent %d HTTP requests, want 0", requests.Load())
	}
}

// TestCLISubmitBatchDuplicateNameDiagnosticIsActionableAndAtomic proves the
// duplicate-name rule at the public process boundary for live human output,
// live structured mode, and topology-independent dry-run validation. The
// invalid live requests are rejected before the Factory Session creates any
// Work, while dry-run never contacts its unreachable server.
func TestCLISubmitBatchDuplicateNameDiagnosticIsActionableAndAtomic(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, duplicateSubmitBatchFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
	})
	defer server.Stop(t)

	process := buildBatchContractProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	messageMarkers := []string{
		"duplicate name \"release\"",
		"works[1].name",
		"works[0].name",
		"unique across the entire batch",
		"different workTypeName values",
		"rename or remove one entry",
	}
	for _, test := range []struct {
		name      string
		json      bool
		requestID string
	}{
		{name: "human", requestID: "batch-duplicate-human"},
		{name: "structured", json: true, requestID: "batch-duplicate-structured"},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"you", "--server", server.URL(), "submit", "batch"}
			if test.json {
				args = []string{"you", "--server", server.URL(), "--json", "submit", "batch"}
			}
			args = append(args, duplicateBatchJSON(test.requestID))
			stdout, stderr, err := executeSubmitBatchCLIExpectError(t, process, args)
			if err == nil {
				t.Fatal("duplicate-name submission succeeded")
			}
			diagnostic := err.Error() + "\n" + stderr
			for _, marker := range messageMarkers {
				if !strings.Contains(diagnostic, marker) {
					t.Fatalf("diagnostic missing %q:\n%s", marker, diagnostic)
				}
			}
			if stdout != "" {
				t.Fatalf("duplicate-name submission emitted success stdout: %q", stdout)
			}

			listed := support.ListDefaultSessionWork(t, server.URL())
			if len(listed.Results) != 0 {
				t.Fatalf("duplicate-name submission admitted partial Work: %#v", listed.Results)
			}
		})
	}

	stdout, stderr, err := executeSubmitBatchCLIExpectError(t, process, []string{
		"you", "--server", "http://127.0.0.1:1", "--json",
		"submit", "batch", "--dry-run", duplicateBatchJSON("batch-duplicate-dry-run"),
	})
	if err == nil {
		t.Fatal("duplicate-name dry-run succeeded")
	}
	diagnostic := err.Error() + "\n" + stderr
	for _, marker := range messageMarkers {
		if !strings.Contains(diagnostic, marker) {
			t.Fatalf("dry-run diagnostic missing %q:\n%s", marker, diagnostic)
		}
	}
	if stdout != "" {
		t.Fatalf("duplicate-name dry-run emitted stdout: %q", stdout)
	}
}

// TestCLISubmitBatchOversizedPayloadDiagnosticAcrossInputModes proves every
// public batch input path preserves the Work-owned size diagnostic, including
// the local dry-run path. A rejected batch emits no success acknowledgement and
// does not admit partial Work into the Factory Session.
func TestCLISubmitBatchOversizedPayloadDiagnosticAcrossInputModes(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, batchAdmissionFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
	})
	defer server.Stop(t)

	process := buildBatchContractProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)

	for _, test := range []struct {
		name   string
		input  string
		json   bool
		dryRun bool
	}{
		{name: "file-human", input: "file"},
		{name: "stdin-json", input: "stdin", json: true},
		{name: "inline-human", input: "inline"},
		{name: "dry-run-json", input: "file", json: true, dryRun: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestID := "batch-payload-limit-" + test.name
			workName := "oversized-" + test.name
			batch := oversizedBatchJSON(requestID, workName)
			args := []string{"you"}
			if test.dryRun {
				args = append(args, "--server", "http://127.0.0.1:1")
			} else {
				args = append(args, "--server", server.URL())
			}
			if test.json {
				args = append(args, "--json")
			}
			args = append(args, "submit", "batch")

			stdin := ""
			stdinIsTTY := true
			switch test.input {
			case "file":
				path := writeBatchContractInputFile(t, batch)
				args = append(args, path)
			case "stdin":
				stdin = batch
				stdinIsTTY = false
			case "inline":
				args = append(args, batch)
			default:
				t.Fatalf("unsupported test input %q", test.input)
			}
			if test.dryRun {
				// Keep dry-run distinct from the live file case while still using
				// the real file acquisition path.
				if len(args) == 0 || !strings.HasSuffix(args[len(args)-1], ".json") {
					t.Fatalf("dry-run args did not receive a batch file: %#v", args)
				}
				args = append(args[:len(args)-1], "--dry-run", args[len(args)-1])
			}

			stdout, stderr, err := executeSubmitBatchCLIExpectErrorWithInput(
				t, process, args, stdin, stdinIsTTY,
			)
			if err == nil {
				t.Fatal("oversized batch submission succeeded")
			}
			diagnostic := err.Error() + "\n" + stderr
			for _, marker := range []string{
				`Work "` + workName + `"`,
				"payloadBytes=65537",
				"payloadLimitBytes=65536",
			} {
				if !strings.Contains(diagnostic, marker) {
					t.Fatalf("diagnostic missing %q:\n%s", marker, diagnostic)
				}
			}
			if stdout != "" {
				t.Fatalf("oversized submission emitted success stdout: %q", stdout)
			}
			for _, marker := range []string{"requestId:", "traceId:", "work count:", "Submitted:"} {
				if strings.Contains(stdout+stderr, marker) {
					t.Fatalf("oversized submission emitted success marker %q:\nstdout:\n%s\nstderr:\n%s", marker, stdout, stderr)
				}
			}
		})
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	if len(listed.Results) != 0 {
		t.Fatalf("oversized batch submissions admitted Work: %#v", listed.Results)
	}
}

// TestCLISubmitBatchAtAndBelowLimitDispatchThroughProviderCommandRunner
// proves the inclusive boundary still reaches the injected provider edge.
// The Codex adapter receives the rendered prompt on stdin, leaving the
// composed host command line comfortably below its Windows loader budget.
func TestCLISubmitBatchAtAndBelowLimitDispatchThroughProviderCommandRunner(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, providerSubmitBatchFactoryConfig())
	support.WriteAgentConfig(t, factoryDir, "provider-worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderCodex,
		"gpt-5-codex",
	))
	support.WriteWorkstationConfig(t, factoryDir, "process-task", "---\ntype: MODEL_WORKSTATION\n---\n{{ (index .Inputs 0).Payload }}\n")
	runner := support.NewRecordingCommandRunner("boundary dispatch COMPLETE")
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	defer server.Stop(t)

	process := buildBatchContractProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)

	for index, test := range []struct {
		name        string
		payloadSize int
	}{
		{name: "below-limit", payloadSize: work.MaxWorkPayloadBytes - 1},
		{name: "at-limit", payloadSize: work.MaxWorkPayloadBytes},
	} {
		requestID := "batch-payload-boundary-" + test.name
		workName := "boundary-" + test.name
		marker := "boundary-prompt-marker-" + test.name
		path := writeBatchContractInputFile(t, boundaryBatchJSON(
			requestID, workName, test.payloadSize, marker,
		))
		stdout := executeSubmitBatchCLI(t, process, []string{
			"you", "--server", server.URL(), "submit", "batch", path,
		})
		if !strings.Contains(stdout, "work count: 1") {
			t.Fatalf("%s submission output missing work count:\n%s", test.name, stdout)
		}

		wantCalls := index + 1
		// The provider dispatch is scheduled after the HTTP response and the
		// injected command edge exposes a deterministic completion signal. Wait
		// on that edge instead of polling a public projection for call count.
		waitCtx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		err := runner.WaitForCall(waitCtx, wantCalls)
		cancel()
		if err != nil {
			t.Fatalf("waiting for %s provider dispatch: %v", test.name, err)
		}
		request := runner.LastRequest()
		if !strings.Contains(string(request.Stdin), marker) {
			t.Fatalf("%s provider stdin omitted rendered payload marker %q", test.name, marker)
		}
		if len(request.Stdin) < test.payloadSize {
			t.Fatalf("%s provider stdin = %d bytes, want the complete rendered payload", test.name, len(request.Stdin))
		}
		if got := platformprocess.ComposedCommandLineLength(request.Command, request.Args); got >= platformprocess.WindowsCommandLineLimit {
			t.Fatalf("%s composed command line = %d, want below Windows limit %d", test.name, got, platformprocess.WindowsCommandLineLimit)
		}
	}
}

// TestCLISubmitBatchRelationEndpointDiagnosticIsActionableAndAtomic proves
// relation endpoints resolve only through the submitted works[] names. A
// valid batch preserves both supported relation types; missing source/target
// endpoints are rejected during live admission, while dry-run accepts the
// shape because board lookup is only available to the live session.
func TestCLISubmitBatchRelationEndpointDiagnosticIsActionableAndAtomic(t *testing.T) {
	factoryDir := support.ScaffoldFactory(t, successSubmitBatchFactoryConfig())
	runner := testutil.NewProviderCommandRunner()
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Edges:      serviceedges.Edges{ProviderCommandRunner: runner},
	})
	defer server.Stop(t)

	process := buildBatchContractProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)

	validStdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", server.URL(),
		"submit", "batch", validRelationsBatchJSON(),
	})
	if !strings.Contains(validStdout, "work count: 3") {
		t.Fatalf("valid relation batch output missing work count:\n%s", validStdout)
	}

	runRelationEndpointCases(t, process, server.URL())

	listed := support.ListDefaultSessionWork(t, server.URL())
	if len(listed.Results) != 3 {
		t.Fatalf("invalid relation batches admitted partial Work: %#v", listed.Results)
	}
}

func runRelationEndpointCases(t *testing.T, process support.Process, serverURL string) {
	t.Helper()
	for _, test := range []struct {
		name        string
		json        bool
		dryRun      bool
		batch       string
		value       string
		source      string
		wantError   bool
		wantMarkers []string
	}{
		{
			name:      "live missing source human",
			batch:     relationEndpointBatchJSON("batch-relation-missing-source-human", "target", "missing-source", "target"),
			value:     "missing-source",
			source:    "missing-source",
			wantError: true,
			wantMarkers: []string{
				`endpoint sourceWorkName="missing-source"`,
				"missing from this batch",
			},
		},
		{
			name:      "live missing target structured",
			json:      true,
			batch:     relationEndpointBatchJSON("batch-relation-missing-target-structured", "source", "source", "missing-target"),
			value:     "missing-target",
			source:    "source",
			wantError: true,
			wantMarkers: []string{
				`unknown targetWorkName="missing-target"`,
				"does not identify a Work on this Factory Session board",
				"correct targetWorkName or provide targetWorkId",
			},
		},
		{
			name:   "dry-run previously submitted target",
			json:   true,
			dryRun: true,
			batch:  relationEndpointBatchJSON("batch-relation-previously-submitted-target", "new-work", "new-work", "parent"),
			value:  "parent",
			source: "new-work",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"you", "--server", serverURL, "submit", "batch"}
			if test.json {
				args = []string{"you", "--server", serverURL, "--json", "submit", "batch"}
			}
			if test.dryRun {
				args = []string{"you", "--server", "http://127.0.0.1:1", "--json", "submit", "batch", "--dry-run"}
			}
			args = append(args, test.batch)
			stdout, stderr, err := executeSubmitBatchCLIExpectError(t, process, args)
			if !test.wantError {
				if err != nil {
					t.Fatalf("dry-run relation shape validation error = %v\nstderr:\n%s", err, stderr)
				}
				if !strings.Contains(stdout, `"dryRun":true`) {
					t.Fatalf("dry-run relation shape output missing dryRun marker: %q", stdout)
				}
				return
			}
			if err == nil {
				t.Fatal("relation endpoint submission succeeded")
			}
			diagnostic := err.Error() + "\n" + stderr
			assertRelationEndpointDiagnostic(t, diagnostic, test.value, test.source)
			for _, marker := range test.wantMarkers {
				if !strings.Contains(diagnostic, marker) {
					t.Fatalf("diagnostic missing %q:\n%s", marker, diagnostic)
				}
			}
			if stdout != "" {
				t.Fatalf("relation endpoint submission emitted success stdout: %q", stdout)
			}
		})
	}
}

// TestCLISubmitBatchSuccessHumanAndJSONShapes proves successful you submit batch
// emits stable human text and --json shapes with request identity, trace context,
// work count, and accepted work entries when exercised through Process.Execute
// against a live Factory Session Work upsert path.
func TestCLISubmitBatchSuccessHumanAndJSONShapes(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI submit batch contract")
	}

	factoryDir := support.ScaffoldFactory(t, successSubmitBatchFactoryConfig())
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:     factoryDir,
		UseMockWorkers: true,
	})
	defer server.Stop(t)

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{})
	process := buildBatchContractProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})

	humanStdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", server.URL(),
		"submit", "batch", successHumanInlineBatchJSON(),
	})
	assertSubmitBatchHumanSuccess(
		t,
		humanStdout,
		successHumanSubmitBatchRequestID,
		successSubmitBatchWorkName,
		successSubmitBatchWorkType,
	)

	jsonStdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", server.URL(), "--json",
		"submit", "batch", successJSONInlineBatchJSON(),
	})
	submitted := decodeSubmitBatchJSONResult(t, jsonStdout)
	assertSubmitBatchJSONSuccess(
		t,
		submitted,
		successJSONSubmitBatchRequestID,
		successSubmitBatchWorkName,
		successSubmitBatchWorkType,
	)

	functionalevidence.Covers(t, "cli/you.submit.batch")
}

// TestCLISubmitBatchInvalidJSONFailsBeforeUpsert proves malformed batch JSON
// fails from public you submit batch before any Factory Session Work upsert so
// bad payloads never partially mutate Work state.
func TestCLISubmitBatchInvalidJSONFailsBeforeUpsert(t *testing.T) {
	serverURL, requests := newInstrumentedSubmitBatchServer(t)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{})
	process := buildBatchContractProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})

	stdout, stderr, err := executeSubmitBatchCLIExpectError(t, process, []string{
		"you", "--server", serverURL,
		"submit", "batch", invalidInlineBatchJSON(),
	})
	if err == nil {
		t.Fatalf(
			"Process.Execute(invalid batch JSON) succeeded; stdout:\n%s\nstderr:\n%s",
			stdout,
			stderr,
		)
	}

	diagnostic := err.Error() + "\n" + stderr
	for _, marker := range []string{
		"parse inline JSON",
		"invalid character",
	} {
		if !strings.Contains(diagnostic, marker) {
			t.Fatalf("invalid batch JSON diagnostic missing %q:\n%s", marker, diagnostic)
		}
	}

	combined := stdout + stderr
	for _, marker := range []string{
		"requestId:",
		"traceId:",
		"work count:",
	} {
		if strings.Contains(combined, marker) {
			t.Fatalf(
				"invalid batch JSON output must not contain success marker %q:\nstdout:\n%s\nstderr:\n%s",
				marker,
				stdout,
				stderr,
			)
		}
	}

	if requests.Load() != 0 {
		t.Fatalf("submit batch invalid JSON sent %d HTTP requests, want 0", requests.Load())
	}
}

// TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess proves the
// Work-owned batch_contract cell constructs a customer process through
// root.BuildProcess, invokes public you submit batch through Process.Execute,
// and replaces external effects only through edges.Edges.
func TestCLISubmitBatchContractHarnessExecutesThroughRootBuildProcess(t *testing.T) {
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{})
	process := buildBatchContractProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})

	stdout := executeSubmitBatchCLI(t, process, []string{
		"you", "--server", "http://127.0.0.1:1",
		"submit", "batch", "--dry-run", harnessInlineBatchJSON(harnessSubmitBatchRequestID),
	})

	for _, marker := range []string{
		"requestId: " + harnessSubmitBatchRequestID,
		"batchSource: inline",
		"dry-run: no request sent",
	} {
		if !strings.Contains(stdout, marker) {
			t.Fatalf("submit batch dry-run output missing %q: %q", marker, stdout)
		}
	}
}
