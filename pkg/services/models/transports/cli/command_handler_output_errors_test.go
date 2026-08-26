package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestWriteGenericCLIOutputPathRejectsInvalidResults(t *testing.T) {
	t.Parallel()

	validConfig := InvokeConfig{Context: context.Background(), OutputPath: "answer.txt", Output: io.Discard}
	validOutputSystem := &outputPathTestFileSystem{}
	cases := []struct {
		name    string
		service *rootService
		result  modelinference.InvokeModelResult
		want    string
	}{
		{name: "missing filesystem", service: &rootService{}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "answer"}}}, want: "filesystem is required"},
		{name: "multiple outputs", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "usage", Content: "two"}}}, want: "multiple model outputs"},
		{name: "unnamed output", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Content: "answer"}}}, want: "unnamed output"},
		{name: "empty output", service: &rootService{outputFileSystem: validOutputSystem}, result: modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}, want: "has no inline bytes"},
	}
	for _, test := range cases {
		test := test
		t.Run(test.name, func(t *testing.T) {
			err := test.service.writeGenericCLIOutputPath(validConfig, test.result)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("writeGenericCLIOutputPath error = %v, want %q", err, test.want)
			}
		})
	}
}
