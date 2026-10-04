package behaviorfixture

type Client struct{} // want `transport-alternate-test-entrypoint:.*Client`
func NewClientMock() {} // want `transport-alternate-test-entrypoint:.*NewClientMock`
