package providers

// Providers lifecycle is a structural shutdown role. The process composition
// boundary asserts Close directly instead of publishing a second root
// interface alongside Service.
