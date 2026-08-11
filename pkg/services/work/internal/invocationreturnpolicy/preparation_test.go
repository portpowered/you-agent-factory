package invocationreturnpolicy

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"
)

func TestResolveFactoryInvocationInput_NoInputReturnsEmptyWithoutError(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared != (PreparedInvocationInput{}) {
		t.Fatalf("prepared = %#v, want empty", prepared)
	}
}

func TestResolveFactoryInvocationInput_PositionalOnly(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"Fix", "the", "lint", "issues"}})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != InputSourcePositionalText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix the lint issues" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_NoSignatureDoesNotActivateNamedSemantics(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Arguments: []string{"--mode", "fast"},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "--mode fast" {
		t.Fatalf("prepared = %#v, want literal compatibility text", prepared)
	}
	if prepared.NormalizedArguments != nil {
		t.Fatalf("normalized arguments = %#v, want nil without a signature", prepared.NormalizedArguments)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromDash(t *testing.T) {
	stdin := "Fix the tests\n"
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &stdin})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != InputSourceStdinText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix the tests\n" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromPipe(t *testing.T) {
	stdin := "Fix from pipe\n"
	prepared, err := newTestPreparation().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.Source != InputSourceStdinText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix from pipe\n" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_StdinOnlyFromOverriddenReaderWithoutTTYHook(t *testing.T) {
	stdin := "Fix from overridden reader\n"
	prepared, err := newTestPreparation().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{StdinText: &stdin})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "Fix from overridden reader\n" {
		t.Fatalf("prepared = %#v", prepared)
	}
}

func TestResolveFactoryInvocationInput_PreservesSurroundingWhitespace(t *testing.T) {
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"  keep", "surrounding", "whitespace  "}})
	if err != nil || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != "  keep surrounding whitespace  " {
		t.Fatalf("prepared = %#v, err = %v", prepared, err)
	}
}

func TestResolveFactoryInvocationInput_StdinPreservesSurroundingWhitespace(t *testing.T) {
	want := "  keep surrounding whitespace  "
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &want})
	if err != nil || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != want {
		t.Fatalf("prepared = %#v, err = %v", prepared, err)
	}
}

func TestResolveFactoryInvocationInput_FilePreservesExactUTF8BytesAndPath(t *testing.T) {
	path := `briefs\long prompt.txt`
	want := "  line one\r\nline two — 東京\r\n\r\nfinal line\n"
	var readPath string
	preparation := newFilePreparation(InvocationInputFileReader(func(got string) ([]byte, error) {
		readPath = got
		return []byte(want), nil
	}))
	prepared, err := preparation.PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{FilePath: &path})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if readPath != path {
		t.Fatalf("read path = %q, want %q", readPath, path)
	}
	if prepared.Source != InputSourceFileText || prepared.ResolvedInput == nil || prepared.ResolvedInput.Text != want {
		t.Fatalf("prepared = %#v, want exact file text %q", prepared, want)
	}
	if len(prepared.ResolvedInput.Content) != 1 || prepared.ResolvedInput.Content[0].Text != want {
		t.Fatalf("content = %#v, want exact file text", prepared.ResolvedInput.Content)
	}
}

func TestResolveFactoryInvocationInput_FileRejectsInvalidUTF8AndEmptyText(t *testing.T) {
	path := "brief.txt"
	tests := []struct {
		name string
		data []byte
		want InputErrorCode
	}{
		{name: "invalid utf8", data: []byte{0xff, 0xfe}, want: InputErrorCodeInvalidUTF8},
		{name: "empty", data: []byte(" \r\n\t"), want: InputErrorCodeEmpty},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparation := newFilePreparation(InvocationInputFileReader(func(string) ([]byte, error) {
				return test.data, nil
			}))
			_, err := preparation.PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{FilePath: &path})
			assertInputErrorCode(t, err, test.want)
			if !strings.Contains(err.Error(), path) {
				t.Fatalf("error = %v, want file path %q", err, path)
			}
		})
	}
}

func TestResolveFactoryInvocationInput_RejectsNonRegularFileBeforeRead(t *testing.T) {
	path := "brief.pipe"
	readCalled := false
	preparation := NewInvocationInputPreparation(
		func(string) ([]byte, error) {
			readCalled = true
			return []byte("must not read"), nil
		},
		func(string) (fs.FileInfo, error) { return fileInfoStub{mode: fs.ModeNamedPipe}, nil },
	)

	_, err := preparation.PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{FilePath: &path})
	assertInputErrorCode(t, err, InputErrorCodeNotRegularFile)
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("error = %v, want source/path-specific regular-file diagnostic", err)
	}
	if readCalled {
		t.Fatal("file reader was called for a non-regular path")
	}
}

func TestResolveFactoryInvocationInput_FileConflictsWithPositionalStdinAndSignatureTo(t *testing.T) {
	path := "brief.txt"
	fileReader := InvocationInputFileReader(func(string) ([]byte, error) { return []byte("from file"), nil })
	stdin := "from stdin"
	positional := InvocationInputPreparationRequest{Arguments: []string{"from positional"}, FilePath: &path}
	_, err := newFilePreparation(fileReader).PrepareInvocationInput(context.Background(), positional)
	assertInputErrorCode(t, err, InputErrorCodeSourceConflict)
	if !strings.Contains(err.Error(), "positional_text") || !strings.Contains(err.Error(), "file_text") {
		t.Fatalf("positional/file conflict = %v, want both source names", err)
	}

	_, err = newFilePreparation(fileReader).PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"-"}, StdinText: &stdin, FilePath: &path,
	})
	assertInputErrorCode(t, err, InputErrorCodeSourceConflict)
	if !strings.Contains(err.Error(), "stdin_text") || !strings.Contains(err.Error(), "file_text") {
		t.Fatalf("stdin/file conflict = %v, want both source names", err)
	}

	_, err = newFilePreparation(fileReader).PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"--to", "from named"}, Signature: signatureWithTo(), FilePath: &path,
	})
	assertInputErrorCode(t, err, InputErrorCodeSourceConflict)
	if !strings.Contains(err.Error(), "--to-file") || !strings.Contains(err.Error(), "--to") {
		t.Fatalf("named/file conflict = %v, want both flag names", err)
	}
}

func TestResolveSignatureFactoryInvocationInput_FilePopulatesPrimaryArgument(t *testing.T) {
	path := "brief.txt"
	want := "multiline — exact\r\n"
	prepared, err := newFilePreparation(InvocationInputFileReader(func(string) ([]byte, error) {
		return []byte(want), nil
	})).PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Signature: signatureWithTo(), FilePath: &path,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	argument := prepared.NormalizedArguments.Arguments["input"]
	if len(argument.Values) != 1 || argument.Values[0] != want {
		t.Fatalf("primary argument = %#v, want exact %q", argument, want)
	}
	if len(argument.Sources) != 1 || argument.Sources[0].Kind != ArgumentSourceKindFile {
		t.Fatalf("primary sources = %#v, want file source", argument.Sources)
	}
}

func TestResolveSignatureFactoryInvocationInput_FileRequiresPrimaryTextParameter(t *testing.T) {
	path := "brief.txt"
	_, err := newFilePreparation(InvocationInputFileReader(func(string) ([]byte, error) {
		return []byte("file prompt"), nil
	})).PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Signature: &InvocationSignatureConfig{Parameters: []InvocationParameterConfig{{
			Name:     "mode",
			TypeHint: typeHintBooleanString,
			Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}},
		}}},
		FilePath: &path,
	})
	assertArgumentErrorCode(t, err, ArgumentErrorCodeInvalidActiveSignature)
}

func TestResolveFactoryInvocationInput_ExplicitEmptyPositionalUsesStableEmptyCode(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{""}})
	assertInputErrorCode(t, err, InputErrorCodeEmpty)
}

func TestResolveFactoryInvocationInput_RejectsWhitespaceOnlyPositional(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"   "}})
	assertInputErrorCode(t, err, InputErrorCodeEmpty)
}

func TestResolveFactoryInvocationInput_ExplicitEmptyStdinUsesStableEmptyCode(t *testing.T) {
	empty := ""
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: []string{"-"}, StdinText: &empty})
	assertInputErrorCode(t, err, InputErrorCodeEmpty)
}

func TestResolveFactoryInvocationInput_RejectsPositionalAndStdinConflict(t *testing.T) {
	stdin := "Fix from stdin\n"
	_, err := newTestPreparation().PrepareInvocationInput(context.Background(), InvocationInputPreparationRequest{
		Arguments: []string{"Fix", "the", "lint", "issues"}, StdinText: &stdin,
	})
	assertInputErrorCode(t, err, InputErrorCodeSourceConflict)
}

func TestResolveSignatureFactoryInvocationInput_NormalizesPositionalNamedBooleanAndStdin(t *testing.T) {
	stdin := "from stdin"
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Arguments: []string{"draft", "--mode", "fast", "--confirm", "--out=result.md", "-"},
		Signature: signatureFactoryInvocationConfig(), StdinText: &stdin,
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	got := prepared.NormalizedArguments
	if got == nil {
		t.Fatal("normalized arguments are nil")
	}
	for name, want := range map[string]string{"input": "draft", "mode": "fast", "confirm": "true", "stdinText": "from stdin", "output": "result.md"} {
		if values := got.Arguments[name].Values; len(values) != 1 || values[0] != want {
			t.Fatalf("%s values = %#v, want [%q]", name, values, want)
		}
	}
}

func TestPrepareInvocationInputDirectArgsUsesPublishedRequestShape(t *testing.T) {
	t.Parallel()

	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Signature:  signatureFactoryInvocationConfig(),
		DirectArgs: []NamedArgumentInput{{Key: "input", Values: []string{"draft"}}},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.NormalizedArguments == nil || prepared.NormalizedArguments.Arguments["input"].Values[0] != "draft" {
		t.Fatalf("prepared = %#v, want direct args normalization", prepared)
	}
}

func TestPrepareInvocationInputCompatibilityContentUsesPublishedRequestShape(t *testing.T) {
	t.Parallel()

	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		CompatibilityContent: []ContentPart{{Type: ContentPartTypeText, Text: "hello"}},
	})
	if err != nil {
		t.Fatalf("PrepareInvocationInput: %v", err)
	}
	if prepared.ResolvedInput == nil || len(prepared.ResolvedInput.Content) != 1 || prepared.ResolvedInput.Content[0].Text != "hello" {
		t.Fatalf("prepared = %#v, want compatibility hello content", prepared)
	}
}

func TestResolveSignatureFactoryInvocationInput_RejectsMissingNamedValue(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Arguments: []string{"draft", "--mode"}, Signature: signatureFactoryInvocationConfig(),
	})
	if err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("error = %v, want missing value", err)
	}
	var argumentErr *ArgumentError
	if !errors.As(err, &argumentErr) || argumentErr.Code != ArgumentErrorCodeMissingValue {
		t.Fatalf("error = %v, want %s", err, ArgumentErrorCodeMissingValue)
	}
}

func TestResolveSignatureFactoryInvocationInput_RejectsNamedValueBeforeNextFlag(t *testing.T) {
	_, err := prepareInvocationInput(t, InvocationInputPreparationRequest{
		Arguments: []string{"draft", "--mode", "--other", "value"}, Signature: signatureFactoryInvocationConfig(),
	})
	if err == nil || !strings.Contains(err.Error(), "factory argument --mode requires a value") {
		t.Fatalf("error = %v, want missing value before next flag", err)
	}
	var argumentErr *ArgumentError
	if !errors.As(err, &argumentErr) || argumentErr.Code != ArgumentErrorCodeMissingValue {
		t.Fatalf("error = %v, want %s", err, ArgumentErrorCodeMissingValue)
	}
}

func TestNormalizeLegacyInvocationExampleRejectsUnstructuredInputs(t *testing.T) {
	t.Parallel()

	normalizer := InvocationExampleNormalizer{}
	if _, err := normalizer.NormalizeLegacyInvocationExample(
		[]string{"draft", "--mode"},
		signatureFactoryInvocationConfig(),
		nil,
	); err == nil || !strings.Contains(err.Error(), "requires a value") {
		t.Fatalf("missing named value error = %v", err)
	}
	if _, err := normalizer.NormalizeLegacyInvocationExample([]string{"free-form input"}, nil, nil); err == nil ||
		!strings.Contains(err.Error(), "does not resolve to structured invocation arguments") {
		t.Fatalf("unstructured compatibility input error = %v", err)
	}
}

func TestPrepareInvocationInputReturnsDetachedCanonicalValues(t *testing.T) {
	signature := signatureFactoryInvocationConfig()
	arguments := []string{"draft", "--mode=fast"}
	prepared, err := prepareInvocationInput(t, InvocationInputPreparationRequest{Arguments: arguments, Signature: signature})
	if err != nil {
		t.Fatal(err)
	}
	arguments[0] = "changed"
	signature.Parameters[0].Name = "changed"
	if got := prepared.NormalizedArguments.Arguments["input"].Values[0]; got != "draft" {
		t.Fatalf("detached input = %q", got)
	}
}

func TestPrepareInvocationInputRequiresLiveContext(t *testing.T) {
	preparation := newTestPreparation()
	if _, err := preparation.PrepareInvocationInput(nil, InvocationInputPreparationRequest{}); err == nil || !strings.Contains(err.Error(), "context is required") {
		t.Fatalf("nil context error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := preparation.PrepareInvocationInput(ctx, InvocationInputPreparationRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error = %v", err)
	}
}

func prepareInvocationInput(t *testing.T, request InvocationInputPreparationRequest) (PreparedInvocationInput, error) {
	t.Helper()
	return newTestPreparation().PrepareInvocationInput(context.Background(), request)
}

func newTestPreparation() InvocationInputPreparation {
	return NewInvocationInputPreparation(nil, nil)
}

func newFilePreparation(reader InvocationInputFileReader) InvocationInputPreparation {
	return NewInvocationInputPreparation(reader, regularFilePathInspector)
}

func regularFilePathInspector(string) (fs.FileInfo, error) {
	return fileInfoStub{mode: 0o600}, nil
}

type fileInfoStub struct {
	mode fs.FileMode
}

func (fileInfoStub) Name() string         { return "prompt.txt" }
func (fileInfoStub) Size() int64          { return 1 }
func (f fileInfoStub) Mode() fs.FileMode  { return f.mode }
func (f fileInfoStub) ModTime() time.Time { return time.Time{} }
func (f fileInfoStub) IsDir() bool        { return f.mode.IsDir() }
func (fileInfoStub) Sys() any             { return nil }

func assertInputErrorCode(t *testing.T, err error, code InputErrorCode) {
	t.Helper()
	var inputErr *InputError
	if !errors.As(err, &inputErr) || inputErr.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func signatureFactoryInvocationConfig() *InvocationSignatureConfig {
	return &InvocationSignatureConfig{Parameters: []InvocationParameterConfig{
		{Name: "input", Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindPositional, Position: 1}}},
		{Name: "mode", ExternalName: "mode", Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}}},
		{Name: "confirm", TypeHint: typeHintBooleanString, Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}}},
		{Name: "stdinText", Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindStdin}}},
		{Name: "output", ExternalName: "output", Aliases: []string{"out"}, Bindings: []InvocationParameterBindingConfig{{Kind: bindingKindNamed}}},
	}}
}

func signatureWithTo() *InvocationSignatureConfig {
	return &InvocationSignatureConfig{Parameters: []InvocationParameterConfig{{
		Name:         "input",
		ExternalName: "to",
		Required:     true,
		Bindings:     []InvocationParameterBindingConfig{{Kind: bindingKindPositional, Position: 1}, {Kind: bindingKindNamed}},
	}}}
}
