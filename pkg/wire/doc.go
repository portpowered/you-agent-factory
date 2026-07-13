// Package wire owns construction of the process-scoped application graph.
//
// Build assembles already-constructed domain services and inert lifecycle
// collaborators. It does not select a process mode or start any component;
// pkg/root and pkg/initializer own those responsibilities respectively.
package wire
