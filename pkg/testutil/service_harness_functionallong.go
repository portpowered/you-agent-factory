//go:build functionallong

package testutil

// WithExecutionBaseDir overrides the runtime base directory used to resolve
// relative workstation execution paths.
func WithExecutionBaseDir(dir string) ServiceTestHarnessOption {
	return func(cfg *harnessConfig) {
		cfg.serviceConfig.ExecutionBaseDir = dir
	}
}
