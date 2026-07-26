package cliinputs

// FormatVersion identifies the CLI command inputs inventory document shape.
const FormatVersion = "cli-command-inputs/v1"

const (
	argumentKindPositional = "positional"

	flagScopeLocal      = "local"
	flagScopePersistent = "persistent"
	flagScopeInherited  = "inherited"

	visibilityVisible = "visible"
	visibilityHidden  = "hidden"

	relationshipKindMutuallyExclusive = "mutually-exclusive"
	relationshipKindRequiredTogether  = "required-together"
	relationshipKindAtLeastOne        = "at-least-one"
	relationshipKindConditional       = "conditional"
)

// Inventory is the root document emitted by the CLI inputs walker.
type Inventory struct {
	FormatVersion string               `json:"formatVersion"`
	Arguments     []ArgumentRecord     `json:"arguments"`
	Flags         []FlagRecord         `json:"flags"`
	Relationships []RelationshipRecord `json:"relationships"`
}

// CommandJoin links an inputs record to a Batch 01 command identity.
type CommandJoin struct {
	CommandPath        string `json:"commandPath"`
	CommandIDCandidate string `json:"commandIdCandidate"`
}

// ArgumentRecord captures one positional argument Cobra exposes on a command.
type ArgumentRecord struct {
	CommandJoin

	IDCandidate        string   `json:"idCandidate"`
	Name               string   `json:"name"`
	DocIDCandidate     string   `json:"docIdCandidate"`
	Position           int      `json:"position"`
	Kind               string   `json:"kind"`
	ValueType          string   `json:"valueType"`
	Required           bool     `json:"required"`
	MinCardinality     int      `json:"minCardinality"`
	MaxCardinality     int      `json:"maxCardinality"`
	Variadic           bool     `json:"variadic"`
	Enum               []string `json:"enum"`
	Pattern            string   `json:"pattern"`
	CompletionKind     string   `json:"completionKind"`
	InputChannels      []string `json:"inputChannels"`
	DoubleDashHandling string   `json:"doubleDashHandling"`
}

// FlagRecord captures one flag Cobra exposes on a command.
type FlagRecord struct {
	CommandJoin

	IDCandidate       string   `json:"idCandidate"`
	Long              string   `json:"long"`
	Shorthand         string   `json:"shorthand"`
	Aliases           []string `json:"aliases"`
	Scope             string   `json:"scope"`
	ValueType         string   `json:"valueType"`
	Required          bool     `json:"required"`
	Default           string   `json:"default"`
	ChangedDefault    bool     `json:"changedDefault"`
	NoOptionDefault   string   `json:"noOptionDefault"`
	Repeatable        bool     `json:"repeatable"`
	Enum              []string `json:"enum,omitempty"`
	Normalization     string   `json:"normalization"`
	CompletionKind    string   `json:"completionKind"`
	Binding           string   `json:"binding"`
	Visibility        string   `json:"visibility"`
	Deprecated        bool     `json:"deprecated"`
	DeprecatedMessage string   `json:"deprecatedMessage"`
}

// RelationshipRecord captures one parse-constraint group registered on a command.
type RelationshipRecord struct {
	CommandJoin

	IDCandidate  string   `json:"idCandidate"`
	Kind         string   `json:"kind"`
	Participants []string `json:"participants"`
}

// EmptyInventory returns a root document with empty input collections.
func EmptyInventory() Inventory {
	return Inventory{
		FormatVersion: FormatVersion,
		Arguments:     []ArgumentRecord{},
		Flags:         []FlagRecord{},
		Relationships: []RelationshipRecord{},
	}
}
