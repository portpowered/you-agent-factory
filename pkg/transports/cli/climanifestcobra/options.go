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
}
