// Package initializer is the canonical composition root for factory config loading
// and domain service construction. It assembles session, factory-definition, model,
// worker, and runtime-host collaborators without constructing root pkg/service
// FactoryService at transport composition boundaries. API, CLI local in-process,
// and MCP serve paths consume initializer-produced transport bundles. Process
// startup graphs are constructed by pkg/wire and handed here only for lifecycle
// execution.
package initializer
