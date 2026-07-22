package factorysessions

// Registry is the live Factory Session directory consumed by session runtime
// roles. Its mutable implementation lives below the service root.
type Registry interface {
	Upsert(*LiveSession, bool)
	Select(string) bool
	Current() *LiveSession
	Get(string) *LiveSession
	Remove(string)
	Count() int
	IDs() []string
	DefaultSession() *LiveSession
	FindByLogicalSessionKeyID(string) *LiveSession
}
