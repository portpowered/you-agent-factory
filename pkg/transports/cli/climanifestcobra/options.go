package climanifestcobra

// PersistentFlagBindings supplies live variables for root persistent flags declared
// in generated representative-family metadata.
type PersistentFlagBindings struct {
	Verbose                    *bool
	Debug                      *bool
	Server                     *string
	JSON                       *bool
	DefaultWorkerModelProvider *string
	DefaultWorkerModel         *string
	// FlagUsages supplies Cobra help text for persistent flags when the manifest
	// does not yet carry per-flag usage descriptions.
	FlagUsages map[string]string
}
