package climanifest

// Manifest is the production CLI command manifest document.
type Manifest struct {
	FormatVersion string             `json:"formatVersion"`
	RootPath      string             `json:"rootPath"`
	Commands      map[string]Command `json:"commands"`
}

// Command is one §4.3 command record from the production manifest.
type Command struct {
	ID            string              `json:"id"`
	Name          string              `json:"name"`
	Path          string              `json:"path"`
	Aliases       []string            `json:"aliases"`
	GroupID       string              `json:"groupId,omitempty"`
	Documentation Documentation       `json:"documentation"`
	Visibility    string              `json:"visibility"`
	Runnable      bool                `json:"runnable"`
	Usage         Usage               `json:"usage"`
	Arguments     map[string]Argument `json:"arguments,omitempty"`
	Flags         map[string]Flag     `json:"flags,omitempty"`
	Channels      Channels            `json:"channels,omitempty"`
	Outputs       map[string]Output   `json:"outputs,omitempty"`
	Exits         map[string]Exit     `json:"exits,omitempty"`
	SideEffects   map[string]SideEffect `json:"sideEffects,omitempty"`
	Constraints   Constraints         `json:"constraints,omitempty"`
	Handler       *Handler            `json:"handler,omitempty"`
}

// Handler carries stable handler identity and optional OpenAPI operation binding.
type Handler struct {
	ID          string `json:"id"`
	OperationID string `json:"operationId,omitempty"`
}

// Channels carries declared input and output channel surfaces.
type Channels struct {
	Input  []string `json:"input"`
	Output []string `json:"output"`
}

// Output is one declared command output record.
type Output struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	Format  string `json:"format"`
}

// Exit is one declared process exit outcome.
type Exit struct {
	ID   string `json:"id"`
	Code int    `json:"code"`
	Kind string `json:"kind"`
}

// SideEffect is one declared command side effect.
type SideEffect struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

// Constraints carries runtime and platform declarations.
type Constraints struct {
	Runtime   []string `json:"runtime"`
	Platforms []string `json:"platforms"`
}

// Documentation carries shared documentation-schema fields used for help parity.
type Documentation struct {
	Documentation DocumentationCopy `json:"documentation"`
	Examples      []string          `json:"examples"`
}

// DocumentationCopy holds canonical English help copy.
type DocumentationCopy struct {
	Title       DocumentationField `json:"title"`
	Description DocumentationField `json:"description"`
}

// DocumentationField is one canonical documentation string.
type DocumentationField struct {
	CanonicalEnglish string `json:"canonicalEnglish"`
}

// Usage carries invocation-line and example metadata.
type Usage struct {
	Line    string `json:"line"`
	Example string `json:"example,omitempty"`
}
