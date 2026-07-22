package factorydefinitions

// AuthoredFactorySourceLoader resolves an authored Factory Definition path and
// returns its representation bytes. The Factory Definitions service owns path
// and filesystem policy; consumers decode the bytes at their representation
// boundary.
type AuthoredFactorySourceLoader func(path string) ([]byte, error)
