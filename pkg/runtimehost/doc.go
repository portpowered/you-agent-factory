// Package runtimehost owns the authoritative factory runtime and session host.
//
// Transports compose a Core through pkg/service.ComposeFactoryCore, wrap it in a
// Host, and attach collaborators through pkg/service wire helpers. Root
// pkg/service.FactoryService is a compatibility alias for runtimehost.Host.
package runtimehost
