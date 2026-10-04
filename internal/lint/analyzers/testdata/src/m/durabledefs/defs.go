package durabledefs

type DirectoryStore struct{ Dir string }
type Config struct{ PersistSessions bool }

func NewJavaScriptRuntimeService[T any](T) {}
func NewLazyProjectStore(string)           {}
func DirForProjectRoot(root string) string { return root }
func BuildInvocationBootstrap()            {}
func NewExecutionService()                 {}
func NewFakeServiceFromContractFixtures()  {}
func ProjectPersistence()                  {}
func AppendDispatchInterruptedEvent()      {}
func BuildCanonicalRuntimeSessionEvents()  {}
func MapCanonicalRuntimeSessionEvents()    {}

type Provider interface{ Infer() }
type Worker interface{ Execute() }
