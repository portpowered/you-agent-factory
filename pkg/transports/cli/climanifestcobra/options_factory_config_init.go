package climanifestcobra

// FactoryConfigInitFlagBindings supplies live variables for local flags declared on
// generated factory/config/init runnable leaves.
type FactoryConfigInitFlagBindings struct {
	FactoryListDir          *string
	FactoryCreateDir        *string
	FactoryUpdateDir        *string
	FactoryDeleteDir        *string
	FactoryCreateFrom       *string
	FactoryCreateSetCurrent *bool
	FactoryUpdateFrom       *string
	FactoryReplaceSessionID *string
	InitDir                 *string
	InitType                *string
	InitExecutor            *string
	FlagUsages              map[string]string
}
