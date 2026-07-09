// Package composebridge is a temporary startup bridge used by pkg/initializer.
//
// It keeps initializer/runtimehost startup wiring compiling during composition
// migration. Dependency construction should move to pkg/inject as import-cycle
// migration allows; this package should not become the final composition root.
package composebridge
