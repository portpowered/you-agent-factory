package wire

// Legacy Workers registry projection lived here and created a reverse ownership
// edge. That projection is removed: Workers adapts the singular Providers
// Service through its own composition boundary, and Providers publishes only
// Providers-owned root contracts.
