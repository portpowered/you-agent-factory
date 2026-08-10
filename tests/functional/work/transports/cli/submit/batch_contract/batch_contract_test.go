package batch_contract_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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

// TestCLISubmitBatchRelationEndpointDiagnosticIsActionableAndAtomic proves
// relation endpoints resolve only through the submitted works[] names. A
// valid batch preserves both supported relation types; missing source/target
// endpoints are rejected in live human, live structured, and dry-run modes,
// including when the target name belongs to an earlier submission.
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

	for _, test := range []struct {
		name       string
		json       bool
		dryRun     bool
		batch      string
		endpoint   string
		value      string
		source     string
		target     string
		additional string
	}{
		{
			name:     "live missing source human",
			batch:    relationEndpointBatchJSON("batch-relation-missing-source-human", "target", "missing-source", "target"),
			endpoint: "sourceWorkName",
			value:    "missing-source",
			source:   "missing-source",
			target:   "target",
		},
		{
			name:     "live missing target structured",
			json:     true,
			batch:    relationEndpointBatchJSON("batch-relation-missing-target-structured", "source", "source", "missing-target"),
			endpoint: "targetWorkName",
			value:    "missing-target",
			source:   "source",
			target:   "missing-target",
		},
		{
			name:       "dry-run previously submitted target",
			json:       true,
			dryRun:     true,
			batch:      relationEndpointBatchJSON("batch-relation-previously-submitted-target", "new-work", "new-work", "parent"),
			endpoint:   "targetWorkName",
			value:      "parent",
			source:     "new-work",
			target:     "parent",
			additional: "not previously submitted Work",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			args := []string{"you", "--server", server.URL(), "submit", "batch"}
			if test.json {
				args = []string{"you", "--server", server.URL(), "--json", "submit", "batch"}
			}
			if test.dryRun {
				args = []string{"you", "--server", "http://127.0.0.1:1", "--json", "submit", "batch", "--dry-run"}
			}
			args = append(args, test.batch)
			stdout, stderr, err := executeSubmitBatchCLIExpectError(t, process, args)
			if err == nil {
				t.Fatal("relation endpoint submission succeeded")
			}
			diagnostic := err.Error() + "\n" + stderr
			assertRelationEndpointDiagnostic(t, diagnostic, test.endpoint, test.value, test.source, test.target)
			if test.additional != "" && !strings.Contains(diagnostic, test.additional) {
				t.Fatalf("diagnostic missing %q:\n%s", test.additional, diagnostic)
			}
			if stdout != "" {
				t.Fatalf("relation endpoint submission emitted success stdout: %q", stdout)
			}
		})
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	if len(listed.Results) != 3 {
		t.Fatalf("invalid relation batches admitted partial Work: %#v", listed.Results)
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
