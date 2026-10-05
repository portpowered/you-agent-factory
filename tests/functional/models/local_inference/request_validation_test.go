package local_inference_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Malformed customer model requests must return actionable HTTP errors before
// model execution. One server owns the table; each request owns its input.
func TestModelRESTRejectsMalformedInvocationContent(t *testing.T) {
	t.Parallel()
	dir := support.ScaffoldFactory(t, map[string]any{"name": "model-request-validation"})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{FactoryDir: dir})
	for _, test := range []struct{ name, body, diagnostic string }{
		{"missing body", "", "request body is required"},
		{"missing operation", `{}`, "operation is required"},
		{"content array", `{"operation":"OMNI","content":"text"}`, "content must be an array"},
		{"content object", `{"operation":"OMNI","content":["text"]}`, "content[0] must be an object"},
		{"missing type", `{"operation":"OMNI","content":[{}]}`, "content[0].type is required"},
		{"invalid type", `{"operation":"OMNI","content":[{"type":42}]}`, "content[0].type must be a non-empty string"},
		{"missing text", `{"operation":"OMNI","content":[{"type":"TEXT"}]}`, "content[0].text"},
		{"unknown text field", `{"operation":"OMNI","content":[{"type":"TEXT","text":"hello","typo":true}]}`, "content[0].typo"},
		{"missing audio location", `{"operation":"ASR","content":[{"type":"AUDIO"}]}`, "content[0].url is required"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response, err := http.Post(server.URL()+"/models/LLM/invocations", "application/json", strings.NewReader(test.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			var diagnostic factoryapi.ErrorResponse
			if err := json.NewDecoder(response.Body).Decode(&diagnostic); err != nil {
				t.Fatal(err)
			}
			if response.StatusCode != http.StatusBadRequest || diagnostic.Family != factoryapi.ErrorFamilyBadRequest || !strings.Contains(diagnostic.Message, test.diagnostic) {
				t.Fatalf("HTTP %d: %#v, want BAD_REQUEST naming %q", response.StatusCode, diagnostic, test.diagnostic)
			}
		})
	}
}
