package cli

// BindService returns the composition-facing Models CLI adapter Service
// constructed from the accepted Models root. Wire and other composition roots
// inject the returned Service without constructing adapter behavior at the
// composition boundary.
func BindService(cfg Config) Service {
	return NewService(cfg)
}
