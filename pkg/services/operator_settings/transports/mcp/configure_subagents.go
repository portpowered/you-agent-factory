package operatorsettingsmcp

import (
	"context"
	"encoding/json"
	"fmt"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

const (
	ToolSetSubagentDefaults = "you.operator_settings.set_subagent_defaults"
	ToolAddACPProvider      = "you.operator_settings.add_acp_provider"
)

// ConfigureSubagentsTool applies default provider/model values or registers a
// custom stdio ACP provider through the Operator Settings root.
func ConfigureSubagentsTool(ctx context.Context, service operatorsettings.Service, home string, name string, raw json.RawMessage, generateID operatorsettings.IDGenerator) (json.RawMessage, error) {
	if ctx == nil || service == nil {
		return nil, fmt.Errorf("operator settings context and service are required")
	}
	path := service.DefaultConfigPath(home)
	switch name {
	case ToolSetSubagentDefaults:
		return setSubagentDefaults(ctx, service, path, raw)
	case ToolAddACPProvider:
		return addACPProvider(ctx, service, path, raw, generateID)
	default:
		return nil, fmt.Errorf("unsupported subagent configuration tool %q", name)
	}
}

func setSubagentDefaults(ctx context.Context, service operatorsettings.Service, path string, raw json.RawMessage) (json.RawMessage, error) {
	var input struct {
		Provider *string `json:"provider"`
		Model    *string `json:"model"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Provider == nil && input.Model == nil {
		return nil, fmt.Errorf("provider or model is required")
	}
	loaded, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{Path: path})
	if err != nil {
		return nil, err
	}
	updated, err := service.ApplyDocumentUpdate(operatorsettings.ApplyDocumentUpdateRequest{
		Path: path, ExpectedBackendScope: loaded.Document.BackendScopeID,
		ProviderModel: operatorsettings.DocumentProviderModelUpdate{Provider: input.Provider, Model: input.Model},
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"result": map[string]any{"path": updated.Path, "document": updated.Document, "persisted": updated.Persisted}})
}

func addACPProvider(ctx context.Context, service operatorsettings.Service, path string, raw json.RawMessage, generateID operatorsettings.IDGenerator) (json.RawMessage, error) {
	var input struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Command   string `json:"command"`
		Transport string `json:"transport"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return nil, err
	}
	if input.Transport == "" {
		input.Transport = "stdio"
	}
	if input.ID == "" {
		if generateID == nil {
			return nil, fmt.Errorf("ACP provider ID generator is required")
		}
		input.ID = generateID()
	}
	document, err := service.ConfigureACPIntegrationAdd(ctx, path, operatorsettings.ACPIntegration{ID: input.ID, Name: input.Name, Command: input.Command, Transport: input.Transport})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"result": map[string]any{"path": path, "document": document, "persisted": true, "requiresRestart": true}})
}
