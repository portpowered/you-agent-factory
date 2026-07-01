// Package service provides factory runtime coordination, session hosting, and
// domain service implementations used by pkg/initializer and legacy callers.
//
// Transport composition (API startup, CLI local in-process startup, and MCP or
// tool-facing entrypoints) must use pkg/initializer as the canonical
// composition root. Root FactoryService and BuildFactoryService remain a
// temporary compatibility shell for in-process runtime hosting and wire
// equivalence tests; they are not the primary application shell for new
// transport wiring. Delete this compatibility layer once SessionRuntimeHost
// and domain services fully replace the monolithic facade.
package service
