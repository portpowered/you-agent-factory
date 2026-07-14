//go:build interfaces
// +build interfaces

// Package http owns generation entrypoints for the HTTP transport contract and
// client. Generated output is kept in dedicated child packages.
package http

//go:generate go tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=../../../api/codegen_config/server.yaml ../../../api/openapi.yaml
//go:generate go tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen --config=../../../api/codegen_config/client.yaml ../../../api/openapi.yaml
