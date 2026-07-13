// Package api preserves the historical HTTP server import path.
//
// Deprecated: use github.com/portpowered/infinite-you/pkg/transports/http.
// This forwarding package is scheduled for removal by Batch 008.
package api

import transporthttp "github.com/portpowered/infinite-you/pkg/transports/http"

// Server is the canonical HTTP transport server.
//
// Deprecated: use transport/http.Server.
type Server = transporthttp.Server

// ServerOptions configures optional HTTP server boundaries.
//
// Deprecated: use transport/http.ServerOptions.
type ServerOptions = transporthttp.ServerOptions

// NewServer forwards to the canonical HTTP transport constructor.
//
// Deprecated: use transport/http.NewServer.
var NewServer = transporthttp.NewServer

// NewServerWithOptions forwards to the canonical HTTP transport constructor.
//
// Deprecated: use transport/http.NewServerWithOptions.
var NewServerWithOptions = transporthttp.NewServerWithOptions
