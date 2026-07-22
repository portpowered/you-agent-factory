package factorydefinitions

// InvocationWorkTypeService selects the Work Type that receives simplified
// Factory invocation input.
type InvocationWorkTypeService interface {
	DefaultWorkType(*FactoryConfig) (string, error)
}
