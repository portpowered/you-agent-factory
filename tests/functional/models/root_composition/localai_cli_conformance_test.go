package root_composition_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"github.com/portpowered/infinite-you/tests/functional/internal/support/conformance"
)

func executeLocalAICLIConformanceRow(
	t *testing.T,
	process support.Process,
	dir string,
	environment []string,
	row conformance.Row,
) (models.GenericInvocationResult, error) {
	t.Helper()
	text := row.Inputs[0].Content
	for _, input := range row.Inputs {
		if input.Modality == models.ModalityText {
			text = input.Content
			break
		}
	}
	args := []string{
		"you", "--json", "models", "invoke", conformanceModelName(row.Operation.Name),
		"--operation", row.Operation.Name,
	}
	for _, input := range row.Inputs {
		value, err := json.Marshal(struct {
			Name        string `json:"name"`
			Modality    string `json:"modality"`
			ContentType string `json:"contentType"`
			MediaType   string `json:"mediaType"`
			Content     string `json:"content"`
		}{
			Name: input.Name, Modality: string(input.Modality),
			ContentType: input.ContentType, MediaType: input.MediaType,
			Content: input.Content,
		})
		if err != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("%s encode CLI input: %w", row.Label, err)
		}
		args = append(args, "--input", string(value))
	}
	if len(row.Inputs) == 0 {
		args = append(args, "--text", text)
	}
	for _, parameter := range conformanceParameters() {
		value, err := json.Marshal(struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		}{Name: parameter.Name, Value: parameter.Value})
		if err != nil {
			return models.GenericInvocationResult{}, fmt.Errorf("%s encode CLI parameter: %w", row.Label, err)
		}
		args = append(args, "--parameter", string(value))
	}

	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err != nil {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"%s CLI invocation: %w (stdout=%q stderr=%q)",
			row.Label, err, inputs.Stdout(), inputs.Stderr(),
		)
	}
	var response factoryapi.GenericModelInvocationResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); err != nil {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"%s decode CLI generic response: %w (stdout=%q stderr=%q)",
			row.Label, err, inputs.Stdout(), inputs.Stderr(),
		)
	}
	if response.Failure != nil {
		return models.GenericInvocationResult{}, fmt.Errorf(
			"%s CLI generic invocation failed: %s", row.Label, response.Failure.Message,
		)
	}
	if err := assertConformanceResponse(row, response); err != nil {
		return models.GenericInvocationResult{}, err
	}
	return models.GenericInvocationResult{Status: models.ModelInvocationStatusCompleted}, nil
}
