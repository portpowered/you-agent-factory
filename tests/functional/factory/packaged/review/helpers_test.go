package review

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func runPackagedReviewCLIJSONInvocation(
	t *testing.T,
	runner platformprocess.CommandRunner,
	requestText string,
	extraArgs ...string,
) factoryapi.InvocationResponse {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedReviewFactoryName)

	args := []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedReviewFactoryName,
		"--no-record",
	}
	args = append(args, extraArgs...)
	args = append(args, requestText)
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()

	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}

	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
		t.Fatalf("decode invocation JSON stdout: %v\nstdout:\n%s", decodeErr, inputs.Stdout())
	}
	return response
}

func assertPackagedReviewCompletedWithText(
	t *testing.T,
	response factoryapi.InvocationResponse,
	want string,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := invocationPrimaryResultText(t, response); got != want {
		t.Fatalf("primaryResult text = %q, want %q", got, want)
	}
}

func providerCommandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	return strings.Join(request.Args, " ")
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}
