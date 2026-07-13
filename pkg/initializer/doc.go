// Package initializer owns startup and shutdown for already-constructed
// application graphs. Legacy transport constructors remain during migration,
// but the canonical root path consumes graph-owned services and lifecycles
// without reconstructing them.
package initializer
