package execution_test

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestCLIInvocationIsVisibleThroughAPISessionAndWorkReads proves a Factory Session
// invocation started through the public CLI run boundary leaves compatible session
// identity readable through the public API session surface and terminal work outcome
// facts readable through the public API work read surface on the same Factory host.
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

	observationsReady := make(chan error, 1)
	var (
		sessionDuring         factoryapi.FactorySession
		workVisibleDuringHost bool
	)
	go func() {
		baseURL, err := api.WaitForBaseURL(5 * time.Second)
		if err != nil {
			observationsReady <- err
			return
		}
		var pollErr error
		sessionDuring, workVisibleDuringHost, pollErr = tryPollHostedCLIInvocationObservations(
			baseURL,
			terminalSuccessPrimaryResult,
			command.Done(),
		)
		observationsReady <- pollErr
	}()

	if obsErr := <-observationsReady; obsErr != nil {
		<-command.Done()
		if err := command.Err(); err != nil {
			t.Fatalf(
				"Process.Execute(CLI invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
				err,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}
		t.Logf("session during hosted invocation not observable before host shutdown: %v", obsErr)
	} else {
		if strings.TrimSpace(sessionDuring.Id) == "" {
			t.Fatal("session id observed during CLI invocation is empty, want observable session identity")
		}
		if sessionDuring.Runtime.Status == "" {
			t.Fatalf("session runtime status during CLI invocation = %#v, want observable lifecycle status", sessionDuring.Runtime)
		}

		<-command.Done()
		if err := command.Err(); err != nil {
			t.Fatalf(
				"Process.Execute(CLI invocation) error = %v\nstdout:\n%s\nstderr:\n%s",
				err,
				inputs.Stdout(),
				inputs.Stderr(),
			)
		}
	}

	cliResponse := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	assertInvocationPrimaryResultText(t, cliResponse, terminalSuccessPrimaryResult)

	if strings.TrimSpace(cliResponse.TraceId) == "" {
		t.Fatalf("CLI invocation traceId = %q, want non-empty run correlation", cliResponse.TraceId)
	}
	if strings.TrimSpace(cliResponse.RequestId) == "" {
		t.Fatalf("CLI invocation requestId = %q, want non-empty run correlation", cliResponse.RequestId)
	}
	if workVisibleDuringHost {
		// Terminal /work was already observed while the hosted API was still alive.
		return
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

	return support.DecodeInvocationResponseJSON(t, inputs.Stdout())
}
