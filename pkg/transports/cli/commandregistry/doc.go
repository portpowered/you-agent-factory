// Package commandregistry maps stable CLI command IDs to handwritten Cobra
// lifecycles for contracted CLI families. Generated metadata and constructors
// attach behavior through this registry instead of embedding execution bodies
// in generated packages. Representative-family handlers cover you and you.session.show;
// work-family handlers cover you.work.list, you.work.watch, you.work.show,
// you.work.move, and you.work.render; factory/config/init handlers cover the B11 factory/config/init
// cutover slice; run/submit handlers cover you.run, you.submit, and
// you.submit.batch, including their retained PreRunE behavior.
package commandregistry
