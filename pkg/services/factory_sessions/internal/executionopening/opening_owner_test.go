package executionopening

import factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"

type openingOwnerStub struct {
	application      factorysessions.ApplicationOpeningScope
	directJavaScript factorysessions.DirectJavaScriptRunScope
	stdio            factorysessions.StdioOpeningScope
	applicationID    factorysessions.OpeningScopeID
	directID         factorysessions.OpeningScopeID
	stdioID          factorysessions.OpeningScopeID
}

func (owner *openingOwnerStub) RegisterApplication(scope factorysessions.ApplicationOpeningScope) (factorysessions.OpeningScopeID, error) {
	owner.application = scope
	owner.applicationID = "application-test"
	return owner.applicationID, nil
}

func (owner *openingOwnerStub) Application(id factorysessions.OpeningScopeID) (factorysessions.ApplicationOpeningScope, bool) {
	return owner.application, id == owner.applicationID
}

func (owner *openingOwnerStub) RegisterDirectJavaScript(scope factorysessions.DirectJavaScriptRunScope) (factorysessions.OpeningScopeID, error) {
	owner.directJavaScript = scope
	owner.directID = "direct-test"
	return owner.directID, nil
}

func (owner *openingOwnerStub) DirectJavaScript(id factorysessions.OpeningScopeID) (factorysessions.DirectJavaScriptRunScope, bool) {
	return owner.directJavaScript, id == owner.directID
}

func (owner *openingOwnerStub) RegisterStdio(scope factorysessions.StdioOpeningScope) (factorysessions.OpeningScopeID, error) {
	owner.stdio = scope
	owner.stdioID = "stdio-test"
	return owner.stdioID, nil
}

func (owner *openingOwnerStub) Stdio(id factorysessions.OpeningScopeID) (factorysessions.StdioOpeningScope, bool) {
	return owner.stdio, id == owner.stdioID
}

func (owner *openingOwnerStub) ObserveHost(id factorysessions.OpeningScopeID, binding factorysessions.RuntimeHostBinding) {
	if id == owner.applicationID && owner.application.RuntimeHostObserver != nil {
		owner.application.RuntimeHostObserver(binding)
	}
	if id == owner.directID && owner.directJavaScript.RuntimeHostObserver != nil {
		owner.directJavaScript.RuntimeHostObserver(binding)
	}
}

func (owner *openingOwnerStub) Close(factorysessions.OpeningScopeID) {}
