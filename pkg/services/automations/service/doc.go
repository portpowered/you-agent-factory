// Package service is a transitional compile shim that re-exports the composed
// Automations root from pkg/services/automations/internal. Peers should
// construct through automations/wire; baseline deletion of this path is owned
// by DEL-AUTO.
package service
