package main

type scanResult struct {
	rootPackageFindings                 []rootPackageFinding
	retiredPackageRootFindings          []retiredPackageRootFinding
	serviceConstructionFindings         []serviceConstructionFinding
	recordedServiceConstructionFindings []serviceConstructionFinding
	staleServiceConstructionEntries     []serviceConstructionBaselineEntry
	serviceConstructionBaselineCount    int
	productionDefaultFindings           []productionDefaultFinding
	recordedProductionDefaultFindings   []productionDefaultFinding
	staleProductionDefaultEntries       []productionDefaultBaselineEntry
	productionDefaultBaselineCount      int
	petriPublicSurfaceFindings          []petriPublicSurfaceFinding
	recordedPetriPublicSurfaceFindings  []petriPublicSurfaceFinding
	stalePetriPublicSurfaceEntries      []petriPublicSurfaceBaselineEntry
	petriPublicSurfaceBaselineCount     int
}

type retiredPackageRoot struct {
	packagePath    string
	canonicalOwner string
}

type retiredPackageRootFinding struct {
	retiredPackageRoot
}

type rootPackageFinding struct {
	packagePath string
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
