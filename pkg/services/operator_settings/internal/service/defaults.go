package service

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func loadFileConfigWithDiagnostics(
	files operatorsettings.FileSystem,
	decode operatorsettings.ConfigDecoder,
	path string,
	diagnosticDecoder operatorsettings.ConfigDiagnosticsDecoder,
) (operatorsettings.Config, []string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return operatorsettings.Config{}, nil, fmt.Errorf("operator config path is required")
	}
	data, err := files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return operatorsettings.Config{PriceTable: operatorsettings.PriceTable{Currency: operatorsettings.PriceTableCurrencyUSD, Models: []operatorsettings.PriceTableModel{}}, Runtime: defaultRuntimeSettings()}, nil, nil
		}
		return operatorsettings.Config{}, nil, fmt.Errorf("read operator config %s: %w", path, err)
	}
	if decode == nil && diagnosticDecoder == nil {
		return operatorsettings.Config{}, nil, fmt.Errorf("parse operator config %s: global config decoder is required", path)
	}
	var config operatorsettings.Config
	var ignoredJSONPaths []string
	if diagnosticDecoder != nil {
		var diagnostics operatorsettings.ConfigDecodeDiagnostics
		config, diagnostics, err = diagnosticDecoder(data)
		ignoredJSONPaths = diagnostics.Paths()
	} else {
		config, err = decode(data)
	}
	if err != nil {
		return operatorsettings.Config{}, nil, fmt.Errorf("parse operator config %s: %w", path, err)
	}
	return config, ignoredJSONPaths, nil
}

func defaultRuntimeSettings() operatorsettings.RuntimeSettings {
	defaults := operatorsettings.RuntimeArtifactSettings{
		MaxSizeMB:  operatorsettings.DefaultRuntimeArtifactMaxSizeMB,
		MaxBackups: operatorsettings.DefaultRuntimeArtifactBackups,
		MaxAgeDays: operatorsettings.DefaultRuntimeArtifactMaxAge,
	}
	return operatorsettings.RuntimeSettings{Logging: defaults, Metrics: defaults}
}

func deriveProviderBackendScopeID(provider, kind, boundary string) string {
	return fmt.Sprintf(
		"provider-%s-%s-%s",
		sanitizeBackendScopeSegment(provider),
		sanitizeBackendScopeSegment(kind),
		sanitizeBackendScopeSegment(boundary),
	)
}

func sanitizeBackendScopeSegment(value string) string {
	trimmed := strings.ToLower(strings.TrimSpace(value))
	if trimmed == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer(" ", "-", "/", "-", "\\", "-", ":", "-", "|", "-")
	return replacer.Replace(trimmed)
}
