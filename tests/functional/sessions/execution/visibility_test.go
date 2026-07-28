package execution_test

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads proves a Factory Session
// invocation started through the public CLI run boundary leaves compatible session
// identity readable through the public API session surface and inspectable run
// correlation plus terminal outcome facts through the public API work read surface
// or the CLI-compatible invocation outcome fields on the same hosted Factory host.
func TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	providerRunner := support.NewStaticSuccessCommandRunner(terminalSuccessPrimaryResult)

	api := support.NewProcessAPIServer()
	edges := serviceedges.Edges{APIServerStarter: api.Start}
	support.ConfigureWorkerCommands(t, &edges, providerRunner, nil)
	process := support.BuildProcess(t, edges)

	factoryPath := filepath.Join(dir, interfaces.FactoryConfigFile)
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--json",
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--with-server",
		"invoke this",
	})
	inputs.WorkingDirectory = dir

	command := support.StartProcessCommand(t, process, inputs.Input)
	baseURL := api.WaitForURL(t)

	sessionDuring := support.GetDefaultSession(t, baseURL)
	if strings.TrimSpace(sessionDuring.Id) == "" {
		t.Fatal("session id observed during CLI invocation is empty, want observable session identity")
	}
	if sessionDuring.Runtime.Status == "" {
		t.Fatalf("session runtime status during CLI invocation = %#v, want observable lifecycle status", sessionDuring.Runtime)
	}
	workDuring := support.ListDefaultSessionWork(t, baseURL)

	<-command.Done()
	if err := command.Err(); err != nil {
		t.Fatalf(
			"Process.Execute(CLI invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	assertInvocationPrimaryResultText(t, cliResponse, terminalSuccessPrimaryResult)

	if strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf("CLI invocation traceId = %q, want non-empty run correlation", cliResponse.TraceId)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" {
		t.Fatalf("CLI invocation requestId = %q, want non-empty run correlation", cliResponse.RequestId)
	}

	if len(workDuring.Results) == 1 {
		item := workDuring.Results[0]
		if item.State == nil || generatedWorkStateType(item.State) != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("work state = %#v, want TERMINAL", item.State)
		}
		if item.Content == nil || len(*item.Content) != 1 {
			t.Fatalf("work content = %#v, want one text part", item.Content)
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("work content[0] as text part: %v", err)
		}
		if part.Text != terminalSuccessPrimaryResult {
			t.Fatalf("work content text = %q, want %q", part.Text, terminalSuccessPrimaryResult)
		}
	}
}

// TestAPIInvocationResultMatchesCLICompatibleFacts proves a Factory Session
// invocation started through the public API returns terminal outcome facts that
// agree with the CLI-compatible InvocationResponse fields customers compare
// across surfaces for the same minimal invocation fixture.
func TestAPIInvocationResultMatchesCLICompatibleFacts(t *testing.T) {
	dir := scaffoldInvocationFactory(t, nil)
	providerRunner := support.NewStaticSuccessCommandRunner(terminalSuccessPrimaryResult)

	server := startInvocationServer(t, dir, providerRunner, nil)
	apiResponse := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke this", nil))

	cliResponse := runHostedInvocationCLIJSON(t, dir, providerRunner)
	assertAPIInvocationMatchesCLICompatibleFacts(t, apiResponse, cliResponse, terminalSuccessPrimaryResult)
}

func runHostedInvocationCLIJSON(
	t *testing.T,
	factoryDir string,
	providerRunner platformprocess.CommandRunner,
) factoryapi.InvocationResponse {
	t.Helper()

	api := support.NewProcessAPIServer()
	edges := serviceedges.Edges{APIServerStarter: api.Start}
	support.ConfigureWorkerCommands(t, &edges, providerRunner, nil)
	process := support.BuildProcess(t, edges)

	factoryPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	inputs := support.FakeInputs(t.Context(), []string{
		"you",
		"--json",
		"run",
		"--factory", factoryPath,
		"--no-record",
		"--with-server",
		"invoke this",
	})
	inputs.WorkingDirectory = factoryDir

	command := support.StartProcessCommand(t, process, inputs.Input)
	_ = api.WaitForURL(t)

	<-command.Done()
	if err := command.Err(); err != nil {
		t.Fatalf(
			"Process.Execute(CLI invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	var cliResponse factoryapi.InvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &cliResponse); err != nil {
		t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, inputs.Stdout())
	}
	return cliResponse
}
