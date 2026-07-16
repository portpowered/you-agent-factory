// Package restclient provides the pre-DI functional adapter around the
// generated REST response client. Callers retain ownership of the endpoint and
// HTTP client; production-shaped dependency graph coverage belongs to later
// functional graph tests.
package restclient
