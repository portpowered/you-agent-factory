package functionaltestmetadata

// Record is one inventoried top-level Test* declaration.
type Record struct {
	// File is the repository-relative source path using forward slashes.
	File string `json:"file"`
	// Package is the Go package name declared in the source file.
	Package string `json:"package"`
	// Name is the Test* function name.
	Name string `json:"name"`
	// Line is the 1-based source line of the function declaration.
	Line int `json:"line"`
	// Description is the first sentence of the conventional Go doc comment.
	// Empty when Undocumented is true.
	Description string `json:"description,omitempty"`
	// Undocumented is true when the declaration has no conventional Go doc
	// comment first sentence.
	Undocumented bool `json:"undocumented"`
}

// Identity returns the stable catalog identity for this record.
func (r Record) Identity() string {
	return r.File + "::" + r.Name
}
