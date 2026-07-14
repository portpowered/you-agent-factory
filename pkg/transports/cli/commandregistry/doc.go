// Package commandregistry maps stable CLI command IDs to handwritten Cobra RunE
// handlers for the representative root/session-show family and the
// factory/config/init family. Generated metadata and constructors attach
// behavior through this registry instead of embedding RunE bodies in generated
// packages.
package commandregistry
