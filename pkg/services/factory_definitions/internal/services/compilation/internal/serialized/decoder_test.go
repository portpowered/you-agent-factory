package serialized_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/internal/serialized"
)

func conversionBytes(t testing.TB, payload []byte) []byte {
	t.Helper()
	hash := sha256.Sum256(payload)
	data, err := json.Marshal(factorydefinitions.SerializedFactoryConfig{
		Version:       factorydefinitions.SerializedFactoryConfigVersion,
		PayloadSHA256: hex.EncodeToString(hash[:]),
		Config:        json.RawMessage(`{"name":"example","work_types":[{"name":"task","states":[{"name":"init","type":"INITIAL"}]}]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecoderReturnsIndependentDefinitionsWithoutCallingFallback(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"published"}`)
	source := fstest.MapFS{factorydefinitions.SerializedFactoryConfigInput(payload).Path(): &fstest.MapFile{Data: conversionBytes(t, payload)}}
	decoder := serialized.NewDecoder(func(input []byte) ([]byte, error) {
		return fs.ReadFile(source, factorydefinitions.SerializedFactoryConfigInput(input).Path())
	}, func([]byte) (*factorydefinitions.FactoryConfig, error) {
		t.Fatal("published conversion called fallback")
		return nil, nil
	})
	first, err := decoder.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	first.WorkTypes[0].Name = "changed"
	second, err := decoder.Decode(payload)
	if err != nil {
		t.Fatal(err)
	}
	if second.Name != "example" || second.WorkTypes[0].Name != "task" {
		t.Fatalf("second definition = %#v, want untouched published value", second)
	}
}

func TestDecoderPreservesFallbackForMissingStaleOrInvalidConversions(t *testing.T) {
	t.Parallel()
	payload := []byte(`{"name":"published"}`)
	for _, testCase := range []struct {
		name string
		data []byte
	}{
		{name: "missing"},
		{name: "malformed", data: []byte(`{`)},
		{name: "version", data: []byte(`{"version":"old"}`)},
		{name: "other-input", data: conversionBytes(t, []byte("different"))},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			source := fstest.MapFS{}
			if testCase.data != nil {
				source[factorydefinitions.SerializedFactoryConfigInput(payload).Path()] = &fstest.MapFile{Data: testCase.data}
			}
			wantErr := errors.New("canonical mapping diagnostic")
			calls := 0
			decoder := serialized.NewDecoder(func(input []byte) ([]byte, error) {
				return fs.ReadFile(source, factorydefinitions.SerializedFactoryConfigInput(input).Path())
			}, func(actual []byte) (*factorydefinitions.FactoryConfig, error) {
				calls++
				if string(actual) != string(payload) {
					t.Fatalf("fallback input = %q, want %q", actual, payload)
				}
				return nil, wantErr
			})
			if _, err := decoder.Decode(payload); !errors.Is(err, wantErr) || calls != 1 {
				t.Fatalf("fallback calls=%d error=%v, want one call and original error", calls, err)
			}
		})
	}
}
