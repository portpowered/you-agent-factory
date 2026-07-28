package review

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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

func runPackagedReviewCLIJSONInvocationWithFactorySetup(
	t *testing.T,
	runner platformprocess.CommandRunner,
	configure func(*testing.T, string),
	requestText string,
	extraArgs ...string,
) factoryapi.InvocationResponse {
	t.Helper()

	homeDir := t.TempDir()
	factoryDir := support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedReviewFactoryName)
	if configure != nil {
		configure(t, factoryDir)
	}

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

func setPackagedReviewWorkerModel(t *testing.T, factoryDir, model string) {
	t.Helper()

	path := filepath.Join(factoryDir, "factory.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode materialized factory: %v", err)
	}
	for _, worker := range factory["workers"].([]any) {
		definition := worker.(map[string]any)
		definition["modelProvider"] = "CODEX"
		definition["model"] = model
	}
	updated, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("encode materialized factory: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write materialized factory: %v", err)
	}
}

func assertPackagedReviewProviderInvocations(
	t *testing.T,
	requests []platformprocess.CommandRequest,
	wantProvider string,
	wantModel string,
) {
	t.Helper()

	for index, request := range requests {
		if wantProvider != "" && request.Command != wantProvider {
			t.Fatalf("provider invocation %d command = %q, want %q", index, request.Command, wantProvider)
		}
		if wantModel != "" && !packagedReviewProviderRequestIncludesModel(request, wantModel) {
			t.Fatalf("provider invocation %d args = %#v, want --model %q", index, request.Args, wantModel)
		}
	}
}

func packagedReviewProviderRequestIncludesModel(
	request platformprocess.CommandRequest,
	wantModel string,
) bool {
	for index := 0; index+1 < len(request.Args); index++ {
		if request.Args[index] == "--model" && request.Args[index+1] == wantModel {
			return true
		}
	}
	return false
}

func runPackagedReviewCLIJSONFailureInvocation(
	t *testing.T,
	runner platformprocess.CommandRunner,
	requestText string,
	extraArgs ...string,
) (factoryapi.InvocationResponse, string, error) {
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
	execErr := process.Execute(inputs.Input)

	var response factoryapi.InvocationResponse
	if stdout := strings.TrimSpace(inputs.Stdout()); stdout != "" {
		if decodeErr := json.Unmarshal([]byte(stdout), &response); decodeErr != nil {
			t.Fatalf("decode invocation JSON stdout: %v\nstdout:\n%s", decodeErr, inputs.Stdout())
		}
	}
	return response, inputs.Stderr(), execErr
}

type packagedReviewFailingCommandRunner struct{}

func (packagedReviewFailingCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("mock provider failure")
}

func assertPackagedReviewBoundedFailureFailed(
	t *testing.T,
	response factoryapi.InvocationResponse,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult != nil {
		t.Fatalf(
			"primaryResult = %#v, want no completed success primary result after bounded review failure",
			response.PrimaryResult,
		)
	}
	if response.WorkState == nil || *response.WorkState != "reviewable-work:failed" {
		t.Fatalf("workState = %#v, want reviewable-work:failed", response.WorkState)
	}
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
