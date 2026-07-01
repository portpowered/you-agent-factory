// Package initializer is the canonical composition root for factory config loading
// and domain service construction. It assembles session, factory-definition, model,
// worker, and runtime-host collaborators without constructing root pkg/service
// FactoryService at transport composition boundaries.
package initializer
