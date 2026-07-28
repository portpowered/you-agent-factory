// Package service is a transitional compile shim that re-exports the composed
// Factory Runtime root from pkg/services/factory_runtime/internal. Peers should
// construct through factory_runtime/wire; baseline deletion of this path is
// owned by DEL-RUN.
package service
