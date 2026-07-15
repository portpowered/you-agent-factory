package retiredsurfaceguard

// Violation is one retired-surface reintroduction finding reported by repository guards.
type Violation struct {
	Family  string
	Surface string
	Detail  string
}
