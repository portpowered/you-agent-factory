package lineagegraph

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
)

type visualizationFileSystem map[string]visualizationFile

type visualizationFile struct {
	data []byte
	err  error
}

func (files visualizationFileSystem) ReadFile(path string) ([]byte, error) {
	file, ok := files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return file.data, file.err
}

const visualizationBatch = `{
  "requestId":"visualize-test","type":"FACTORY_REQUEST_BATCH",
  "works":[{"name":"alpha","workTypeName":"task"},{"name":"beta","workTypeName":"task"},{"name":"solo","workTypeName":"task"}],
  "relations":[{"type":"DEPENDS_ON","sourceWorkName":"beta","targetWorkName":"alpha"}]
}`

func TestVisualizationOperation_RendersSupportedFormats(t *testing.T) {
	operation := NewVisualizationOperation(
		visualizationFileSystem{"batch.json": {data: []byte(visualizationBatch)}},
		testVisualizationParser,
	)
	tests := []struct {
		format string
		want   []string
	}{
		{"", []string{"flowchart TD", `alpha["alpha"]`, "beta --> alpha", `solo["solo"]`}},
		{"markdown-mermaid", []string{"# Work Dependency Graph", "```mermaid", "3 work items and 1 declared dependency."}},
	}
	for _, test := range tests {
		output, err := operation(VisualizationRequest{BatchFile: "batch.json", Format: test.format})
		if err != nil {
			t.Fatalf("format %q: %v", test.format, err)
		}
		for _, want := range test.want {
			if !strings.Contains(output, want) {
				t.Fatalf("format %q output missing %q:\n%s", test.format, want, output)
			}
		}
	}
}

func TestVisualizationOperation_PreservesValidationAndReadFailures(t *testing.T) {
	readErr := errors.New("permission denied")
	files := visualizationFileSystem{
		"empty.json":   {},
		"invalid.json": {data: []byte(`{not-json`)},
		"retired.json": {data: []byte(`{"requestId":"retired","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","work_type_id":"task"}]}`)},
		"unknown.json": {data: []byte(`{"requestId":"unknown","type":"FACTORY_REQUEST_BATCH","works":[{"name":"alpha","workTypeName":"task"}],"relations":[{"type":"DEPENDS_ON","sourceWorkName":"alpha","targetWorkName":"missing"}]}`)},
		"denied.json":  {err: readErr},
	}
	operation := NewVisualizationOperation(files, testVisualizationParser)
	tests := []struct {
		request VisualizationRequest
		want    string
	}{
		{VisualizationRequest{}, "batch file path is required"},
		{VisualizationRequest{BatchFile: "batch.json", Format: "svg"}, `unsupported format "svg"`},
		{VisualizationRequest{BatchFile: "missing.json"}, "batch file not found"},
		{VisualizationRequest{BatchFile: "empty.json"}, "batch input is empty"},
		{VisualizationRequest{BatchFile: "invalid.json"}, "invalid JSON"},
		{VisualizationRequest{BatchFile: "retired.json"}, "work_type_id is not supported"},
		{VisualizationRequest{BatchFile: "unknown.json"}, "unknown targetWorkName"},
		{VisualizationRequest{BatchFile: "denied.json"}, "permission denied"},
	}
	for _, test := range tests {
		output, err := operation(test.request)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Fatalf("request %#v error = %v, want %q", test.request, err, test.want)
		}
		if output != "" {
			t.Fatalf("request %#v output = %q, want empty", test.request, output)
		}
	}
}

func TestVisualizationOperation_RequiresFilesystem(t *testing.T) {
	_, err := NewVisualizationOperation(nil, testVisualizationParser)(VisualizationRequest{BatchFile: "batch.json"})
	if err == nil || !strings.Contains(err.Error(), "filesystem is required") {
		t.Fatalf("error = %v", err)
	}
}

func TestVisualizationOperation_RequiresParser(t *testing.T) {
	_, err := NewVisualizationOperation(visualizationFileSystem{"batch.json": {data: []byte(visualizationBatch)}}, nil)(
		VisualizationRequest{BatchFile: "batch.json"},
	)
	if err == nil || !strings.Contains(err.Error(), "batch parser is required") {
		t.Fatalf("error = %v", err)
	}
}
