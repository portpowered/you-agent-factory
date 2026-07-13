// Package wire owns construction of the process-scoped application graph.
//
// Build constructs domain services and inert lifecycle collaborators in
// dependency order. Construction resources are retained by the returned Graph
// and unwound on failure; Build never starts a component. pkg/root and
// pkg/initializer own mode selection and lifecycle activation respectively.
package wire
