// Package runtimehost is a migration-era facade around factory runtime hosting.
//
// Factory Session ownership lives in pkg/factorysessions and related session
// services. New application composition should be injected through
// pkg/initializer and, as import-cycle migration allows, pkg/inject rather than
// adding new ownership to this facade. Root pkg/service.FactoryService remains
// a compatibility alias for runtimehost.Host while callers migrate.
package runtimehost
