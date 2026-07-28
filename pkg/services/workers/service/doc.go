// Package service is a transitional compile shim that re-exports the composed
// Workers runtime construction implementation from pkg/services/workers/internal.
// Peers should construct through workers/wire; baseline deletion of this path is
// owned by DEL-WRK.
package service
