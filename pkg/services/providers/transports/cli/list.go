package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/transports/cli/clidiag"
)

// ListConfig holds parameters for the providers list command.
type ListConfig struct {
	Context     context.Context
	JSON        bool
	Verbose     bool
	Output      io.Writer
	Diagnostics io.Writer
}

// List delegates catalog list intent to the Providers-owned CLI adapter Service
// and surfaces typed results and cancellation failures for CLI consumption.
func List(cfg ListConfig, root providers.Service) error {
	adapter := New(root)
	if adapter == nil {
		return fmt.Errorf("providers service is required")
	}
	return adapter.List(cfg)
}

func (service *service) List(cfg ListConfig) error {
	if cfg.Context == nil {
		return fmt.Errorf("context is required")
	}
	if cfg.Output == nil {
		return fmt.Errorf("output writer is required")
	}
	if err := cfg.Context.Err(); err != nil {
		logListFailure(cfg, "cancellation", err)
		return err
	}
	clidiag.Printf(cfg.Diagnostics, cfg.Verbose, "providers list request")
	result, err := service.root.ListProviders(cfg.Context, providers.ListProvidersRequest{})
	if err != nil {
		logListFailure(cfg, "service", err)
		return err
	}
	if err := cfg.Context.Err(); err != nil {
		logListFailure(cfg, "cancellation", err)
		return err
	}

	var rendered bytes.Buffer
	if cfg.JSON {
		err = json.NewEncoder(&rendered).Encode(listResultToJSON(result))
	} else {
		err = renderListResult(cfg.Context, &rendered, result)
	}
	if err != nil {
		logListFailure(cfg, "render", err)
		return err
	}
	if err := cfg.Context.Err(); err != nil {
		logListFailure(cfg, "cancellation", err)
		return err
	}
	if _, err := io.Copy(cfg.Output, &rendered); err != nil {
		logListFailure(cfg, "write", err)
		return err
	}
	if err := cfg.Context.Err(); err != nil {
		logListFailure(cfg, "cancellation", err)
		return err
	}
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"providers list complete count=%d",
		len(result.Providers),
	)
	return nil
}

func logListFailure(cfg ListConfig, stage string, err error) {
	// Keep diagnostics useful without copying provider-native error text, which
	// may contain command lines, paths, credentials, or other sensitive values.
	clidiag.Printf(
		cfg.Diagnostics,
		cfg.Verbose,
		"providers list failed stage=%s errorType=%T",
		stage,
		err,
	)
}
