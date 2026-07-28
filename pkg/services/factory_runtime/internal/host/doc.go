// Package host owns inert Factory Runtime instances and their generic
// start, readiness, replacement, stop, and artifact-finalization lifecycle.
//
// Factory Sessions may coordinate these public host values, while runtime
// construction remains owned by factory_runtime/internal.
package host
