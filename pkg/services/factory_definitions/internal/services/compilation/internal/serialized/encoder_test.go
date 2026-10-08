package serialized_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/serialized"
)

func TestEncoderReturnsDetachedCanonicalBytesForExactInput(t *testing.T) {
	t.Parallel()
	config := &factorydefinitions.FactoryConfig{Name: "published"}
	wantInput, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	cached := []byte(`{"name":"published"}`)
	encoder := serialized.NewEncoder(func(input []byte) ([]byte, error) {
		if !bytes.Equal(input, wantInput) {
			return nil, fs.ErrNotExist
		}
		return cached, nil
	}, func(actual *factorydefinitions.FactoryConfig) ([]byte, error) {
		return []byte(actual.Name), nil
	})
	first, err := encoder.Encode(config)
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'x'
	second, err := encoder.Encode(config)
	if err != nil || !bytes.Equal(second, cached) || cached[0] != '{' {
		t.Fatalf("shared output: %q %v", second, err)
	}
	config.Name = "edited"
	edited, err := encoder.Encode(config)
	if err != nil || string(edited) != "edited" {
		t.Fatalf("edited input did not use canonical fallback: %q %v", edited, err)
	}
}

func TestEncoderPreservesOriginalFallbackDiagnostics(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"nil", "missing", "malformed", "null", "unknown-fields", "file-prompt", "worker-session"} {
		t.Run(name, func(t *testing.T) {
			config := &factorydefinitions.FactoryConfig{Name: "published"}
			if name == "nil" {
				config = nil
			}
			if name == "unknown-fields" {
				config.SetIgnoredJSONPaths([]string{"$.extra"})
			}
			if name == "file-prompt" {
				config.Workstations = append(config.Workstations, factorydefinitions.FactoryWorkstationConfig{PromptSourcePath: "prompt.md"})
			}
			if name == "worker-session" {
				config.Workers = append(config.Workers, factorydefinitions.FactoryWorkerConfig{SessionID: "owned-session"})
			}
			wantErr := errors.New("original mapping diagnostic")
			calls := 0
			encoder := serialized.NewEncoder(func([]byte) ([]byte, error) {
				if name == "nil" || name == "unknown-fields" || name == "file-prompt" || name == "worker-session" {
					t.Fatal("nonserializable input reached cache")
				}
				if name == "missing" {
					return nil, fs.ErrNotExist
				}
				if name == "null" {
					return []byte("null"), nil
				}
				return []byte("{"), nil
			}, func(actual *factorydefinitions.FactoryConfig) ([]byte, error) {
				calls++
				if actual != config {
					t.Fatal("fallback input changed")
				}
				return nil, wantErr
			})
			if _, err := encoder.Encode(config); !errors.Is(err, wantErr) || calls != 1 {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}
