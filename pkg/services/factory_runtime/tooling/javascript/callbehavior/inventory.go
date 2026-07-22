package callbehavior

// FormatVersion identifies the JavaScript runtime call-behavior inventory shape.
const FormatVersion = "javascript-runtime-call-behavior/v1"

const (
	kindValue     = "value"
	kindNamespace = "namespace"
	kindFunction  = "function"
)

// Inventory is the root document emitted by the installed call-behavior descriptor.
type Inventory struct {
	FormatVersion string               `json:"formatVersion"`
	Records       []CallBehaviorRecord `json:"records"`
}

// CallBehaviorRecord captures observable call details for one installed symbol.
type CallBehaviorRecord struct {
	IDCandidate string `json:"idCandidate"`
	Path        string `json:"path"`
	Kind        string `json:"kind"`

	Mutability  string `json:"mutability,omitempty"`
	Nullability string `json:"nullability,omitempty"`
	Lifecycle   string `json:"lifecycle,omitempty"`

	Callable       bool            `json:"callable,omitempty"`
	Async          bool            `json:"async,omitempty"`
	Parameters     []Parameter     `json:"parameters,omitempty"`
	Callback       *CallbackShape  `json:"callback,omitempty"`
	Return         *ReturnBehavior `json:"return,omitempty"`
	EmittedRecords []string        `json:"emittedRecords,omitempty"`
	Errors         []ErrorCase     `json:"errors,omitempty"`
	PolicyChecks   []PolicyCheck   `json:"policyChecks,omitempty"`
	Determinism    string          `json:"determinism,omitempty"`
	ResumeNotes    string          `json:"resumeNotes,omitempty"`
}

// Parameter documents one ordered callable argument.
type Parameter struct {
	IDCandidate      string           `json:"idCandidate"`
	Name             string           `json:"name"`
	Required         bool             `json:"required"`
	Rest             bool             `json:"rest,omitempty"`
	Default          string           `json:"default,omitempty"`
	Type             string           `json:"type"`
	ObjectProperties []ObjectProperty `json:"objectProperties,omitempty"`
}

// ObjectProperty documents one required or optional object field.
type ObjectProperty struct {
	IDCandidate string `json:"idCandidate"`
	Name        string `json:"name"`
	Required    bool   `json:"required"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
}

// CallbackShape documents a function argument or stage callback.
type CallbackShape struct {
	Role       string      `json:"role,omitempty"`
	Parameters []Parameter `json:"parameters"`
	Notes      string      `json:"notes,omitempty"`
}

// ReturnBehavior documents synchronous and Promise return behavior.
type ReturnBehavior struct {
	SyncType    string `json:"syncType,omitempty"`
	Async       bool   `json:"async,omitempty"`
	PromiseType string `json:"promiseType,omitempty"`
}

// ErrorCase documents one observable failure shape.
type ErrorCase struct {
	Condition string `json:"condition"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

// PolicyCheck documents one policy gate evaluated before side effects.
type PolicyCheck struct {
	Kind    string `json:"kind"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}
