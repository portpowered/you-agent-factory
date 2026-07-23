package factorysessions

// HomeDirectoryResolver resolves the process user's home directory at the
// external filesystem edge selected by Wire.
type HomeDirectoryResolver func() (string, error)
