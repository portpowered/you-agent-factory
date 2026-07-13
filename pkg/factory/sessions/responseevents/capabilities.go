package responseevents

// Capabilities declares which response-event features a provider session supports.
// Adapters publish capability flags so consumers can interpret fidelity and phase
// availability without depending on provider-native schemas.
type Capabilities struct {
	NativeStreaming    bool `json:"nativeStreaming"`
	MessageDeltas      bool `json:"messageDeltas"`
	MessageSnapshots   bool `json:"messageSnapshots"`
	ReasoningSummaries bool `json:"reasoningSummaries"`
	ToolLifecycle      bool `json:"toolLifecycle"`
	ToolOutputDeltas   bool `json:"toolOutputDeltas"`
	FileChanges        bool `json:"fileChanges"`
	Plans              bool `json:"plans"`
	Usage              bool `json:"usage"`
	StableItemIDs      bool `json:"stableItemIds"`
	ProviderReconnect  bool `json:"providerReconnect"`
}
