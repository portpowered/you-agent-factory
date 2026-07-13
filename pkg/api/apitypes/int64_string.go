// Package apitypes preserves historical API-specific type imports.
//
// Deprecated: use
// github.com/portpowered/infinite-you/pkg/transports/http/apitypes. This
// forwarding package is scheduled for removal by Batch 008.
package apitypes

import transporttypes "github.com/portpowered/infinite-you/pkg/transports/http/apitypes"

// Int64String is the canonical HTTP transport representation of a JSON int64
// encoded as a decimal string.
//
// Deprecated: use transport/http/apitypes.Int64String.
type Int64String = transporttypes.Int64String
