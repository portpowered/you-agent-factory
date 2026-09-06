package proto

import "google.golang.org/grpc"

// RegisterBackendServerAtProductionPath exposes the same wire-compatible
// fixture service under the pinned LocalAI service name. The fixture's own
// generated namespace stays distinct so its descriptors do not collide with
// the private production subset, while NetworkDialer can still cross the
// production method path in root-composition tests.
func RegisterBackendServerAtProductionPath(s grpc.ServiceRegistrar, srv BackendServer) {
	serviceDescription := Backend_ServiceDesc
	serviceDescription.ServiceName = "backend.Backend"
	s.RegisterService(&serviceDescription, srv)
}
