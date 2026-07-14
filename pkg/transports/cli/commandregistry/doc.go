// Package commandregistry maps stable CLI command IDs to handwritten Cobra RunE
// handlers for contracted CLI families. Generated metadata and constructors
// attach behavior through this registry instead of embedding RunE bodies in
// generated packages. Representative-family handlers cover you and you.session.show;
// work-family handlers cover you.work.list, you.work.show, you.work.move, and
// you.work.visualize.
package commandregistry
