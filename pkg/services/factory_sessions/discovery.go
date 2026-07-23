package factorysessions

// TargetProbe loads metadata for one discovered factory session target.
type TargetProbe func(folderPath string, factoryDir string, ref TargetRef) (Target, bool, *DiscoveryFailure)

// DiscoveryFailure captures one target that looked like a factory but failed to load.
type DiscoveryFailure struct {
	FactoryDir string
	Ref        TargetRef
	Summary    string
}
