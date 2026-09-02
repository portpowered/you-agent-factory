package acp_test

import "embed"

// Golden RPC payloads drive the customer-facing ACP command scenarios.
//
//go:embed testdata/json_golden/upstream/*.json
var acpGoldenFiles embed.FS
