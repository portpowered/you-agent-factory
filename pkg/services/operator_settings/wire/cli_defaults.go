package wire

// RegisterDefaultsResolutionFromHome is retained as a compatibility no-op for
// old composition tests. Defaults resolution is selected by the injected
// Operator Settings Service and never through a package-global hook.
func RegisterDefaultsResolutionFromHome() {}
