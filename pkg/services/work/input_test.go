package work

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work/internal/stateaccessquery"
)

func TestResolveTextInput_PositionalOnly(t *testing.T) {
	text := "build the thing"

	got, err := ResolveTextInput(TextInputSources{PositionalText: &text})
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}

	assertResolvedTextInput(t, got, InputSourcePositionalText, text)
}

func TestResolveTextInput_StdinOnly(t *testing.T) {
	text := "build from stdin\n"

	got, err := ResolveTextInput(TextInputSources{StdinText: &text})
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}

	assertResolvedTextInput(t, got, InputSourceStdinText, text)
}

func TestResolveTextInput_FileOnly(t *testing.T) {
	text := "line one\r\nline two\n"

	got, err := ResolveTextInput(TextInputSources{FileText: &text})
	if err != nil {
		t.Fatalf("ResolveTextInput: %v", err)
	}

	assertResolvedTextInput(t, got, InputSourceFileText, text)
}

func TestResolveTextInput_RejectsFileAndPositionalConflict(t *testing.T) {
	positional := "from args"
	file := "from file"

	_, err := ResolveTextInput(TextInputSources{
		PositionalText: &positional,
		FileText:       &file,
	})

	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if inputErr.Code != InputErrorCodeSourceConflict {
		t.Fatalf("code = %q, want %q", inputErr.Code, InputErrorCodeSourceConflict)
	}
	want := []InputSourceLabel{InputSourcePositionalText, InputSourceFileText}
	if !reflect.DeepEqual(inputErr.ConflictingSources, want) {
		t.Fatalf("conflicting sources = %#v, want %#v", inputErr.ConflictingSources, want)
	}
}

func TestResolveTextInput_RejectsEmptyFile(t *testing.T) {
	file := "  \n\t"

	_, err := ResolveTextInput(TextInputSources{FileText: &file})

	assertInputEmptyError(t, err, InputSourceFileText)
}

func TestResolveTextInput_RejectsPositionalAndStdinConflict(t *testing.T) {
	positional := "from args"
	stdin := "from stdin"

	_, err := ResolveTextInput(TextInputSources{
		PositionalText: &positional,
		StdinText:      &stdin,
	})

	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if inputErr.Code != InputErrorCodeSourceConflict {
		t.Fatalf("code = %q, want %q", inputErr.Code, InputErrorCodeSourceConflict)
	}
	want := []InputSourceLabel{InputSourcePositionalText, InputSourceStdinText}
	if len(inputErr.ConflictingSources) != len(want) {
		t.Fatalf("conflicting sources = %#v, want %#v", inputErr.ConflictingSources, want)
	}
	for i := range want {
		if inputErr.ConflictingSources[i] != want[i] {
			t.Fatalf("conflicting sources = %#v, want %#v", inputErr.ConflictingSources, want)
		}
	}
}

func TestResolveTextInput_RejectsEmptySelectedStdin(t *testing.T) {
	stdin := ""

	_, err := ResolveTextInput(TextInputSources{StdinText: &stdin})

	assertInputEmptyError(t, err, InputSourceStdinText)
}

func TestResolveTextInput_RejectsWhitespaceOnlyPositional(t *testing.T) {
	text := "   "

	_, err := ResolveTextInput(TextInputSources{PositionalText: &text})

	assertInputEmptyError(t, err, InputSourcePositionalText)
}

func TestResolveTextInput_RejectsWhitespaceOnlyStdin(t *testing.T) {
	stdin := "  \n\t  "

	_, err := ResolveTextInput(TextInputSources{StdinText: &stdin})

	assertInputEmptyError(t, err, InputSourceStdinText)
}

func TestResolveAPITextInputContent_RejectsWhitespaceOnlyText(t *testing.T) {
	_, err := ResolveAPITextInputContent([]WorkContentPart{{
		Type: WorkContentPartTypeText,
		Text: "   ",
	}})

	assertInputEmptyError(t, err, InputSourcePositionalText)
}

func TestResolveAPITextInputContent_RejectsWhitespaceOnlyJoinedParts(t *testing.T) {
	_, err := ResolveAPITextInputContent([]WorkContentPart{
		{Type: WorkContentPartTypeText, Text: "  "},
		{Type: WorkContentPartTypeText, Text: "\t"},
	})

	assertInputEmptyError(t, err, InputSourcePositionalText)
}

func assertInputEmptyError(t *testing.T, err error, wantSource InputSourceLabel) {
	t.Helper()

	var inputErr *InputError
	if !errors.As(err, &inputErr) {
		t.Fatalf("error = %v, want InputError", err)
	}
	if inputErr.Code != InputErrorCodeEmpty {
		t.Fatalf("code = %q, want %q", inputErr.Code, InputErrorCodeEmpty)
	}
	if inputErr.Source != wantSource {
		t.Fatalf("source = %q, want %q", inputErr.Source, wantSource)
	}
}

func assertResolvedTextInput(t *testing.T, got ResolvedInput, wantSource InputSourceLabel, wantText string) {
	t.Helper()

	if got.Source != wantSource {
		t.Fatalf("source = %q, want %q", got.Source, wantSource)
	}
	if got.Text != wantText {
		t.Fatalf("text = %q, want %q", got.Text, wantText)
	}
	if len(got.Content) != 1 {
		t.Fatalf("content = %#v, want one text part", got.Content)
	}
	if got.Content[0].Type != WorkContentPartTypeText {
		t.Fatalf("content[0].type = %q, want %q", got.Content[0].Type, WorkContentPartTypeText)
	}
	if got.Content[0].Text != wantText {
		t.Fatalf("content[0].text = %q, want %q", got.Content[0].Text, wantText)
	}
}

func TestWorkRequestJSONPreservesPublicContract(t *testing.T) {
	request := WorkRequest{
		RequestID: "request-1",
		Type:      WorkRequestTypeFactoryRequestBatch,
		Works: []Work{{
			Name:       "draft",
			WorkTypeID: "story",
			Content: []WorkContentPart{
				{Type: WorkContentPartTypeText, Text: "hello"},
				{Type: WorkContentPartTypeJSON, JSON: json.RawMessage(`{"answer":42}`)},
			},
		}},
	}

	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal WorkRequest: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("decode WorkRequest JSON: %v", err)
	}
	if decoded["requestId"] != "request-1" || decoded["type"] != "FACTORY_REQUEST_BATCH" {
		t.Fatalf("public request fields changed: %#v", decoded)
	}
	if _, present := decoded["currentChainingTraceId"]; present {
		t.Fatalf("omitted chaining trace was serialized: %#v", decoded)
	}
}

func TestCloneWorkDispatchDetachesMutableState(t *testing.T) {
	dispatch := WorkDispatch{
		PreviousChainingTraceIDs: []string{"trace-1"},
		Execution:                ExecutionMetadata{WorkIDs: []string{"work-1"}},
		InputTokens:              []any{"input"},
		InputBindings:            map[string][]string{"slot": {"work-1"}},
	}
	clone := CloneWorkDispatch(dispatch)

	clone.PreviousChainingTraceIDs[0] = "changed"
	clone.Execution.WorkIDs[0] = "changed"
	clone.InputTokens[0] = "changed"
	clone.InputBindings["slot"][0] = "changed"

	if !reflect.DeepEqual(dispatch.PreviousChainingTraceIDs, []string{"trace-1"}) ||
		!reflect.DeepEqual(dispatch.Execution.WorkIDs, []string{"work-1"}) ||
		!reflect.DeepEqual(dispatch.InputTokens, []any{"input"}) ||
		!reflect.DeepEqual(dispatch.InputBindings, map[string][]string{"slot": {"work-1"}}) {
		t.Fatalf("clone mutated source dispatch: %#v", dispatch)
	}
}

func TestPayloadLineageSnapshotsDetachContent(t *testing.T) {
	item := FactoryWorkItem{
		ID:      "work-1",
		TraceID: "trace-1",
		Content: []WorkContentPart{{
			Type:     WorkContentPartTypeText,
			Text:     "original",
			Metadata: map[string]any{"source": "request"},
		}},
	}
	var projection WorkPayloadLineageProjection
	projection.RecordWorkRequestSnapshot(1, "request-1", item)
	item.Content[0].Text = "changed"
	item.Content[0].Metadata["source"] = "changed"

	resolved := projection.ResolveInitialSubmittedSnapshot("work-1")
	if resolved.Status != WorkPayloadResolutionResolved || resolved.Snapshot == nil {
		t.Fatalf("resolution = %#v", resolved)
	}
	got := resolved.Snapshot.WorkItem.Content[0]
	if got.Text != "original" || got.Metadata["source"] != "request" {
		t.Fatalf("snapshot content was not detached: %#v", got)
	}
}

func TestContentFromWorkerOutputReturnsPlainText(t *testing.T) {
	got, err := ContentFromWorkerOutput("worker response")
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != WorkContentPartTypeText || got[0].Text != "worker response" {
		t.Fatalf("content = %#v, want one text response part", got)
	}
}

func TestContentFromWorkerOutputParsesCanonicalParts(t *testing.T) {
	raw, err := json.Marshal([]WorkContentPart{{
		Type: WorkContentPartTypeText,
		Text: "structured response",
	}})
	if err != nil {
		t.Fatalf("marshal parts: %v", err)
	}
	got, err := ContentFromWorkerOutput(string(raw))
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Text != "structured response" {
		t.Fatalf("content = %#v, want structured response part", got)
	}
}

func TestContentFromWorkerOutputNormalizesSupportedParts(t *testing.T) {
	got, err := ContentFromWorkerOutput(`{"content":[{"type":"TEXT","text":"structured response"},{"type":"unknown","text":"ignored"}]}`)
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if len(got) != 1 || got[0].Type != WorkContentPartTypeText || got[0].Text != "structured response" {
		t.Fatalf("content = %#v, want one normalized text response part", got)
	}
}

func TestContentFromWorkerOutputReturnsNilForEmptyOutput(t *testing.T) {
	got, err := ContentFromWorkerOutput("   ")
	if err != nil {
		t.Fatalf("ContentFromWorkerOutput: %v", err)
	}
	if got != nil {
		t.Fatalf("content = %#v, want nil", got)
	}
}

func TestContentURLNormalization(t *testing.T) {
	dir := t.TempDir()
	absolutePath := filepath.Join(dir, "image.png")
	urlPath := filepath.ToSlash(absolutePath)
	if volume := filepath.VolumeName(absolutePath); volume != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	wantAbsoluteURL := (&url.URL{Scheme: "file", Path: urlPath}).String()

	got, err := FilesystemPathToContentURL(absolutePath)
	if err != nil {
		t.Fatalf("FilesystemPathToContentURL: %v", err)
	}
	if got != wantAbsoluteURL {
		t.Fatalf("absolute content URL = %q, want %q", got, wantAbsoluteURL)
	}

	part, err := NormalizeFileBackedContentPart(WorkContentPart{
		Type: WorkContentPartTypeImage,
		File: "fixtures/ui.png",
	})
	if err != nil {
		t.Fatalf("NormalizeFileBackedContentPart: %v", err)
	}
	if part.URL != "file://fixtures/ui.png" || part.File != "" {
		t.Fatalf("normalized part = %#v", part)
	}
}

func TestResolveDispatchContentURL(t *testing.T) {
	workspace := t.TempDir()
	got, err := ResolveDispatchContentURL(workspace, "file://fixtures/ui.png")
	if err != nil {
		t.Fatalf("ResolveDispatchContentURL: %v", err)
	}
	want, err := FilesystemPathToContentURL(filepath.Join(workspace, "fixtures", "ui.png"))
	if err != nil {
		t.Fatalf("FilesystemPathToContentURL: %v", err)
	}
	if got != want {
		t.Fatalf("resolved URL = %q, want %q", got, want)
	}
}

func TestValidateContentURL(t *testing.T) {
	for _, rawURL := range []string{
		"file:///tmp/example.png",
		"https://example.com/a.png",
		"http://example.com/a.png",
		"data:image/png;base64,AAAA",
	} {
		if err := ValidateContentURL(rawURL); err != nil {
			t.Fatalf("ValidateContentURL(%q): %v", rawURL, err)
		}
	}
	if err := ValidateContentURL("ftp://example.com/a.png"); err == nil {
		t.Fatal("ValidateContentURL accepted unsupported scheme")
	}
}

// The mapping adapter is isolated from the query policy by its injected role.
type controlledListPreparation struct {
	prepare func(context.Context, stateaccessquery.ListOptions) (stateaccessquery.PreparedListRequest, error)
}

func (p controlledListPreparation) PrepareListRequest(ctx context.Context, options stateaccessquery.ListOptions) (stateaccessquery.PreparedListRequest, error) {
	return p.prepare(ctx, options)
}
func TestListRequestPreparationMapsDetachedResults(t *testing.T) {
	t.Parallel()
	parentContext := t.Context()
	role := NewListRequestPreparation(controlledListPreparation{prepare: func(ctx context.Context, options stateaccessquery.ListOptions) (stateaccessquery.PreparedListRequest, error) {
		if ctx != parentContext || options.Name != "selected" || !options.Counts {
			t.Fatal("request mapping changed")
		}
		options.MaxResults = 7
		return stateaccessquery.PreparedListRequest{Options: options, FilterSummary: "name"}, nil
	}})
	first, err := role.PrepareListRequest(t.Context(), ListOptions{Name: "selected", Counts: true})
	if err != nil || first.Options.MaxResults != 7 || first.FilterSummary != "name" {
		t.Fatalf("prepared = %#v, %v", first, err)
	}
	first.Options.Name = "mutated"
	second, err := role.PrepareListRequest(t.Context(), ListOptions{Name: "selected", Counts: true})
	if err != nil || second.Options.Name != "selected" {
		t.Fatalf("repeated result = %#v, %v", second, err)
	}
	for i := 0; i < 3; i++ {
		t.Run(string(rune('a'+i)), func(t *testing.T) {
			t.Parallel()
			got, err := role.PrepareListRequest(parentContext, ListOptions{Name: "selected", Counts: true})
			if err != nil || got.Options.Name != "selected" || got.Options.MaxResults != 7 {
				t.Fatalf("concurrent result = %#v, %v", got, err)
			}
		})
	}
}

func TestListRequestPreparationPreservesTypedErrors(t *testing.T) {
	t.Parallel()
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded, &stateaccessquery.ValidationError{Field: "state.type", Message: "invalid state type"}} {
		t.Run(cause.Error(), func(t *testing.T) {
			t.Parallel()
			role := NewListRequestPreparation(controlledListPreparation{prepare: func(context.Context, stateaccessquery.ListOptions) (stateaccessquery.PreparedListRequest, error) {
				return stateaccessquery.PreparedListRequest{}, cause
			}})
			result, err := role.PrepareListRequest(t.Context(), ListOptions{})
			if result != (PreparedListRequest{}) {
				t.Fatalf("error returned result: %#v", result)
			}
			var inner *stateaccessquery.ValidationError
			if errors.As(cause, &inner) {
				var mapped *ValidationError
				if !errors.As(err, &mapped) || mapped.Field != inner.Field || mapped.Message != inner.Message {
					t.Fatalf("mapped error = %v", err)
				}
			} else if !errors.Is(err, cause) {
				t.Fatalf("cause lost: %v", err)
			}
		})
	}
}
