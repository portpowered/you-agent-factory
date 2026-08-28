package openapitests

import (
	"encoding/base64"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"gopkg.in/yaml.v3"
)

func TestFactoryInvocationExamples_CanonicalRoundTripPreservesLocalizedStructuredValues(t *testing.T) {
	payload := []byte(`{"name":"examples","invocationSignature":{"parameters":[{"name":"input","required":true,"bindings":[{"kind":"NAMED"}]},{"name":"tag","valueMode":"REPEATED","bindings":[{"kind":"NAMED"}]}]},"examples":[{"name":"multiline","description":{"type":"LOCALIZABLE_ASSET","value":"Base explanation","locales":["en-US"],"values":{"fr-FR":"Explication"},"id":"example-multiline"},"args":{"input":"first line\nsecond line","tag":["alpha","beta"]}}]}`)
	cfg, err := FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	wantDescription := interfaces.NameValueConfig{Type: interfaces.NameValueTypeLocalizableAsset, Value: "Base explanation", Locales: []string{"en-US"}, Values: map[string]string{"fr-FR": "Explication"}, ID: "example-multiline"}
	if len(cfg.Examples) != 1 || !reflect.DeepEqual(cfg.Examples[0].Description, wantDescription) {
		t.Fatalf("examples = %#v, want localized example", cfg.Examples)
	}
	canonical, err := MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if strings.Contains(string(canonical), `"argv"`) || strings.Contains(string(canonical), `"invocationSignature":{"examples"`) {
		t.Fatalf("canonical output retained legacy examples: %s", canonical)
	}
	roundTripped, err := FactoryConfigFromOpenAPIJSON(canonical)
	if err != nil || !reflect.DeepEqual(roundTripped.Examples, cfg.Examples) {
		t.Fatalf("round-trip examples = %#v, error = %v", roundTripped.Examples, err)
	}
}

func TestFactoryInvocationExamples_LegacyInputMapsThroughInvocationNormalizer(t *testing.T) {
	payload := []byte(`{"name":"legacy-examples","invocationSignature":{"parameters":[{"name":"input","required":true,"bindings":[{"kind":"POSITIONAL","position":1},{"kind":"STDIN"}]},{"name":"tag","externalName":"tag","valueMode":"REPEATED","bindings":[{"kind":"NAMED"}]}],"examples":[{"name":"legacy","description":"Legacy explanation","argv":["hello","--tag","alpha","--tag=beta"]}]}}`)
	cfg, err := FactoryConfigFromOpenAPIJSON(payload)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if got := cfg.Examples[0].Args["input"]; got != "hello" {
		t.Fatalf("args.input = %#v, want hello", got)
	}
	if got := cfg.Examples[0].Args["tag"]; !reflect.DeepEqual(got, []string{"alpha", "beta"}) {
		t.Fatalf("args.tag = %#v", got)
	}
}

func TestFactoryInvocationExamples_RejectsDualSourcesAndInvalidArgumentShapes(t *testing.T) {
	tests := []struct{ name, payload, want string }{
		{"dual sources", `{"name":"conflict","examples":[],"invocationSignature":{"examples":[]}}`, "examples and invocationSignature.examples cannot both be defined"},
		{"invalid canonical value", `{"name":"invalid","examples":[{"name":"bad","description":{"type":"LOCALIZABLE_ASSET","value":"Bad"},"args":{"count":3}}]}`, "factory.examples[0].args: count must be a string or array of strings"},
		{"invalid legacy value", `{"name":"invalid","invocationSignature":{"parameters":[],"examples":[{"name":"bad","argv":[3]}]}}`, "factory.invocationSignature.examples[0].argv[0] must be a string"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FactoryConfigFromOpenAPIJSON([]byte(test.payload))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestFactoryInvocationExamples_CanonicalWriterRejectsInvalidInternalArgumentShapes(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		key   string
	}{
		{name: "mixed array", value: []interface{}{"alpha", 3}, key: "tag"},
		{name: "unsupported scalar", value: 3, key: "count"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg := &interfaces.FactoryConfig{
				Name: "invalid-writer",
				Examples: []interfaces.InvocationExampleConfig{{
					Name:        "invalid",
					Description: interfaces.NameValueConfig{Type: interfaces.NameValueTypeLocalizableAsset, Value: "Invalid"},
					Args:        interfaces.InvocationExampleArguments{test.key: test.value},
				}},
			}
			want := "factory.examples[0].args." + test.key + " must be a string or array of strings"
			for name, write := range map[string]func() error{
				"OpenAPI mapping": func() error {
					_, err := FactoryConfigToOpenAPI(cfg)
					return err
				},
				"canonical flatten": func() error {
					_, err := MarshalCanonicalFactoryConfig(cfg)
					return err
				},
			} {
				t.Run(name, func(t *testing.T) {
					err := write()
					if err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("writer error = %v, want containing %q", err, want)
					}
				})
			}
		})
	}
}

func TestFactoryInvocationExamples_YAMLRepresentationPreservesStructuredValues(t *testing.T) {
	cfg := interfaces.FactoryConfig{Name: "yaml", Examples: []interfaces.InvocationExampleConfig{{Name: "yaml-example", Description: interfaces.NameValueConfig{Type: interfaces.NameValueTypeLocalizableAsset, Value: "YAML example", ID: "yaml-example-id", Locales: []string{"en-US"}, Values: map[string]string{"es-ES": "Ejemplo YAML"}}, Args: interfaces.InvocationExampleArguments{"input": "line one\nline two", "tag": []string{"one", "two"}}}}}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var decoded interfaces.FactoryConfig
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if !reflect.DeepEqual(decoded.Examples, cfg.Examples) {
		t.Fatalf("decoded examples = %#v, want %#v", decoded.Examples, cfg.Examples)
	}
}

const portableLayoutMaxEmbeddedImageBytes = 2 * 1024 * 1024

var (
	portableLayoutImageDataOnce      sync.Once
	portableLayoutMaximumImageData   string
	portableLayoutOverlargeImageData string
)

type portableLayoutBoundaryCase struct {
	name      string
	payload   []byte
	wantError string
}

func portableLayoutImageFixtures() (maximum, overlarge string) {
	portableLayoutImageDataOnce.Do(func() {
		portableLayoutMaximumImageData = base64.StdEncoding.EncodeToString(make([]byte, portableLayoutMaxEmbeddedImageBytes))
		portableLayoutOverlargeImageData = base64.StdEncoding.EncodeToString(make([]byte, portableLayoutMaxEmbeddedImageBytes+1))
	})
	return portableLayoutMaximumImageData, portableLayoutOverlargeImageData
}

func TestFactoryConfigFromOpenAPIJSON_RejectsUnsafePortableLayoutAnnotationContent(t *testing.T) {
	maximumImageData, overlargeImageData := portableLayoutImageFixtures()

	tests := []portableLayoutBoundaryCase{
		{"overlong note title", layoutFactoryJSON(layoutNoteAnnotation("note", strings.Repeat("t", 161), "safe")), "decode factory generated-schema boundary: layout.annotations[0].note.title must contain no more than 160 characters"},
		{"whitespace-only note body", layoutFactoryJSON(layoutNoteAnnotation("note", "", " \n\t ")), "decode factory generated-schema boundary: layout.annotations[0].note.body must contain at least 1 character"},
		{"overlong note body", layoutFactoryJSON(layoutNoteAnnotation("note", "", strings.Repeat("b", 4001))), "decode factory generated-schema boundary: layout.annotations[0].note.body must contain no more than 4000 characters"},
		{"blank alternative text", layoutFactoryJSON(layoutImageAnnotation("image", "", "EMBEDDED", "image/png", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].image.alternativeText must contain at least 1 character"},
		{"whitespace-only alternative text", layoutFactoryJSON(layoutImageAnnotation("image", "   ", "EMBEDDED", "image/png", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].image.alternativeText must contain at least 1 character"},
		{"blank annotation id", layoutFactoryJSON(layoutImageAnnotation("", "Example", "EMBEDDED", "image/png", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].id must contain a non-whitespace character"},
		{"whitespace-only annotation id", layoutFactoryJSON(layoutImageAnnotation("   ", "Example", "EMBEDDED", "image/png", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].id must contain a non-whitespace character"},
		{"overlong alternative text", layoutFactoryJSON(layoutImageAnnotation("image", strings.Repeat("a", 501), "EMBEDDED", "image/png", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].image.alternativeText must contain no more than 500 characters"},
		{"invalid base64", layoutFactoryJSON(layoutImageAnnotation("image", "Example", "EMBEDDED", "image/png", "AQI")), "decode factory generated-schema boundary: layout.annotations[0].image.source.data must be non-empty strict padded base64"},
		{"unsupported svg media type", layoutFactoryJSON(layoutImageAnnotation("image", "Example", "EMBEDDED", "image/svg+xml", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].image.source.mediaType must be image/png, image/jpeg, or image/webp"},
		{"unsupported remote source", layoutFactoryJSON(layoutImageAnnotation("image", "Example", "REMOTE", "image/png", "AQID")), "decode factory generated-schema boundary: layout.annotations[0].image.source.kind must be EMBEDDED"},
		{"individual image exceeds byte limit", layoutFactoryJSON(layoutImageAnnotation("image", "Example", "EMBEDDED", "image/png", overlargeImageData)), "decode factory generated-schema boundary: layout.annotations[0].image.source.data exceeds the 2097152-byte embedded-image limit"},
		{
			"factory image budget exceeded",
			layoutFactoryJSON(strings.Join([]string{
				layoutImageAnnotation("one", "One", "EMBEDDED", "image/png", maximumImageData),
				layoutImageAnnotation("two", "Two", "EMBEDDED", "image/jpeg", maximumImageData),
				layoutImageAnnotation("three", "Three", "EMBEDDED", "image/webp", maximumImageData),
				layoutImageAnnotation("four", "Four", "EMBEDDED", "image/png", maximumImageData),
				layoutImageAnnotation("five", "Five", "EMBEDDED", "image/png", maximumImageData),
			}, ",")),
			"decode factory generated-schema boundary: layout.annotations[4].image.source.data exceeds the 8388608-byte Factory embedded-image budget",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := FactoryConfigFromOpenAPIJSON(test.payload)
			if err == nil {
				t.Fatal("expected unsafe layout annotation to be rejected")
			}
			if got := err.Error(); got != test.wantError {
				t.Fatalf("error = %q, want %q", got, test.wantError)
			}
		})
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsUnsafePortableLayoutEmptyStates(t *testing.T) {
	maximumImageData, _ := portableLayoutImageFixtures()
	tests := []portableLayoutBoundaryCase{
		{"missing variant", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState must contain exactly one of text or image"},
		{"multiple variants", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":"Nothing here","image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":"Empty"}}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState must contain exactly one of text or image"},
		{"empty text", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":""}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState.text must contain at least 1 character"},
		{"whitespace-only text", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":"   "}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState.text must contain at least 1 character"},
		{"blank canonical node id", layoutFactoryJSONWithNodes(`{"id":"","position":{"x":10,"y":20},"emptyState":{"text":"Nothing here"}}`), "decode factory generated-schema boundary: layout.nodes[0].id must contain a non-whitespace character"},
		{"whitespace-only canonical node id", layoutFactoryJSONWithNodes(`{"id":"   ","position":{"x":10,"y":20},"emptyState":{"text":"Nothing here"}}`), "decode factory generated-schema boundary: layout.nodes[0].id must contain a non-whitespace character"},
		{"overlong text", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":"` + strings.Repeat("x", 501) + `"}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState.text must contain no more than 500 characters"},
		{"empty image alternative text", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":""}}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState.image.alternativeText must contain at least 1 character"},
		{"unsupported image source", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/svg+xml","data":"AQID"},"alternativeText":"Empty"}}}`), "decode factory generated-schema boundary: layout.nodes[0].emptyState.image.source.mediaType must be image/png, image/jpeg, or image/webp"},
		{"duplicate node empty state", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":"First"}},{"id":"workstation:review","position":{"x":20,"y":30},"emptyState":{"text":"Second"}}`), "decode factory generated-schema boundary: layout.nodes[1].emptyState duplicates an empty state for canonical node \"workstation:review\""},
		{"factory image budget includes empty states", layoutFactoryJSONWithNodes(strings.Join([]string{
			`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"` + maximumImageData + `"},"alternativeText":"One"}}}`,
			`{"id":"workstation:approve","position":{"x":20,"y":30},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"` + maximumImageData + `"},"alternativeText":"Two"}}}`,
			`{"id":"workstation:publish","position":{"x":30,"y":40},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"` + maximumImageData + `"},"alternativeText":"Three"}}}`,
			`{"id":"workstation:archive","position":{"x":40,"y":50},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"` + maximumImageData + `"},"alternativeText":"Four"}}}`,
			`{"id":"workstation:notify","position":{"x":50,"y":60},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"` + maximumImageData + `"},"alternativeText":"Five"}}}`,
		}, ",")), "decode factory generated-schema boundary: layout.nodes[4].emptyState.image.source.data exceeds the 8388608-byte Factory embedded-image budget"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := FactoryConfigFromOpenAPIJSON(test.payload)
			if err == nil {
				t.Fatal("expected unsafe layout empty state to be rejected")
			}
			if got := err.Error(); got != test.wantError {
				t.Fatalf("error = %q, want %q", got, test.wantError)
			}
		})
	}
}

func TestProjectedFactorySchema_RejectsWhitespaceOnlyRequiredLayoutValues(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
		path    string
	}{
		{"note body", layoutFactoryJSON(layoutNoteAnnotation("note", "", "   ")), "/layout/annotations/0/note/body"},
		{"image alternative text", layoutFactoryJSON(layoutImageAnnotation("image", "   ", "EMBEDDED", "image/png", "AQID")), "/layout/annotations/0/image/alternativeText"},
		{"annotation id", layoutFactoryJSON(layoutNoteAnnotation("   ", "", "safe")), "/layout/annotations/0/id"},
		{"empty-state text", layoutFactoryJSONWithNodes(`{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":"   "}}`), "/layout/nodes/0/emptyState/text"},
		{"canonical node id", layoutFactoryJSONWithNodes(`{"id":"   ","position":{"x":10,"y":20},"emptyState":{"text":"Nothing here"}}`), "/layout/nodes/0/id"},
	}

	schema := projectedFactorySchema(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document any
			if err := json.Unmarshal(test.payload, &document); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			err := schema.Validate(document)
			if err == nil {
				t.Fatal("projected Factory schema accepted whitespace-only required layout value")
			}
			if !strings.Contains(err.Error(), test.path) {
				t.Fatalf("schema error = %v, want path %q", err, test.path)
			}
		})
	}
}

func TestProjectedFactorySchema_EnforcesLayoutAnnotationVariants(t *testing.T) {
	tests := []struct {
		name       string
		annotation string
		wantValid  bool
	}{
		{"valid note", layoutNoteAnnotation("note", "", "safe"), true},
		{"valid image", layoutImageAnnotation("image", "Example", "EMBEDDED", "image/png", "AQID"), true},
		{"note missing content", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20}}`, false},
		{"image missing content and size", `{"id":"image","kind":"IMAGE","position":{"x":10,"y":20}}`, false},
		{"note carrying image without note", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20},"size":{"width":180,"height":120},"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":"Example"}}`, false},
		{"note carrying null image", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20},"note":{"body":"safe","tone":"INFO"},"image":null}`, false},
		{"image carrying null note", `{"id":"image","kind":"IMAGE","position":{"x":10,"y":20},"size":{"width":180,"height":120},"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":"Example"},"note":null}`, false},
	}

	schema := projectedFactorySchema(t)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document any
			if err := json.Unmarshal(layoutFactoryJSON(test.annotation), &document); err != nil {
				t.Fatalf("decode fixture: %v", err)
			}
			err := schema.Validate(document)
			if test.wantValid && err != nil {
				t.Fatalf("projected Factory schema rejected valid annotation: %v", err)
			}
			if !test.wantValid && err == nil {
				t.Fatal("projected Factory schema accepted malformed annotation variant")
			}
		})
	}
}

func TestFactoryConfigFromOpenAPIJSON_PreservesLiteralLayoutEmptyStateTextAndImage(t *testing.T) {
	nodes := `{"id":"workstation:review","position":{"x":10,"y":20},"emptyState":{"text":"# Nothing\n[not a link](javascript:alert(1))"}},{"id":"workstation:approve","position":{"x":20,"y":30},"emptyState":{"image":{"source":{"kind":"EMBEDDED","mediaType":"image/webp","data":"AQID"},"alternativeText":"<img alt=literal>"}}}`
	cfg, err := FactoryConfigFromOpenAPIJSON(layoutFactoryJSONWithNodes(nodes))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Layout.Nodes[0].EmptyState.Text != "# Nothing\n[not a link](javascript:alert(1))" {
		t.Fatalf("literal empty-state text = %#v", cfg.Layout.Nodes[0].EmptyState)
	}
	if cfg.Layout.Nodes[1].EmptyState.Image.AlternativeText != "<img alt=literal>" || cfg.Layout.Nodes[1].EmptyState.Image.Source.MediaType != "image/webp" {
		t.Fatalf("image empty state = %#v", cfg.Layout.Nodes[1].EmptyState)
	}
}

func TestFactoryConfigFromOpenAPIJSON_PreservesLiteralLayoutAnnotationText(t *testing.T) {
	literalText := "# Guidance\n[not a link](javascript:alert(1))\n<img src=example>"
	annotations := layoutNoteAnnotation("literal-note", "", literalText) + "," +
		layoutImageAnnotation("literal-image", "<img alt=literal>", "EMBEDDED", "image/png", "AQID")

	cfg, err := FactoryConfigFromOpenAPIJSON(layoutFactoryJSON(annotations))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if got := cfg.Layout.Annotations[0].Note.Body; got != literalText {
		t.Fatalf("literal note body = %q, want %q", got, literalText)
	}
	if got := cfg.Layout.Annotations[1].Image.AlternativeText; got != "<img alt=literal>" {
		t.Fatalf("literal alternative text = %q", got)
	}
}

func TestFactoryConfigFromOpenAPIJSON_RejectsInvalidLayoutAnnotationGeometryAndTopologyFields(t *testing.T) {
	tests := []struct {
		name        string
		annotations string
		wantPath    string
	}{
		{"missing position", `{"id":"note","kind":"NOTE","note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].position"},
		{"position exceeds lower bound", `{"id":"note","kind":"NOTE","position":{"x":-100001,"y":20},"note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].position.x"},
		{"position exceeds upper bound", `{"id":"note","kind":"NOTE","position":{"x":10,"y":100001},"note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].position.y"},
		{"zero note width", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20},"size":{"width":0,"height":20},"note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].size.width"},
		{"oversized note height", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20},"size":{"width":20,"height":10001},"note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].size.height"},
		{"image requires size", `{"id":"image","kind":"IMAGE","position":{"x":10,"y":20},"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":"Example"}}`, "layout.annotations[0].size"},
		{"connection field", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20},"source":"workstation:review","note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].source"},
		{"wrong content for kind", `{"id":"note","kind":"NOTE","position":{"x":10,"y":20},"image":{"source":{"kind":"EMBEDDED","mediaType":"image/png","data":"AQID"},"alternativeText":"Example"},"note":{"body":"safe","tone":"INFO"}}`, "layout.annotations[0].image"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := FactoryConfigFromOpenAPIJSON(layoutFactoryJSON(test.annotations))
			if err == nil {
				t.Fatal("expected invalid layout annotation to be rejected")
			}
			if !strings.Contains(err.Error(), test.wantPath) {
				t.Fatalf("error = %v, want path %q", err, test.wantPath)
			}
		})
	}
}

func layoutFactoryJSON(annotations string) []byte {
	return []byte(`{
		"name":"layout-factory",
		"layout":{"schemaVersion":1,"annotations":[` + annotations + `],"viewport":{"x":0,"y":0,"zoom":1}},
		"workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"writer","type":"MODEL_WORKER"}],
		"workstations":[{"name":"review","worker":"writer","inputs":[{"workType":"task","state":"ready"}],"outputs":[{"workType":"task","state":"done"}]}]
	}`)
}

func layoutFactoryJSONWithNodes(nodes string) []byte {
	return []byte(`{
		"name":"layout-factory",
		"layout":{"schemaVersion":1,"nodes":[` + nodes + `],"viewport":{"x":0,"y":0,"zoom":1}},
		"workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],
		"workers":[{"name":"writer","type":"MODEL_WORKER"}],
		"workstations":[{"name":"review","worker":"writer","inputs":[{"workType":"task","state":"ready"}],"outputs":[{"workType":"task","state":"done"}]}]
	}`)
}

func layoutNoteAnnotation(id, title, body string) string {
	return `{"id":"` + id + `","kind":"NOTE","position":{"x":10,"y":20},"note":{"title":` + marshalLayoutAnnotationString(title) + `,"body":` + marshalLayoutAnnotationString(body) + `,"tone":"INFO"}}`
}

func layoutImageAnnotation(id, alternativeText, kind, mediaType, data string) string {
	return `{"id":"` + id + `","kind":"IMAGE","position":{"x":10,"y":20},"size":{"width":180,"height":120},"image":{"source":{"kind":` + marshalLayoutAnnotationString(kind) + `,"mediaType":` + marshalLayoutAnnotationString(mediaType) + `,"data":` + marshalLayoutAnnotationString(data) + `},"alternativeText":` + marshalLayoutAnnotationString(alternativeText) + `}}`
}

func marshalLayoutAnnotationString(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsMisCasedEnumValuesAtBoundary(t *testing.T) {
	for _, tc := range generatedFactoryMisCasedEnumTestCases() {
		t.Run(tc.name, func(t *testing.T) {
			assertGeneratedFactoryRejectsMisCasedEnumValue(t, tc.fieldPath, tc.value, tc.payload)
		})
	}
}

type generatedFactoryMisCasedEnumTestCase struct {
	name      string
	fieldPath string
	value     string
	payload   string
}

func assertGeneratedFactoryRejectsMisCasedEnumValue(t *testing.T, fieldPath, value, payload string) {
	t.Helper()

	_, err := GeneratedFactoryFromOpenAPIJSON([]byte(payload))
	if err == nil {
		t.Fatal("expected mis-cased enum value to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), fieldPath) {
		t.Fatalf("expected field path %q in error, got %v", fieldPath, err)
	}
	if !strings.Contains(err.Error(), `unsupported value "`+value+`"`) {
		t.Fatalf("expected unsupported value %q in error, got %v", value, err)
	}
}

func generatedFactoryMisCasedEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	cases := generatedFactoryMisCasedWorkTypeEnumTestCases()
	cases = append(cases, generatedFactoryMisCasedWorkerEnumTestCases()...)
	cases = append(cases, generatedFactoryMisCasedWorkstationEnumTestCases()...)
	cases = append(cases, generatedFactoryMisCasedOrchestratorEnumTestCases()...)
	cases = append(cases, generatedFactoryMisCasedResourceEnumTestCases()...)
	return cases
}

func generatedFactoryMisCasedWorkTypeEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "work type handling behavior",
			fieldPath: "workTypes[0].handlingBehavior[0]",
			value:     "default",
			payload: `{
				"name":"work-type-handling-factory",
				"workTypes": [{
					"name":"story",
					"handlingBehavior":["default"],
					"states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]
				}],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func generatedFactoryMisCasedWorkerEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "worker type",
			fieldPath: "workers[0].type",
			value:     "model_worker",
			payload: `{
				"name":"worker-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"model_worker"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker model provider",
			fieldPath: "workers[0].modelProvider",
			value:     "Claude",
			payload: `{
				"name":"worker-model-provider-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","modelProvider":"Claude"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker model locality",
			fieldPath: "workers[0].modelLocality",
			value:     "local",
			payload: `{
				"name":"worker-model-locality-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","modelLocality":"local"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker operation name",
			fieldPath: "workers[0].operations[0].name",
			value:     "tts",
			payload: `{
				"name":"worker-operation-name-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","operations":[{"name":"tts","outputs":[{"name":"audio","contentTypes":["AUDIO"]}]}]}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "worker operation content type",
			fieldPath: "workers[0].operations[0].outputs[0].contentTypes[0]",
			value:     "audio",
			payload: `{
				"name":"worker-operation-content-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER","operations":[{"name":"TTS","outputs":[{"name":"audio","contentTypes":["audio"]}]}]}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func generatedFactoryMisCasedOrchestratorEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "orchestrator kind",
			fieldPath: "orchestrator.kind",
			value:     "javascript",
			payload: `{
				"name":"miscased-orchestrator-factory",
				"orchestrator":{
					"kind":"javascript",
					"javascript":{"sourceRef":"factory/workflows/review.js"}
				}
			}`,
		},
	}
}

func generatedFactoryMisCasedResourceEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "resource type",
			fieldPath: "resources[0].type",
			value:     "model",
			payload: `{
				"name":"miscased-resource-factory",
				"workTypes":[{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"resources":[{"name":"slot","type":"model","capacity":1}],
				"workers":[{"name":"executor"}],
				"workstations":[{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func generatedFactoryMisCasedWorkstationEnumTestCases() []generatedFactoryMisCasedEnumTestCase {
	return []generatedFactoryMisCasedEnumTestCase{
		{
			name:      "workstation behavior",
			fieldPath: "workstations[0].behavior",
			value:     "cron",
			payload: `{
				"name":"workstation-behavior-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"behavior":"cron",
					"type":"MODEL_WORKSTATION",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation type",
			fieldPath: "workstations[0].type",
			value:     "logical_move",
			payload: `{
				"name":"workstation-type-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"logical_move",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation operation",
			fieldPath: "workstations[0].operation",
			value:     "tts",
			payload: `{
				"name":"workstation-operation-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"type":"MODEL_INVOKE",
					"operation":"tts",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
		{
			name:      "workstation outcome format",
			fieldPath: "workstations[0].outcomeFormat",
			value:     "Decision-Envelope",
			payload: `{
				"name":"workstation-outcome-format-factory",
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"workers": [{"name":"executor","type":"MODEL_WORKER"}],
				"workstations": [{
					"name":"review-story",
					"worker":"executor",
					"type":"MODEL_WORKSTATION",
					"outcomeFormat":"Decision-Envelope",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`,
		},
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_ParsesCanonicalUppercaseSharedEnumsAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"uppercase-enums-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"modelProvider":"CODEX",
			"modelLocality":"LOCAL",
			"operations":[{
				"name":"TTS",
				"inputs":[{"name":"text","contentTypes":["TEXT"],"required":true}],
				"outputs":[{"name":"audio","contentTypes":["AUDIO"]}]
			}],
			"executorProvider":"SCRIPT_WRAP"
		}],
		"workstations": [{
			"name":"execute-story",
			"behavior":"STANDARD",
			"worker":"executor",
			"type":"MODEL_INVOKE",
			"operation":"TTS",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	generated, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON: %v", err)
	}
	assertGeneratedCanonicalUppercaseWorkerEnums(t, generated)

	cfg, err := FactoryConfigFromOpenAPI(generated)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPI: %v", err)
	}
	assertRuntimeCanonicalUppercaseWorkerEnums(t, cfg)
}

func assertGeneratedCanonicalUppercaseWorkerEnums(t *testing.T, generated factoryapi.Factory) {
	t.Helper()
	if generated.Workers == nil || len(*generated.Workers) != 1 {
		t.Fatalf("expected one generated worker, got %#v", generated.Workers)
	}
	worker := (*generated.Workers)[0]
	if worker.ModelProvider == nil || *worker.ModelProvider != factoryapi.WorkerModelProviderCodex {
		t.Fatalf("expected generated worker modelProvider CODEX, got %#v", worker.ModelProvider)
	}
	if worker.ModelLocality == nil || *worker.ModelLocality != factoryapi.WorkerModelLocalityLocal {
		t.Fatalf("expected generated worker modelLocality LOCAL, got %#v", worker.ModelLocality)
	}
	if worker.ExecutorProvider == nil || *worker.ExecutorProvider != factoryapi.WorkerProvider("SCRIPT_WRAP") {
		t.Fatalf("expected generated worker executorProvider SCRIPT_WRAP, got %#v", worker.ExecutorProvider)
	}
	if worker.Operations == nil || len(*worker.Operations) != 1 {
		t.Fatalf("expected one generated worker operation, got %#v", worker.Operations)
	}
}

func assertRuntimeCanonicalUppercaseWorkerEnums(t *testing.T, cfg interfaces.FactoryConfig) {
	t.Helper()
	if got := cfg.Workers[0].ModelProvider; got != "codex" {
		t.Fatalf("expected runtime worker modelProvider codex, got %q", got)
	}
	if got := cfg.Workers[0].ModelLocality; got != interfaces.ModelLocalityLocal {
		t.Fatalf("expected runtime worker modelLocality LOCAL, got %q", got)
	}
	if got := cfg.Workers[0].ExecutorProvider; got != "script_wrap" {
		t.Fatalf("expected runtime worker executorProvider script_wrap, got %q", got)
	}
	if len(cfg.Workers[0].Operations) != 1 || cfg.Workers[0].Operations[0].Name != "TTS" {
		t.Fatalf("expected runtime worker TTS operation, got %#v", cfg.Workers[0].Operations)
	}
	if got := cfg.Workstations[0].Type; got != interfaces.WorkstationTypeInvoke {
		t.Fatalf("expected runtime workstation type MODEL_INVOKE, got %q", got)
	}
	if got := cfg.Workstations[0].Operation; got != "TTS" {
		t.Fatalf("expected runtime workstation operation TTS, got %q", got)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_RejectsMalformedExecutorProviderAtBoundary(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"unsupported-executor-provider-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{
			"name":"executor",
			"type":"MODEL_WORKER",
			"executorProvider":"CUSTOM_EXECUTOR"
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	_, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err == nil {
		t.Fatal("expected malformed executorProvider to fail at generated boundary")
	}
	if !strings.Contains(err.Error(), generatedFactoryBoundaryErrorPrefix) {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].executorProvider") {
		t.Fatalf("expected executorProvider field path in error, got %v", err)
	}
	if !strings.Contains(err.Error(), `unsupported value "CUSTOM_EXECUTOR"`) {
		t.Fatalf("expected malformed executorProvider value in error, got %v", err)
	}
}

func TestGeneratedFactoryFromOpenAPIJSON_AcceptsACPExecutorProviderIdentity(t *testing.T) {
	cfgJSON := []byte(`{
		"name":"acp-executor-provider-factory",
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"executor","type":"MODEL_WORKER","executorProvider":"cursor-acp"}],
		"workstations": [{
			"name":"execute-story","worker":"executor","type":"MODEL_WORKSTATION",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)

	cfg, err := GeneratedFactoryFromOpenAPIJSON(cfgJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON() error = %v", err)
	}
	if cfg.Workers == nil || len(*cfg.Workers) != 1 || (*cfg.Workers)[0].ExecutorProvider == nil || *(*cfg.Workers)[0].ExecutorProvider != "cursor-acp" {
		t.Fatalf("generated executorProvider = %#v, want cursor-acp", cfg.Workers)
	}
}
