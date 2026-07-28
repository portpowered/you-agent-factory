// Package service is a transitional compile shim that re-exports the composed
// Recordings root from pkg/services/recordings/internal. Peers should
// construct through recordings/wire; baseline deletion of this path is owned
// by DEL-REC.
package service
