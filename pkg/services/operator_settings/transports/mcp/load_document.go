package operatorsettingsmcp

import (
	"context"
	"errors"
	"io/fs"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

// LoadDocumentInput is the MCP request shape for you.operator_settings.load_document.
type LoadDocumentInput struct {
	Path            string `json:"path"`
	RequireExisting bool   `json:"requireExisting"`
}

// LoadDocument returns detached operator document facts through the
// you.operator_settings.load_document MCP tool.
func LoadDocument(
	ctx context.Context,
	service operatorsettings.Service,
	input LoadDocumentInput,
) ToolResponse[operatorsettings.LoadDocumentResult] {
	if ctx == nil {
		envelope := executionErrorEnvelope(errMissingRequestContext)
		return ToolResponse[operatorsettings.LoadDocumentResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[operatorsettings.LoadDocumentResult](ctx); done {
		return response
	}
	if service == nil {
		envelope := unavailableServiceErrorEnvelope()
		return ToolResponse[operatorsettings.LoadDocumentResult]{Error: &envelope}
	}

	result, err := service.LoadDocument(operatorsettings.LoadDocumentRequest{
		Path:            input.Path,
		RequireExisting: input.RequireExisting,
	})
	if err != nil {
		envelope := loadDocumentErrorEnvelope(input.Path, err)
		return ToolResponse[operatorsettings.LoadDocumentResult]{Error: &envelope}
	}
	return ToolResponse[operatorsettings.LoadDocumentResult]{Result: &result}
}

// ReadCurrentConfig returns the exact operator-owned JSON document used by
// subagent defaults. An absent file is the valid empty configuration.
func ReadCurrentConfig(ctx context.Context, configPath func(string) string, files operatorsettings.FileSystem, homeDir string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path := configPath(homeDir)
	data, err := files.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []byte("{}"), nil
	}
	return data, err
}
