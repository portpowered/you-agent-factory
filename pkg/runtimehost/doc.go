// Package runtimehost owns the authoritative factory runtime and session host.
//
// Transports compose a Core through pkg/initializer (via pkg/composebridge) and
// wrap it in a Host with collaborators attached through runtimehost helpers.
// Root pkg/service.FactoryService is a compatibility alias for runtimehost.Host.
package runtimehost
