package providers

// ACPIntegration is one customer-configured stdio Agent Client Protocol peer.
type ACPIntegration struct {
	ID        string
	Name      ID
	Aliases   []string
	Transport string
	Command   string
}

// Clone returns a detached integration copy.
func (integration ACPIntegration) Clone() ACPIntegration {
	integration.Aliases = append([]string(nil), integration.Aliases...)
	return integration
}

// Factory constructs the singular Providers root with invocation-scoped ACP
// configuration. Construction is inert; commands start only during Execute.
type Factory func([]ACPIntegration) (Service, error)
