package registeredowner

type Dependency interface { Run() }
type Service struct{}
type Domain struct{}
type Maker struct{}
type Alias = Maker
type Embedded struct { Maker }

func New(dep Dependency) *Service { return &Service{} }
func Generic[T any](dep Dependency) *Service { return &Service{} }
func Value() Domain { return Domain{} }
func (Maker) Construct(dep Dependency) *Service { return &Service{} }
