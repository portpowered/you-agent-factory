package climanifest

// Manifest is the production CLI command manifest document.
type Manifest struct {
	FormatVersion string             `json:"formatVersion"`
	RootPath      string             `json:"rootPath"`
	Commands      map[string]Command `json:"commands"`
}

// Command is one §4.3 command record from the production manifest.
type Command struct {
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Path          string        `json:"path"`
	Aliases       []string      `json:"aliases"`
	GroupID       string        `json:"groupId,omitempty"`
	Documentation Documentation `json:"documentation"`
	Visibility    string        `json:"visibility"`
	Runnable      bool          `json:"runnable"`
	Usage         Usage         `json:"usage"`
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
