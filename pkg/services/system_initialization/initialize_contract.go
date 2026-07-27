package systeminitialization

import "errors"

// ErrMissingHomeDir reports that Initialize request is missing a non-blank
// home-directory target. Peers branch on this sentinel with errors.Is without
// importing Operator Settings, Factory Definitions, or initializer lifecycle
// types.
var ErrMissingHomeDir = errors.New("system bootstrap home directory is required")

// ErrInitializeCancelled reports that Initialize was cancelled through context
// cancellation before bootstrap work completed. Peers branch on this sentinel
// with errors.Is without importing implementation packages.
var ErrInitializeCancelled = errors.New("system bootstrap initialize cancelled")
