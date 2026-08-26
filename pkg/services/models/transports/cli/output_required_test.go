package cli

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	modelinference "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"go.uber.org/zap"
)

func TestModelCommands_RequireCallerOwnedOutput(t *testing.T) {
	t.Parallel()

	tests := map[string]func() error{
		"list": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).List(ListConfig{Context: context.Background()})
		},
		"inspect": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).Inspect(InspectConfig{Context: context.Background()})
		},
		"invoke": func() error { return invokeForTest(t, InvokeConfig{Context: context.Background()}) },
		"pull": func() error {
			return New(testHTTPProtocol(t), testModelInvocationBuilder).Pull(PullConfig{Context: context.Background()})
		},
	}
	for name, run := range tests {
		name, run := name, run
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := run(); err == nil || err.Error() != "output writer is required" {
				t.Fatalf("error = %v, want output writer is required", err)
			}
		})
	}
}

func TestParseGenericCLIInputSpecsPreservesOrderedPublicFields(t *testing.T) {
	t.Parallel()

	inputs, err := parseGenericCLIInputSpecs([]string{
		`{"name":"first","modality":" text ","contentType":" text/plain ","mediaType":" image/png ","content":"one"}`,
		`{"name":"second","modality":"IMAGE","contentType":"image/png","mediaType":"image/png","content":"two"}`,
	})
	if err != nil {
		t.Fatalf("parseGenericCLIInputSpecs() error = %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("parsed inputs = %#v, want two ordered inputs", inputs)
	}
	if inputs[0].Name != "first" || inputs[0].Modality != modelinference.ModalityText ||
		inputs[0].ContentType != "text/plain" || inputs[0].MediaType != "image/png" || inputs[0].Content != "one" {
		t.Fatalf("first parsed input = %#v", inputs[0])
	}
	if inputs[1].Name != "second" || inputs[1].Content != "two" {
		t.Fatalf("second parsed input = %#v, want ordered second input", inputs[1])
	}

	if got, err := parseGenericCLIInputSpecs(nil); err != nil || got != nil {
		t.Fatalf("empty input specs = %#v, %v; want nil, nil", got, err)
	}
}

func TestParseGenericCLIInputSpecsReportsActionableValidationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		json string
		want string
	}{
		{name: "malformed json", json: `{`, want: "parse --input 1:"},
		{name: "missing name", json: `{"modality":"TEXT","content":"value"}`, want: "parse --input 1: name is required"},
		{name: "missing modality", json: `{"name":"prompt","content":"value"}`, want: "modality is required"},
		{name: "missing content", json: `{"name":"prompt","modality":"TEXT"}`, want: "content is required"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseGenericCLIInputSpecs([]string{testCase.json})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("parseGenericCLIInputSpecs(%q) error = %v, want substring %q", testCase.json, err, testCase.want)
			}
		})
	}
}

func TestParseGenericCLIParameterSpecsPreservesJSONValuesAndOrder(t *testing.T) {
	t.Parallel()

	parameters, err := parseGenericCLIParameterSpecs([]string{
		`{"name":"temperature","value":0.2}`,
		`{"name":"labels","value":["first","second"]}`,
	})
	if err != nil {
		t.Fatalf("parseGenericCLIParameterSpecs() error = %v", err)
	}
	if len(parameters) != 2 || parameters[0].Name != "temperature" || parameters[1].Name != "labels" {
		t.Fatalf("parsed parameters = %#v, want ordered parameters", parameters)
	}
	if got, ok := parameters[0].Value.(float64); !ok || got != 0.2 {
		t.Fatalf("temperature value = %#v, want JSON number 0.2", parameters[0].Value)
	}
	labels, ok := parameters[1].Value.([]any)
	if !ok || len(labels) != 2 || labels[0] != "first" || labels[1] != "second" {
		t.Fatalf("labels value = %#v, want ordered JSON array", parameters[1].Value)
	}

	if got, err := parseGenericCLIParameterSpecs(nil); err != nil || got != nil {
		t.Fatalf("empty parameter specs = %#v, %v; want nil, nil", got, err)
	}
}

func TestParseGenericCLIParameterSpecsReportsActionableValidationErrors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		json string
		want string
	}{
		{name: "malformed json", json: `{`, want: "parse --parameter 1:"},
		{name: "missing name", json: `{"value":1}`, want: "parse --parameter 1: name is required"},
		{name: "missing value", json: `{"name":"temperature"}`, want: "value is required"},
		{name: "malformed value", json: `{"name":"temperature","value":}`, want: "parse --parameter 1:"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseGenericCLIParameterSpecs([]string{testCase.json})
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("parseGenericCLIParameterSpecs(%q) error = %v, want substring %q", testCase.json, err, testCase.want)
			}
		})
	}
}

func TestParseGenericCLIOutputMappingsValidatesIdentityAndPaths(t *testing.T) {
	t.Parallel()

	mappings, err := parseGenericCLIOutputMappings([]string{"text=first.txt", "audio=second.wav"})
	if err != nil {
		t.Fatalf("parseGenericCLIOutputMappings() error = %v", err)
	}
	if len(mappings) != 2 || mappings[0].slot != "text" || mappings[0].path != "first.txt" || mappings[1].slot != "audio" {
		t.Fatalf("parsed output mappings = %#v, want ordered mappings", mappings)
	}

	cases := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "missing separator", values: []string{"text"}, want: "expected slot=path"},
		{name: "empty slot", values: []string{"=text.txt"}, want: "slot and path are required"},
		{name: "empty path", values: []string{"text="}, want: "slot and path are required"},
		{name: "stdout path", values: []string{"text=-"}, want: "path '-' is not supported"},
		{name: "duplicate slot", values: []string{"text=first.txt", "text=second.txt"}, want: "duplicate output mapping"},
		{name: "duplicate path", values: []string{"text=shared.txt", "audio=shared.txt"}, want: "same path"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseGenericCLIOutputMappings(testCase.values)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("parseGenericCLIOutputMappings(%#v) error = %v, want substring %q", testCase.values, err, testCase.want)
			}
		})
	}
}

func TestGenericCLIInputNormalizeMediaTypeUsesStableAliases(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		input, want string
	}{
		{input: " audio/x-wav; charset=binary ", want: "audio/wav"},
		{input: "application/ogg; codecs=opus", want: "audio/ogg"},
		{input: " Text/Plain; charset=utf-8 ", want: "text/plain"},
	} {
		if got := genericCLIInputNormalizeMediaType(testCase.input); got != testCase.want {
			t.Fatalf("genericCLIInputNormalizeMediaType(%q) = %q, want %q", testCase.input, got, testCase.want)
		}
	}
}

func TestValidateGenericCLIOutputMappingsChecksDeclaredOutputContract(t *testing.T) {
	t.Parallel()

	operation := modelinference.Operation{Outputs: []modelinference.OperationSlot{
		{Name: "text"}, {Name: "audio"},
	}}
	if err := validateGenericCLIOutputMappings([]string{"text=text.txt", "audio=audio.wav"}, operation, true); err != nil {
		t.Fatalf("valid output mappings error = %v", err)
	}
	for _, testCase := range []struct {
		name   string
		values []string
		found  bool
		want   string
	}{
		{name: "unknown operation", values: []string{"text=text.txt", "audio=audio.wav"}, want: "unknown operation"},
		{name: "missing output", values: []string{"text=text.txt"}, found: true, want: "must cover every output slot"},
		{name: "unknown slot", values: []string{"text=text.txt", "other=other.txt"}, found: true, want: "unknown slot"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			err := validateGenericCLIOutputMappings(testCase.values, operation, testCase.found)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateGenericCLIOutputMappings(%#v) error = %v, want substring %q", testCase.values, err, testCase.want)
			}
		})
	}
}

type genericOutputTemporary struct {
	name       string
	data       []byte
	writeErr   error
	closeErr   error
	shortWrite bool
}

func (temporary *genericOutputTemporary) Write(data []byte) (int, error) {
	if temporary.writeErr != nil {
		return 0, temporary.writeErr
	}
	if temporary.shortWrite {
		return len(data) - 1, nil
	}
	temporary.data = append(temporary.data, data...)
	return len(data), nil
}

func (temporary *genericOutputTemporary) Close() error { return temporary.closeErr }
func (temporary *genericOutputTemporary) Name() string { return temporary.name }

type genericOutputFileSystem struct {
	createErr     error
	createNil     bool
	closeErr      error
	writeErr      error
	shortWrite    bool
	temporaryName string
	emptyName     bool
	inspectErr    error
	inspectExists bool
	removeErr     error
	renameErr     error
	created       []*genericOutputTemporary
	removed       []string
	renamed       [][2]string
}

func (fileSystem *genericOutputFileSystem) CreateTemp(_ string, pattern string) (OutputTemporaryFile, error) {
	if fileSystem.createErr != nil {
		return nil, fileSystem.createErr
	}
	if fileSystem.createNil {
		return nil, nil
	}
	name := fileSystem.temporaryName
	if name == "" && !fileSystem.emptyName {
		name = pattern + "temporary"
	}
	temporary := &genericOutputTemporary{
		name: name, writeErr: fileSystem.writeErr, closeErr: fileSystem.closeErr,
		shortWrite: fileSystem.shortWrite,
	}
	fileSystem.created = append(fileSystem.created, temporary)
	return temporary, nil
}

func (fileSystem *genericOutputFileSystem) Inspect(string) (os.FileInfo, error) {
	if fileSystem.inspectErr != nil {
		return nil, fileSystem.inspectErr
	}
	if fileSystem.inspectExists {
		return genericOutputFileInfo{}, nil
	}
	return nil, os.ErrNotExist
}

func (fileSystem *genericOutputFileSystem) Remove(path string) error {
	fileSystem.removed = append(fileSystem.removed, path)
	return fileSystem.removeErr
}

func (fileSystem *genericOutputFileSystem) Rename(oldPath, newPath string) error {
	fileSystem.renamed = append(fileSystem.renamed, [2]string{oldPath, newPath})
	return fileSystem.renameErr
}

type genericOutputFileInfo struct{}

func (genericOutputFileInfo) Name() string       { return "output" }
func (genericOutputFileInfo) Size() int64        { return 0 }
func (genericOutputFileInfo) Mode() os.FileMode  { return 0o600 }
func (genericOutputFileInfo) ModTime() time.Time { return time.Time{} }
func (genericOutputFileInfo) IsDir() bool        { return false }
func (genericOutputFileInfo) Sys() any           { return nil }

func TestGenericCLIOutputPublicationHelpersCoverSuccessAndFailures(t *testing.T) {
	t.Parallel()
	testGenericCLIOutputPublicationSuccess(t)
	testGenericCLIOutputPublicationStageFailures(t)
	testGenericCLIOutputPublicationFailures(t)
}

func genericCLIOutputFixture() (context.Context, modelinference.InvokeModelResult, map[string]genericCLIOutputMapping) {
	return context.Background(), modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
		Name: "text", Content: "fixture bytes",
	}}}, map[string]genericCLIOutputMapping{"text": {slot: "text", path: "result.txt"}}
}

func testGenericCLIOutputPublicationSuccess(t *testing.T) {
	t.Helper()
	ctx, result, mappings := genericCLIOutputFixture()
	fileSystem := &genericOutputFileSystem{temporaryName: "result.tmp"}
	staged, err := stageGenericCLIOutputs(ctx, fileSystem, result, mappings)
	if err != nil || len(staged) != 1 || staged[0].temporary != "result.tmp" {
		t.Fatalf("stageGenericCLIOutputs() = (%#v, %v), want one staged output", staged, err)
	}
	published, err := publishGenericCLIOutputs(ctx, fileSystem, result, staged)
	if err != nil || published != 1 || len(fileSystem.renamed) != 1 {
		t.Fatalf("publishGenericCLIOutputs() = (%d, %v), renames=%#v", published, err, fileSystem.renamed)
	}
}

func testGenericCLIOutputPublicationStageFailures(t *testing.T) {
	t.Helper()
	ctx, result, mappings := genericCLIOutputFixture()
	if _, err := stageGenericCLIOutputs(ctx, &genericOutputFileSystem{}, result, map[string]genericCLIOutputMapping{}); err == nil || !strings.Contains(err.Error(), "unmapped output") {
		t.Fatalf("missing mapping error = %v, want unmapped output", err)
	}
	emptyResult := modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text"}}}
	if _, err := stageGenericCLIOutputs(ctx, &genericOutputFileSystem{}, emptyResult, mappings); err == nil || !strings.Contains(err.Error(), "no inline bytes") {
		t.Fatalf("empty output error = %v, want no inline bytes", err)
	}
	if _, err := stageGenericCLIOutputs(ctx, &genericOutputFileSystem{writeErr: errors.New("write failed")}, result, mappings); err == nil || !strings.Contains(err.Error(), "write mapped output") {
		t.Fatalf("write failure = %v, want mapped output context", err)
	}
	if _, err := stageGenericCLIOutputs(ctx, &genericOutputFileSystem{closeErr: errors.New("close failed")}, result, mappings); err == nil || !strings.Contains(err.Error(), "close failed") {
		t.Fatalf("close failure = %v, want close error", err)
	}
	if _, err := stageGenericCLIOutputs(ctx, &genericOutputFileSystem{shortWrite: true}, result, mappings); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("short write error = %v, want io.ErrShortWrite", err)
	}
}

func testGenericCLIOutputPublicationFailures(t *testing.T) {
	t.Helper()
	ctx, result, _ := genericCLIOutputFixture()
	fileSystem := &genericOutputFileSystem{temporaryName: "result.tmp"}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := publishGenericCLIOutputs(canceled, fileSystem, result, []genericCLIOutputStage{{targetPath: "result.txt", temporary: "result.tmp"}}); err == nil {
		t.Fatal("publishGenericCLIOutputs() error = nil for canceled context")
	}
	fileSystem.renameErr = errors.New("rename failed")
	if _, err := publishGenericCLIOutputs(ctx, fileSystem, result, []genericCLIOutputStage{{targetPath: "result.txt", temporary: "result.tmp"}}); err == nil || !strings.Contains(err.Error(), "publish mapped output") {
		t.Fatalf("rename failure = %v, want publish context", err)
	}
}

func TestGenericCLIOutputBackupAndRollbackHelpersCoverLifecycleBranches(t *testing.T) {
	t.Parallel()
	testGenericCLIOutputBackups(t)
	testGenericCLIOutputReserveFailures(t)
	testGenericCLIOutputRollback(t)
}

func testGenericCLIOutputBackups(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	mappings := []genericCLIOutputMapping{{slot: "text", path: "result.txt"}}
	missing := &genericOutputFileSystem{}
	backups, err := backupGenericCLIOutputTargets(ctx, missing, mappings)
	if err != nil || len(backups) != 0 {
		t.Fatalf("missing target backups = (%#v, %v), want none", backups, err)
	}

	existing := &genericOutputFileSystem{inspectExists: true, temporaryName: "backup.tmp"}
	backups, err = backupGenericCLIOutputTargets(ctx, existing, mappings)
	if err != nil || len(backups) != 1 || backups[0].backupPath != "backup.tmp" {
		t.Fatalf("existing target backups = (%#v, %v), want one backup", backups, err)
	}

	inspectFailure := &genericOutputFileSystem{inspectErr: errors.New("inspect failed")}
	if _, err := backupGenericCLIOutputTargets(ctx, inspectFailure, mappings); err == nil || !strings.Contains(err.Error(), "inspect mapped output") {
		t.Fatalf("inspect failure = %v, want inspect context", err)
	}
	renameFailure := &genericOutputFileSystem{inspectExists: true, temporaryName: "backup.tmp", renameErr: errors.New("backup rename failed")}
	if _, err := backupGenericCLIOutputTargets(ctx, renameFailure, mappings); err == nil || !strings.Contains(err.Error(), "backup mapped output") {
		t.Fatalf("backup rename failure = %v, want backup context", err)
	}
}

func testGenericCLIOutputReserveFailures(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for _, testCase := range []struct {
		name string
		fs   *genericOutputFileSystem
		want string
	}{
		{name: "create error", fs: &genericOutputFileSystem{createErr: errors.New("create failed")}, want: "create failed"},
		{name: "nil handle", fs: &genericOutputFileSystem{createNil: true}, want: "no named handle"},
		{name: "empty name", fs: &genericOutputFileSystem{emptyName: true}, want: "no named handle"},
		{name: "close error", fs: &genericOutputFileSystem{temporaryName: "backup.tmp", closeErr: errors.New("close failed")}, want: "close failed"},
		{name: "remove error", fs: &genericOutputFileSystem{temporaryName: "backup.tmp", removeErr: errors.New("remove failed")}, want: "remove failed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := reserveGenericCLIOutputPath(ctx, testCase.fs, "result.txt")
			if err == nil || (testCase.want != "" && !strings.Contains(err.Error(), testCase.want)) {
				t.Fatalf("reserveGenericCLIOutputPath() error = %v, want substring %q", err, testCase.want)
			}
		})
	}
}

func testGenericCLIOutputRollback(t *testing.T) {
	t.Helper()
	rollbackFS := &genericOutputFileSystem{}
	rollbackGenericCLIOutputPublication(
		rollbackFS,
		[]genericCLIOutputStage{{targetPath: "first", temporary: "first.tmp"}, {targetPath: "second", temporary: "second.tmp"}},
		[]genericCLIOutputBackup{{targetPath: "old", backupPath: "old.tmp"}}, 2,
	)
	removeGenericCLIOutputBackups(rollbackFS, []genericCLIOutputBackup{{backupPath: "one.tmp"}, {backupPath: "two.tmp"}})
	if len(rollbackFS.removed) < 4 || len(rollbackFS.renamed) != 1 {
		t.Fatalf("rollback cleanup calls = removed:%#v renamed:%#v", rollbackFS.removed, rollbackFS.renamed)
	}
}

type genericOutputErrorWriter struct{}

func (genericOutputErrorWriter) Write([]byte) (int, error) {
	return 0, errors.New("response write failed")
}

func TestWriteGenericCLIOutputMappingsClassifiesCompositionFailures(t *testing.T) {
	t.Parallel()

	result := modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{
		Name: "text", Content: "fixture bytes",
	}}}
	base := InvokeConfig{
		Context: context.Background(), OutputMappings: []string{"text=result.txt"}, Output: io.Discard,
	}

	if err := (&rootService{}).writeGenericCLIOutputMappings(base, result); err == nil || !strings.Contains(err.Error(), "output filesystem is required") {
		t.Fatalf("missing output filesystem error = %v", err)
	}
	if err := (&rootService{outputFileSystem: &genericOutputFileSystem{}}).writeGenericCLIOutputMappings(
		InvokeConfig{Context: base.Context, OutputMappings: []string{"invalid"}, Output: io.Discard}, result,
	); err == nil || !strings.Contains(err.Error(), "expected slot=path") {
		t.Fatalf("invalid mapping error = %v", err)
	}
	if err := (&rootService{outputFileSystem: &genericOutputFileSystem{}}).writeGenericCLIOutputMappings(
		InvokeConfig{Context: base.Context, OutputMappings: []string{"text=result.txt", "audio=audio.wav"}, Output: io.Discard}, result,
	); err == nil || !strings.Contains(err.Error(), "returned 1 outputs") {
		t.Fatalf("output count error = %v", err)
	}

	for _, testCase := range []struct {
		name   string
		fs     *genericOutputFileSystem
		output io.Writer
		want   string
	}{
		{name: "success", fs: &genericOutputFileSystem{temporaryName: "result.tmp"}, output: io.Discard, want: ""},
		{name: "backup failure", fs: &genericOutputFileSystem{temporaryName: "result.tmp", inspectErr: errors.New("inspect failed")}, output: io.Discard, want: "inspect mapped output"},
		{name: "publish failure", fs: &genericOutputFileSystem{temporaryName: "result.tmp", renameErr: errors.New("rename failed")}, output: io.Discard, want: "publish mapped output"},
		{name: "response write failure", fs: &genericOutputFileSystem{temporaryName: "result.tmp"}, output: genericOutputErrorWriter{}, want: "response write failed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := (&rootService{outputFileSystem: testCase.fs}).writeGenericCLIOutputMappings(
				InvokeConfig{Context: base.Context, OutputMappings: base.OutputMappings, Output: testCase.output}, result,
			)
			if testCase.want == "" {
				if err != nil {
					t.Fatalf("writeGenericCLIOutputMappings() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("writeGenericCLIOutputMappings() error = %v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestWriteGenericCLIOutputPathPublishesSingleOutput(t *testing.T) {
	t.Parallel()

	fileSystem := &genericOutputFileSystem{temporaryName: "speech.tmp"}
	var output strings.Builder
	err := (&rootService{outputFileSystem: fileSystem}).writeGenericCLIOutputPath(
		InvokeConfig{Context: context.Background(), OutputPath: "speech.wav", Output: &output},
		modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "audio", Content: "RIFF fixture"}}},
	)
	if err != nil {
		t.Fatalf("writeGenericCLIOutputPath() error = %v", err)
	}
	if output.String() != "Wrote audio: speech.wav\n" {
		t.Fatalf("output notice = %q, want published path", output.String())
	}
	if len(fileSystem.created) != 1 || string(fileSystem.created[0].data) != "RIFF fixture" {
		t.Fatalf("created output = %#v, want fixture bytes", fileSystem.created)
	}
	if len(fileSystem.renamed) != 1 || fileSystem.renamed[0][1] != "speech.wav" {
		t.Fatalf("published renames = %#v, want speech.wav target", fileSystem.renamed)
	}
}

func TestWriteGenericCLIOutputRejectsAmbiguousOrEmptyResults(t *testing.T) {
	t.Parallel()

	if err := writeGenericCLIOutputWithCatalog(io.Discard, modelinference.InvokeModelResult{}, modelinference.Detail{}, ""); err == nil || !strings.Contains(err.Error(), "multiple model outputs") {
		t.Fatalf("empty result error = %v, want multiple-output guidance", err)
	}
	if err := writeGenericCLIOutputWithCatalog(io.Discard, modelinference.InvokeModelResult{
		Outputs: []modelinference.InferenceOutput{{Name: "text"}, {Name: "other", Content: "value"}},
	}, modelinference.Detail{}, ""); err == nil || !strings.Contains(err.Error(), "multiple model outputs") {
		t.Fatalf("multiple result error = %v, want multiple-output guidance", err)
	}
	if err := writeGenericCLIOutputWithCatalog(io.Discard, modelinference.InvokeModelResult{
		Outputs: []modelinference.InferenceOutput{{Name: "text"}},
	}, modelinference.Detail{}, ""); err == nil || !strings.Contains(err.Error(), "no inline output") {
		t.Fatalf("empty output error = %v, want no-inline guidance", err)
	}
	if err := writeGenericCLIOutputWithCatalog(genericOutputErrorWriter{}, modelinference.InvokeModelResult{
		Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "value"}},
	}, modelinference.Detail{}, ""); err == nil || !strings.Contains(err.Error(), "response write failed") {
		t.Fatalf("output writer error = %v, want writer failure", err)
	}
}

func TestGenericCLIJSONResultHonorsExplicitBindingsAndOutputShape(t *testing.T) {
	t.Parallel()

	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "OMNI", Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}}}}
	result := modelinference.InvokeModelResult{Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "value"}}}
	if !genericCLIJSONResult(InvokeConfig{JSON: true, InputSpecs: []string{"{}"}}, catalog, "OMNI", result) {
		t.Fatal("genericCLIJSONResult() = false with explicit input specs, want true")
	}
	if !genericCLIJSONResult(InvokeConfig{JSON: true, ParameterSpecs: []string{"{}"}}, catalog, "OMNI", result) {
		t.Fatal("genericCLIJSONResult() = false with explicit parameter specs, want true")
	}
	if genericCLIJSONResult(InvokeConfig{}, catalog, "OMNI", result) {
		t.Fatal("genericCLIJSONResult() = true without JSON or explicit bindings, want false")
	}
	if genericCLIJSONResult(InvokeConfig{JSON: true}, catalog, "OMNI", modelinference.InvokeModelResult{}) {
		t.Fatal("genericCLIJSONResult() = true without outputs, want false")
	}
	if !genericCLIJSONResult(InvokeConfig{JSON: true}, catalog, "OMNI", modelinference.InvokeModelResult{
		Outputs: []modelinference.InferenceOutput{{Name: "text", Content: "one"}, {Name: "other", Content: "two"}},
	}) {
		t.Fatal("genericCLIJSONResult() = false with multiple outputs, want true")
	}
}

func TestValidateCLIOutputShapeRequiresAnUnambiguousPublicOutput(t *testing.T) {
	t.Parallel()

	textOperation := modelinference.Operation{
		Name: "OMNI", Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}
	multiOutputOperation := modelinference.Operation{
		Name: "OMNI", Outputs: []modelinference.OperationSlot{{Name: "text"}, {Name: "audio"}},
	}
	textCatalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{textOperation}}}
	multiCatalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{multiOutputOperation}}}
	if err := validateCLIOutputShape(InvokeConfig{JSON: false}, textCatalog, "OMNI"); err != nil {
		t.Fatalf("single text output shape error = %v, want nil", err)
	}
	if err := validateCLIOutputShape(InvokeConfig{OutputPath: "audio.wav"}, textCatalog, "OMNI"); err != nil {
		t.Fatalf("explicit output path shape error = %v, want nil", err)
	}
	for _, testCase := range []struct {
		name    string
		cfg     InvokeConfig
		catalog modelinference.Detail
		want    string
	}{
		{name: "mappings with output path", cfg: InvokeConfig{OutputPath: "result", OutputMappings: []string{"text=result"}}, catalog: textCatalog, want: "cannot be combined"},
		{name: "multiple outputs without json", cfg: InvokeConfig{}, catalog: multiCatalog, want: "multiple model outputs"},
		{name: "unknown operation", cfg: InvokeConfig{}, catalog: modelinference.Detail{}, want: "--output is required"},
		{name: "audio stdout output", cfg: InvokeConfig{}, catalog: modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{Name: "OMNI", Outputs: []modelinference.OperationSlot{{Name: "audio", Modality: modelinference.ModalityAudio}}}}}}, want: ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := validateCLIOutputShape(testCase.cfg, testCase.catalog, "OMNI")
			if testCase.want == "" {
				if err != nil {
					t.Fatalf("validateCLIOutputShape() error = %v, want nil", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("validateCLIOutputShape() error = %v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestHTTPServiceInvokeRejectsLocalOnlyGenericBindings(t *testing.T) {
	t.Parallel()

	service := &httpService{}
	base := InvokeConfig{
		Context: context.Background(), ModelName: "model", Operation: "OMNI", Text: "prompt", Output: io.Discard,
	}
	for _, testCase := range []struct {
		name   string
		mutate func(*InvokeConfig)
		want   string
	}{
		{name: "inputs", mutate: func(cfg *InvokeConfig) { cfg.InputSpecs = []string{"{}"} }, want: "explicit generic inputs"},
		{name: "parameters", mutate: func(cfg *InvokeConfig) { cfg.ParameterSpecs = []string{"{}"} }, want: "explicit generic parameters"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			cfg := base
			testCase.mutate(&cfg)
			err := service.Invoke(cfg)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("Invoke() error = %v, want substring %q", err, testCase.want)
			}
		})
	}
}

func TestJoinedCLIInvocationRequestReturnsTypedParsingErrors(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("generic-input-test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	_, err = joinedCLIInvocationRequest(scope, "model", "OMNI", "prompt", []string{`{`}, nil, modelinference.Detail{})
	if err == nil || !strings.Contains(err.Error(), "parse --input 1") {
		t.Fatalf("joinedCLIInvocationRequest() input error = %v", err)
	}
	_, err = joinedCLIInvocationRequest(scope, "model", "OMNI", "prompt", nil, []string{`{`}, modelinference.Detail{})
	if err == nil || !strings.Contains(err.Error(), "parse --parameter 1") {
		t.Fatalf("joinedCLIInvocationRequest() parameter error = %v", err)
	}
}

func TestReadModelsInvokeInputsReportsRepeatableValueKindErrors(t *testing.T) {
	t.Parallel()

	for _, inputID := range []string{modelsInvokeInputID, modelsInvokeParameterID} {
		t.Run(inputID, func(t *testing.T) {
			definitions := []resolvedinput.Definition{
				{ID: modelsInvokeNameInputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
				{ID: modelsInvokeOperationID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
				{ID: modelsInvokeTextID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
				{ID: inputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
				{ID: modelsInvokeOutputID, Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
			}
			candidates := []resolvedinput.Candidate{
				{InputID: modelsInvokeNameInputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("model")},
				{InputID: modelsInvokeOperationID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("OMNI")},
				{InputID: modelsInvokeTextID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("prompt")},
				{InputID: inputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("not-an-array")},
				{InputID: modelsInvokeOutputID, Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue("")},
			}
			inputs, err := resolvedinput.Resolve(definitions, candidates)
			if err != nil {
				t.Fatalf("resolve inputs: %v", err)
			}
			_, err = readModelsInvokeInputs(inputs)
			if err == nil || !strings.Contains(err.Error(), "read models invoke") {
				t.Fatalf("readModelsInvokeInputs() error = %v, want repeatable value-kind error", err)
			}
		})
	}
}

func TestGenericCLIInvocationRequestWithExplicitBindingsUsesDetachedInputs(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("generic-input-test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	request, err := joinedCLIInvocationRequest(scope, "model", "OMNI", "prompt", []string{
		`{"name":"first","modality":"IMAGE","contentType":"image/png","mediaType":"image/png","content":"one"}`,
		`{"name":"second","modality":"IMAGE","contentType":"image/png","mediaType":"image/png","content":"two"}`,
	}, []string{`{"name":"temperature","value":0.2}`}, modelinference.Detail{})
	if err != nil {
		t.Fatalf("joinedCLIInvocationRequest() error = %v", err)
	}
	if request.Scope != scope || request.Model.NameOrURI != "model" || request.Operation != "OMNI" || request.Holder != modelsCLIInvokeHolder {
		t.Fatalf("request identity = %#v", request)
	}
	if len(request.Inputs) != 2 || request.Inputs[0].Name != "first" || request.Inputs[1].Name != "second" {
		t.Fatalf("request inputs = %#v, want ordered explicit inputs", request.Inputs)
	}
	if len(request.Parameters) != 1 || request.Parameters[0].Name != "temperature" {
		t.Fatalf("request parameters = %#v, want explicit parameter", request.Parameters)
	}
}

func TestGenericCLIInvocationRequestWithoutBindingsUsesCatalogTextSlot(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("generic-input-test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	required := true
	request, err := joinedCLIInvocationRequest(scope, "model", "TTS", "hello", nil, nil, modelinference.Detail{
		Summary: modelinference.Summary{Operations: []modelinference.Operation{{
			Name: "TTS",
			Inputs: []modelinference.OperationSlot{
				{Name: "options", Modality: modelinference.ModalityJSON},
				{Name: "voice", Modality: modelinference.ModalityText, Required: &required, MediaTypes: []string{"text/plain"}},
			},
		}}},
	})
	if err != nil {
		t.Fatalf("joinedCLIInvocationRequest() error = %v", err)
	}
	if len(request.Inputs) != 1 || request.Inputs[0].Name != "voice" || request.Inputs[0].Content != "hello" || request.Inputs[0].ContentType != "text/plain" {
		t.Fatalf("catalog text request input = %#v", request.Inputs)
	}
}

type genericCLIModelsService struct {
	modelinference.Service
	catalog   modelinference.Detail
	readiness modelinference.Runtime
	request   modelinference.InvokeModelRequest
	invokeErr error
}

func (service *genericCLIModelsService) GetCatalogModel(
	context.Context, modelinference.GetModelRequest,
) (modelinference.GetModelResult, error) {
	return modelinference.GetModelResult{Model: service.catalog}, nil
}

func (service *genericCLIModelsService) GetModelReadiness(
	context.Context, modelinference.GetModelReadinessRequest,
) (modelinference.GetModelReadinessResult, error) {
	return modelinference.GetModelReadinessResult{Readiness: service.readiness}, nil
}

func (service *genericCLIModelsService) InvokeModel(
	_ context.Context, request modelinference.InvokeModelRequest,
) (modelinference.InvokeModelResult, error) {
	service.request = request
	if service.invokeErr != nil {
		return modelinference.InvokeModelResult{}, service.invokeErr
	}
	return modelinference.InvokeModelResult{
		ModelName: request.Model.NameOrURI,
		Operation: request.Operation,
		Outputs: []modelinference.InferenceOutput{{
			Name: "text", Modality: modelinference.ModalityText,
			ContentType: "text/plain", MediaType: "text/plain", Content: "fixture result",
		}},
	}, nil
}

func TestInvokeGenericInScopeClassifiesParsingAndBackendFailures(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("generic-input-test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	catalog := modelinference.Detail{Summary: modelinference.Summary{Operations: []modelinference.Operation{{
		Name: "OMNI", Inputs: []modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText}},
		Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
	}}}}
	base := InvokeConfig{Context: context.Background(), Output: io.Discard}

	parseRoot := &rootService{models: &genericCLIModelsService{catalog: catalog}}
	handled, err := parseRoot.invokeGenericInScope(
		InvokeConfig{Context: base.Context, Output: base.Output, InputSpecs: []string{`{`}},
		scope, "model", "OMNI", "prompt", catalog,
	)
	if !handled || err == nil || !strings.Contains(err.Error(), "parse --input 1") {
		t.Fatalf("parse failure = handled:%v error:%v, want handled with actionable error", handled, err)
	}

	backendRoot := &rootService{models: &genericCLIModelsService{catalog: catalog, invokeErr: errors.New("backend failed")}}
	handled, err = backendRoot.invokeGenericInScope(
		InvokeConfig{Context: base.Context, Output: base.Output, ParameterSpecs: []string{`{"name":"temperature","value":0.2}`}},
		scope, "model", "OMNI", "prompt", catalog,
	)
	if !handled || err == nil || !strings.Contains(err.Error(), "backend failed") {
		t.Fatalf("explicit backend failure = handled:%v error:%v, want handled with backend error", handled, err)
	}

	fallbackRoot := &rootService{models: &genericCLIModelsService{catalog: catalog, invokeErr: modelinference.ErrUnsupportedOperation}}
	handled, err = fallbackRoot.invokeGenericInScope(base, scope, "model", "OMNI", "prompt", catalog)
	if handled || err != nil {
		t.Fatalf("unsupported fallback = handled:%v error:%v, want false, nil", handled, err)
	}

	nonFallbackRoot := &rootService{models: &genericCLIModelsService{catalog: catalog, invokeErr: errors.New("unexpected backend failure")}}
	handled, err = nonFallbackRoot.invokeGenericInScope(base, scope, "model", "OMNI", "prompt", catalog)
	if !handled || err == nil || !strings.Contains(err.Error(), "unexpected backend failure") {
		t.Fatalf("unexpected backend failure = handled:%v error:%v, want handled error", handled, err)
	}
}

func TestRootServiceInvokeRoutesExplicitBindingsThroughGenericModelsRequest(t *testing.T) {
	t.Parallel()

	scope, err := (modelinference.RuntimeScopeRef{}).Parse("generic-input-test:scope")
	if err != nil {
		t.Fatalf("parse runtime scope: %v", err)
	}
	root := &genericCLIModelsService{
		catalog: modelinference.Detail{Summary: modelinference.Summary{
			Name: "model",
			Operations: []modelinference.Operation{{
				Name:    "OMNI",
				Inputs:  []modelinference.OperationSlot{{Name: "prompt", Modality: modelinference.ModalityText}},
				Outputs: []modelinference.OperationSlot{{Name: "text", Modality: modelinference.ModalityText}},
			}},
		}},
	}
	service := NewService(Config{
		Models: root,
		OpenInvokeScope: func(context.Context, InvokeConfig) (InvokeRuntimeScope, error) {
			return InvokeRuntimeScope{Scope: scope}, nil
		},
	})
	if service == nil {
		t.Fatal("NewService() = nil, want Models CLI service")
	}

	var output strings.Builder
	err = service.Invoke(InvokeConfig{
		Context: context.Background(), ModelName: "model", Operation: "OMNI",
		InputSpecs: []string{
			`{"name":"first","modality":"IMAGE","contentType":"image/png","mediaType":"image/png","content":"one"}`,
			`{"name":"second","modality":"IMAGE","contentType":"image/png","mediaType":"image/png","content":"two"}`,
		},
		ParameterSpecs: []string{`{"name":"temperature","value":0.2}`},
		JSON:           true, Output: &output,
	})
	if err != nil {
		t.Fatalf("Invoke() error = %v", err)
	}
	if len(root.request.Inputs) != 2 || root.request.Inputs[0].Name != "first" || root.request.Inputs[1].Name != "second" {
		t.Fatalf("Models request inputs = %#v, want ordered explicit inputs", root.request.Inputs)
	}
	if len(root.request.Parameters) != 1 || root.request.Parameters[0].Name != "temperature" {
		t.Fatalf("Models request parameters = %#v, want explicit parameter", root.request.Parameters)
	}
	if !strings.Contains(output.String(), "fixture result") {
		t.Fatalf("Invoke() output = %q, want generic result", output.String())
	}
}

func assertInvokeCommandConfig(t *testing.T, cfg InvokeConfig, server string, logger *zap.Logger, diagnostics io.Writer) {
	t.Helper()
	assertInvokeCommandValues(t, cfg)
	assertInvokeBindingValues(t, cfg)
	assertInvokeGlobalValues(t, cfg, server)
	assertInvokeDependencyValues(t, cfg, logger, diagnostics)
}

func assertInvokeCommandValues(t *testing.T, cfg InvokeConfig) {
	t.Helper()
	if cfg.ModelName != "OMNIVOICE_Q4_K_M" || cfg.Operation != "TTS" || cfg.Text != "hello" || cfg.OutputPath != "speech.wav" {
		t.Fatalf("InvokeConfig command values = %#v", cfg)
	}
}

func assertInvokeBindingValues(t *testing.T, cfg InvokeConfig) {
	t.Helper()
	if len(cfg.InputMappings) != 1 || cfg.InputMappings[0] != `{"name":"prompt","modality":"TEXT","contentType":"text/plain","mediaType":"text/plain","content":"hello"}` {
		t.Fatalf("InvokeConfig input values = %#v", cfg.InputMappings)
	}
	if len(cfg.ParameterSpecs) != 1 || cfg.ParameterSpecs[0] != `{"name":"temperature","value":0.2}` {
		t.Fatalf("InvokeConfig parameter specs = %#v", cfg.ParameterSpecs)
	}
}

func assertInvokeGlobalValues(t *testing.T, cfg InvokeConfig, server string) {
	t.Helper()
	if cfg.Server != server || !cfg.JSON || !cfg.Verbose || !cfg.Debug {
		t.Fatalf("InvokeConfig global values = %#v", cfg)
	}
}

func assertInvokeDependencyValues(t *testing.T, cfg InvokeConfig, logger *zap.Logger, diagnostics io.Writer) {
	t.Helper()
	if cfg.FactoryDir != "" || cfg.HomeDir != "/home/tester" || cfg.Logger != logger || cfg.Diagnostics != diagnostics {
		t.Fatalf("InvokeConfig dependencies = %#v", cfg)
	}
}
