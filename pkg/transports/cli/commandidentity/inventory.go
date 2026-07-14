package commandidentity

// FormatVersion identifies the CLI command identity inventory document shape.
const FormatVersion = "cli-command-identity/v1"

const (
	visibilityVisible = "visible"
	visibilityHidden  = "hidden"

	lifecycleActive     = "active"
	lifecycleDeprecated = "deprecated"
)

// Inventory is the root document emitted by the command identity walker.
type Inventory struct {
	FormatVersion string          `json:"formatVersion"`
	RootPath      string          `json:"rootPath"`
	Commands      []CommandRecord `json:"commands"`
}

// CommandRecord captures one reachable Cobra command identity.
type CommandRecord struct {
	IDCandidate       string   `json:"idCandidate"`
	Name              string   `json:"name"`
	Path              string   `json:"path"`
	Aliases           []string `json:"aliases"`
	GroupID           string   `json:"groupId"`
	Short             string   `json:"short"`
	Long              string   `json:"long"`
	Example           string   `json:"example"`
	Visibility        string   `json:"visibility"`
	Lifecycle         string   `json:"lifecycle"`
	DeprecatedMessage string   `json:"deprecatedMessage"`
	Runnable          bool     `json:"runnable"`
	DocIDCandidate    string   `json:"docIdCandidate"`
	HandlerPresent    bool     `json:"handlerPresent"`
}
