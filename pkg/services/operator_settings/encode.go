package operatorsettings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

const (
	backendScopeIDField = "backendScopeID"
	defaultsField       = "defaults"
	providerField       = "workerModelProvider"
	modelField          = "workerModel"
)

// ConfigDocument is one complete, validated operator configuration. It keeps
// the encoded fields private so callers can only change it through semantic
// operator-settings operations.
type ConfigDocument struct {
	config FileConfig
	fields map[string]json.RawMessage
}

// ProviderModelUpdate distinguishes omitted defaults from explicitly supplied
// values. A nil field preserves the current value; a non-nil field replaces it
// after trimming, including clearing it when the supplied value is empty.
type ProviderModelUpdate struct {
	Provider *string
	Model    *string
}

// ConfigDocumentService owns complete operator-config loading, semantic merge,
// and encoding. Files is required only by Load; pure operations remain usable
// without a filesystem dependency.
type ConfigDocumentService struct {
	Files FileSystem
}

// MarshalInputInventoryJSON renders the operator config input inventory as stable JSON.
func MarshalInputInventoryJSON(inventory InputInventory) ([]byte, error) {
	payload, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal operator config input inventory: %w", err)
	}

	var buffer bytes.Buffer
	buffer.Write(payload)
	buffer.WriteByte('\n')
	return buffer.Bytes(), nil
}

// Load reads and validates a complete operator configuration. A
// missing destination is represented by an empty, valid document.
func (service ConfigDocumentService) Load(path string) (ConfigDocument, error) {
	if service.Files == nil {
		return ConfigDocument{}, fmt.Errorf("operator config filesystem is required")
	}
	data, err := service.Files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return emptyConfigDocument(), nil
		}
		return ConfigDocument{}, fmt.Errorf("read operator config %s: %w", path, err)
	}
	document, err := service.Parse(data)
	if err != nil {
		return ConfigDocument{}, fmt.Errorf("parse operator config %s: %w", path, err)
	}
	return document, nil
}

// Parse validates bytes against the canonical operator-config
// contract while retaining every accepted field for later semantic encoding.
func (ConfigDocumentService) Parse(data []byte) (ConfigDocument, error) {
	config, err := ParseFileConfig(data)
	if err != nil {
		return ConfigDocument{}, err
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(data, &fields); err != nil {
		return ConfigDocument{}, fmt.Errorf("decode operator config fields: %w", err)
	}
	if fields == nil {
		return ConfigDocument{}, fmt.Errorf("decode operator config fields: expected a JSON object")
	}
	return ConfigDocument{config: config, fields: cloneRawFields(fields)}, nil
}

// FileConfig returns the validated semantic view of the document.
func (document ConfigDocument) FileConfig() FileConfig {
	config := document.config
	if document.config.WorkerPresets != nil {
		config.WorkerPresets = append([]WorkerPreset{}, document.config.WorkerPresets...)
	}
	return config
}

// BackendScopeID returns the operator identity stored beside the defaults.
func (document ConfigDocument) BackendScopeID() string {
	var value string
	if err := json.Unmarshal(document.fields[backendScopeIDField], &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

// MergeProviderModelDefaults returns a new validated document with only the
// explicitly supplied provider/model defaults changed.
func (service ConfigDocumentService) MergeProviderModelDefaults(
	document ConfigDocument,
	update ProviderModelUpdate,
) (ConfigDocument, error) {
	fields := cloneRawFields(document.fields)
	if fields == nil {
		fields = make(map[string]json.RawMessage)
	}
	if update.Provider != nil || update.Model != nil {
		defaults, err := decodeDefaultsFields(fields[defaultsField])
		if err != nil {
			return ConfigDocument{}, err
		}
		setOptionalString(defaults, providerField, update.Provider)
		setOptionalString(defaults, modelField, update.Model)
		encoded, err := json.Marshal(defaults)
		if err != nil {
			return ConfigDocument{}, fmt.Errorf("encode operator defaults: %w", err)
		}
		fields[defaultsField] = encoded
	}
	return service.validateFields(fields)
}

// Marshal encodes the complete validated document as JSON.
func (service ConfigDocumentService) Marshal(document ConfigDocument) ([]byte, error) {
	validated, err := service.validateFields(document.fields)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(validated.fields, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode operator config: %w", err)
	}
	return append(data, '\n'), nil
}

func emptyConfigDocument() ConfigDocument {
	return ConfigDocument{fields: make(map[string]json.RawMessage)}
}

func (service ConfigDocumentService) validateFields(fields map[string]json.RawMessage) (ConfigDocument, error) {
	data, err := json.Marshal(fields)
	if err != nil {
		return ConfigDocument{}, fmt.Errorf("encode operator config candidate: %w", err)
	}
	return service.Parse(data)
}

func decodeDefaultsFields(data json.RawMessage) (map[string]json.RawMessage, error) {
	defaults := make(map[string]json.RawMessage)
	if len(data) == 0 {
		return defaults, nil
	}
	if err := json.Unmarshal(data, &defaults); err != nil {
		return nil, fmt.Errorf("decode operator defaults: %w", err)
	}
	return defaults, nil
}

func setOptionalString(fields map[string]json.RawMessage, name string, value *string) {
	if value == nil {
		return
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		delete(fields, name)
		return
	}
	fields[name] = json.RawMessage(strconv.Quote(trimmed))
}

func cloneRawFields(fields map[string]json.RawMessage) map[string]json.RawMessage {
	if fields == nil {
		return nil
	}
	cloned := make(map[string]json.RawMessage, len(fields))
	for name, value := range fields {
		cloned[name] = append(json.RawMessage(nil), value...)
	}
	return cloned
}
