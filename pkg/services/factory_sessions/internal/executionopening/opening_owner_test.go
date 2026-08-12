package executionopening

import (
	"context"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
)

type openingOwnerStub struct {
	directJavaScript factorysessions.DirectJavaScriptRunScope
	stdio            factorysessions.StdioOpeningScope
	directID         factorysessions.OpeningScopeID
	stdioID          factorysessions.OpeningScopeID
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

func (*openingOwnerStub) RegisterInvocationEvents(factorysessions.InvocationEventScope) (factorysessions.OpeningScopeID, error) {
	return "", nil
}

func (*openingOwnerStub) InvocationEvents(factorysessions.OpeningScopeID) (factorysessions.FactoryEventConsumer, bool) {
	return nil, false
}

func (*openingOwnerStub) StartFactoryEventBridge(context.Context, roles.FactoryEventReader, factorysessions.OpeningScopeID) (interface {
	Finish(context.Context, roles.FactoryEventReader, factorysessions.FactoryInvocationOutcome) error
}, error) {
	return nil, nil
}

func (owner *openingOwnerStub) Close(factorysessions.OpeningScopeID) {}
