package symbolidentity

// FormatVersion identifies the JavaScript runtime symbol identity inventory shape.
const FormatVersion = "javascript-runtime-symbol-identity/v1"

const (
	kindValue     = "value"
	kindNamespace = "namespace"
	kindFunction  = "function"
)

// Inventory is the root document emitted by the installed-binding descriptor.
type Inventory struct {
	FormatVersion string         `json:"formatVersion"`
	Symbols       []SymbolRecord `json:"symbols"`
}

// SymbolRecord captures one installed JavaScript runtime symbol identity.
// Identity fields only: no signatures, schemas, errors, limits, or examples.
type SymbolRecord struct {
	IDCandidate string   `json:"idCandidate"`
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Kind        string   `json:"kind"`
	Parent      string   `json:"parent,omitempty"`
	Members     []string `json:"members,omitempty"`
	Callable    bool     `json:"callable"`
	Async       bool     `json:"async"`
}
