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

func configurePackagedReviewRejectionClassification(t *testing.T, factoryDir string) {
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
	workstations, ok := factory["workstations"].([]any)
	if !ok {
		t.Fatal("materialized factory workstations are not an array")
	}
	for _, raw := range workstations {
		workstation, ok := raw.(map[string]any)
		if !ok || workstation["name"] != "review-review-work" {
			continue
		}
		workstation["stopWords"] = []any{"COMPLETE"}
		workstation["limits"] = map[string]any{"maxRetries": float64(3)}
		updated, err := json.Marshal(factory)
		if err != nil {
			t.Fatalf("encode materialized factory: %v", err)
		}
		if err := os.WriteFile(path, updated, 0o644); err != nil {
			t.Fatalf("write materialized factory: %v", err)
		}
		return
	}
	t.Fatal("materialized factory does not contain review-review-work workstation")
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
		response = support.DecodeInvocationResponseJSON(t, stdout)
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
