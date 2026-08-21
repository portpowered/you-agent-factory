package main

type scanResult struct {
	rootPackageFindings                  []rootPackageFinding
	retiredPackageRootFindings           []retiredPackageRootFinding
	retiredPackageImportFindings         []retiredPackageImportFinding
	migrationShimFindings                []migrationShimFinding
	applicationGraphImportFindings       []applicationGraphImportFinding
	handwrittenGeneratedFindings         []handwrittenGeneratedFinding
	domainTransportFindings              []domainTransportImportFinding
	peerServiceImportFindings            []peerServiceImportFinding
	recordedPeerServiceImportFindings    []peerServiceImportFinding
	stalePeerServiceBaselineEntries      []peerServiceImportBaselineEntry
	peerServiceBaselineCount             int
	testServiceImportFindings            []testServiceImportFinding
	recordedTestServiceImportFindings    []testServiceImportFinding
	staleTestServiceBaselineEntries      []testServiceImportBaselineEntry
	testServiceBaselineCount             int
	supportServiceImportFindings         []supportServiceImportFinding
	recordedSupportServiceImportFindings []supportServiceImportFinding
	staleSupportServiceBaselineEntries   []supportServiceImportBaselineEntry
	supportServiceBaselineCount          int
	serviceConstructionFindings          []serviceConstructionFinding
	recordedServiceConstructionFindings  []serviceConstructionFinding
	staleServiceConstructionEntries      []serviceConstructionBaselineEntry
	serviceConstructionBaselineCount     int
	transportImplementationFindings      []transportServiceImplementationFinding
	externalImplementationFindings       []transportServiceImplementationFinding
	transportBehaviorFindings            []transportBehaviorFinding
	recordedTransportBehaviorFindings    []transportBehaviorFinding
	staleTransportBehaviorEntries        []transportBehaviorBaselineEntry
	transportBehaviorBaselineCount       int
	functionalProcessEdgeFindings        []functionalProcessEdgeFinding
	constructedServiceEdgesFindings      []constructedServiceEdgesFinding
	testWorkNormalizationFindings        []testWorkNormalizationFinding
	productionDefaultFindings            []productionDefaultFinding
	recordedProductionDefaultFindings    []productionDefaultFinding
	staleProductionDefaultEntries        []productionDefaultBaselineEntry
	productionDefaultBaselineCount       int
	initializerBehaviorFindings          []initializerBehaviorFinding
	recordedInitializerBehaviorFindings  []initializerBehaviorFinding
	staleInitializerBehaviorEntries      []initializerBehaviorBaselineEntry
	initializerBehaviorBaselineCount     int
	testBehaviorFindings                 []testBehaviorFinding
	recordedTestBehaviorFindings         []testBehaviorFinding
	staleTestBehaviorEntries             []testBehaviorBaselineEntry
	testBehaviorBaselineCount            int
	petriPublicSurfaceFindings           []petriPublicSurfaceFinding
	recordedPetriPublicSurfaceFindings   []petriPublicSurfaceFinding
	stalePetriPublicSurfaceEntries       []petriPublicSurfaceBaselineEntry
	petriPublicSurfaceBaselineCount      int
	providerEffectOwnershipFindings      []providerEffectOwnershipFinding
}

type retiredPackageRoot struct {
	packagePath    string
	canonicalOwner string
}

type retiredPackageRootFinding struct {
	retiredPackageRoot
}

type retiredPackageImportFinding struct {
	retiredPackageRoot
	importPath string
	filePath   string
	class      boundarySourceClass
}

type handwrittenGeneratedFinding struct {
	filePath    string
	packagePath string
}

type rootPackageFinding struct {
	packagePath string
}

type migrationShimFinding struct {
	packagePath     string
	marker          string
	canonicalTarget string
}

type applicationGraphImportFinding struct {
	packagePath string
	filePath    string
	class       boundarySourceClass
}

type domainTransportImportFinding struct {
	packagePath string
	importPath  string
	filePath    string
	class       boundarySourceClass
}

type peerServiceImportFinding struct {
	owner      string
	peer       string
	importPath string
	filePath   string
	class      boundarySourceClass
}

type peerServiceImportBaseline struct {
	Version int                              `json:"version"`
	Entries []peerServiceImportBaselineEntry `json:"entries"`
}

type peerServiceImportBaselineEntry struct {
	Owner        string `json:"owner"`
	Peer         string `json:"peer"`
	ImportPath   string `json:"importPath"`
	FilePath     string `json:"filePath"`
	TargetRoot   string `json:"targetRoot"`
	Class        string `json:"class,omitempty"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

type transportServiceImplementationFinding struct {
	importPath string
	filePath   string
	class      boundarySourceClass
}

type testServiceImportFinding struct {
	owner      string
	importPath string
	filePath   string
	class      boundarySourceClass
}

type testServiceImportBaseline struct {
	Version int                              `json:"version"`
	Entries []testServiceImportBaselineEntry `json:"entries"`
}

type testServiceImportBaselineEntry struct {
	Owner        string `json:"owner"`
	ImportPath   string `json:"importPath"`
	FilePath     string `json:"filePath"`
	TargetRoot   string `json:"targetRoot"`
	Class        string `json:"class,omitempty"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}

type serviceConstructionFinding struct {
	owner      string
	importPath string
	symbol     string
	filePath   string
	line       int
	count      int
	class      boundarySourceClass
}

type serviceConstructionBaseline struct {
	Version int                                `json:"version"`
	Entries []serviceConstructionBaselineEntry `json:"entries"`
}

type serviceConstructionBaselineEntry struct {
	Owner        string `json:"owner"`
	ImportPath   string `json:"importPath"`
	Symbol       string `json:"symbol"`
	FilePath     string `json:"filePath"`
	Count        int    `json:"count"`
	Class        string `json:"class,omitempty"`
	Stage        string `json:"stage"`
	DeletionGate string `json:"deletionGate"`
}
