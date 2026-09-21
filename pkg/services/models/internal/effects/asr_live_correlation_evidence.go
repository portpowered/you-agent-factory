package effects

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
)

const (
	ASRLiveCorrelationEvidenceSchema        = "asr-live-correlation/v1"
	ASRLiveCorrelationScenarioResponseFirst = "response_first"
	ASRLiveCorrelationScenarioExitFirst     = "exit_first_after_rpc_terminal"
)

// ASRLiveCorrelationEvidence is the private, bounded record emitted by the
// compiled Windows ASR correlation harness. It intentionally contains no raw
// endpoint, path, model response, transcript, token, or backend diagnostic.
type ASRLiveCorrelationEvidence struct {
	Schema          string                        `json:"schema"`
	RunID           string                        `json:"run_id"`
	Scenario        string                        `json:"scenario"`
	SourceCommit    string                        `json:"source_commit"`
	SourceTree      string                        `json:"source_tree"`
	GoToolchain     string                        `json:"go_toolchain"`
	ExecutableSHA   string                        `json:"executable_sha256"`
	WAVSHA          string                        `json:"wav_sha256"`
	ModelSHA        string                        `json:"model_identity_sha256"`
	BackendSHA      string                        `json:"backend_artifact_sha256"`
	CacheSHA        string                        `json:"cache_manifest_sha256"`
	Endpoint        ASRLiveCorrelationEndpoint    `json:"endpoint"`
	RPC             ASRLiveCorrelationRPC         `json:"rpc"`
	Child           ASRLiveCorrelationChild       `json:"child"`
	Application     ASRLiveCorrelationApplication `json:"application"`
	Cleanup         ASRLiveCorrelationCleanup     `json:"cleanup"`
	RedactionPassed bool                          `json:"redaction_scan_passed"`
}

// ASRLiveCorrelationEndpoint is the redacted identity of a loopback listener.
type ASRLiveCorrelationEndpoint struct {
	Network           string `json:"network"`
	AddressClass      string `json:"address_class"`
	Port              int    `json:"port"`
	IdentitySHA256    string `json:"identity_sha256"`
	ListenerProcessID int    `json:"listener_process_id"`
}

// ASRLiveCorrelationRPC records the successful gRPC terminal and semantic
// request/response identities without retaining either payload.
type ASRLiveCorrelationRPC struct {
	Method                 string `json:"method"`
	Status                 string `json:"status"`
	TerminalSequence       uint64 `json:"terminal_sequence"`
	ResponseDecoded        bool   `json:"response_decoded"`
	RequestSemanticSHA256  string `json:"request_semantic_sha256"`
	ResponseSemanticSHA256 string `json:"response_semantic_sha256"`
}

// ASRLiveCorrelationChild records the owner-scoped operating-system Wait.
type ASRLiveCorrelationChild struct {
	ProcessID     int    `json:"process_id"`
	WaitSequence  uint64 `json:"wait_sequence"`
	ExitClass     string `json:"exit_class"`
	ExitCodeKnown bool   `json:"exit_code_known"`
	ExitCode      int    `json:"exit_code"`
}

// ASRLiveCorrelationApplication records only the typed final result shape.
type ASRLiveCorrelationApplication struct {
	Outcome      string   `json:"outcome"`
	ErrorClasses []string `json:"error_classes"`
	OutputCount  int      `json:"output_count"`
}

// ASRLiveCorrelationCleanup records counts for resources owned by this run.
type ASRLiveCorrelationCleanup struct {
	OwnedProcessesRemaining int `json:"owned_processes_remaining"`
	OwnedListenersRemaining int `json:"owned_listeners_remaining"`
	StagedAudioRemaining    int `json:"staged_audio_remaining"`
}

// ASRLiveCorrelationEndpointFromAddress converts a transient gRPC address to
// a redacted endpoint identity. The address itself is never returned.
func ASRLiveCorrelationEndpointFromAddress(
	address string,
	listenerProcessID int,
	expectedPort int,
) (ASRLiveCorrelationEndpoint, error) {
	_, port, err := parseASRCorrelationEndpointAddress(address)
	if err != nil || !validASRCorrelationPort(expectedPort) || port == 7437 ||
		port != expectedPort || listenerProcessID <= 0 {
		return ASRLiveCorrelationEndpoint{}, errors.New("invalid ASR correlation endpoint ownership")
	}
	endpoint := ASRLiveCorrelationEndpoint{
		Network: "tcp", AddressClass: "LOOPBACK", Port: port,
		ListenerProcessID: listenerProcessID,
	}
	endpoint.IdentitySHA256 = asrCorrelationEndpointSHA256(endpoint)
	return endpoint, nil
}

// ValidateASRLiveCorrelationEvidence checks enums, identity digests, event
// ordering, endpoint ownership, result atomicity, and task-owned cleanup.
func ValidateASRLiveCorrelationEvidence(evidence ASRLiveCorrelationEvidence) error {
	if evidence.Schema != ASRLiveCorrelationEvidenceSchema ||
		!safeCorrelationRunID(evidence.RunID) || !validASRCorrelationScenario(evidence.Scenario) ||
		!validASRCorrelationGitID(evidence.SourceCommit) || !validASRCorrelationGitID(evidence.SourceTree) ||
		evidence.GoToolchain != "go1.26.8 windows/amd64" ||
		!validASRCorrelationDigest(evidence.ExecutableSHA) || !validASRCorrelationDigest(evidence.WAVSHA) ||
		!validASRCorrelationDigest(evidence.ModelSHA) || !validASRCorrelationDigest(evidence.BackendSHA) ||
		!validASRCorrelationDigest(evidence.CacheSHA) {
		return errors.New("invalid ASR live-correlation evidence identity")
	}
	if err := validateASRCorrelationEndpoint(evidence.Endpoint); err != nil {
		return err
	}
	if err := validateASRCorrelationRPC(evidence.RPC); err != nil {
		return err
	}
	if err := validateASRCorrelationChild(evidence.Child, evidence.Endpoint, evidence.RPC); err != nil {
		return err
	}
	if err := validateASRCorrelationApplication(evidence.Application); err != nil {
		return err
	}
	if evidence.Cleanup != (ASRLiveCorrelationCleanup{}) || !evidence.RedactionPassed {
		return errors.New("ASR live-correlation evidence has incomplete cleanup or redaction")
	}
	return nil
}

// MarshalASRLiveCorrelationEvidence validates before serializing the bounded
// evidence record.
func MarshalASRLiveCorrelationEvidence(evidence ASRLiveCorrelationEvidence) ([]byte, error) {
	if err := ValidateASRLiveCorrelationEvidence(evidence); err != nil {
		return nil, err
	}
	return json.Marshal(evidence)
}

// UnmarshalASRLiveCorrelationEvidence rejects unknown or trailing JSON fields
// before applying the semantic validator.
func UnmarshalASRLiveCorrelationEvidence(payload []byte) (ASRLiveCorrelationEvidence, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	var evidence ASRLiveCorrelationEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return ASRLiveCorrelationEvidence{}, errors.New("invalid ASR live-correlation JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return ASRLiveCorrelationEvidence{}, errors.New("ASR live-correlation JSON has trailing values")
	}
	if err := ValidateASRLiveCorrelationEvidence(evidence); err != nil {
		return ASRLiveCorrelationEvidence{}, err
	}
	return evidence, nil
}

func validateASRCorrelationEndpoint(endpoint ASRLiveCorrelationEndpoint) error {
	if endpoint.Network != "tcp" || endpoint.AddressClass != "LOOPBACK" ||
		!validASRCorrelationPort(endpoint.Port) || endpoint.Port == 7437 || endpoint.ListenerProcessID <= 0 ||
		!validASRCorrelationDigest(endpoint.IdentitySHA256) ||
		endpoint.IdentitySHA256 != asrCorrelationEndpointSHA256(endpoint) {
		return errors.New("invalid ASR live-correlation endpoint identity")
	}
	return nil
}

func validateASRCorrelationRPC(rpc ASRLiveCorrelationRPC) error {
	if rpc.Method != "AUDIO_TRANSCRIPTION" || rpc.Status != "OK" || rpc.TerminalSequence == 0 ||
		!rpc.ResponseDecoded || !validASRCorrelationDigest(rpc.RequestSemanticSHA256) ||
		!validASRCorrelationDigest(rpc.ResponseSemanticSHA256) {
		return errors.New("invalid ASR live-correlation RPC terminal")
	}
	return nil
}

func validateASRCorrelationChild(
	child ASRLiveCorrelationChild,
	endpoint ASRLiveCorrelationEndpoint,
	rpc ASRLiveCorrelationRPC,
) error {
	if child.ProcessID <= 0 || child.ProcessID != endpoint.ListenerProcessID ||
		child.WaitSequence <= rpc.TerminalSequence || !validASRCorrelationExit(child) {
		return errors.New("invalid ASR live-correlation child Wait ownership")
	}
	return nil
}

func validASRCorrelationExit(child ASRLiveCorrelationChild) bool {
	switch child.ExitClass {
	case "EXITED":
		return child.ExitCodeKnown && child.ExitCode == 0
	case "NONZERO_EXIT":
		return child.ExitCodeKnown && child.ExitCode != 0
	case "WAIT_FAILED":
		return !child.ExitCodeKnown && child.ExitCode == 0
	default:
		return false
	}
}

func validateASRCorrelationApplication(application ASRLiveCorrelationApplication) error {
	if application.OutputCount < 0 || application.Outcome != "COMPLETED" && application.Outcome != "FAILED" {
		return errors.New("invalid ASR live-correlation application outcome")
	}
	if application.Outcome == "COMPLETED" {
		if application.OutputCount == 0 || len(application.ErrorClasses) != 0 {
			return errors.New("completed ASR live-correlation result is not atomic")
		}
		return nil
	}
	if application.OutputCount != 0 || len(application.ErrorClasses) != 2 ||
		!containsASRCorrelationError(application.ErrorClasses, "INFERENCE_FAILED") ||
		!containsASRCorrelationError(application.ErrorClasses, "HOST_LEASE_EXPIRED") {
		return errors.New("failed ASR live-correlation result is not atomic")
	}
	return nil
}

func containsASRCorrelationError(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func safeCorrelationRunID(value string) bool {
	if value == "" || len(value) > 96 {
		return false
	}
	for _, char := range value {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func validASRCorrelationScenario(value string) bool {
	return value == ASRLiveCorrelationScenarioResponseFirst || value == ASRLiveCorrelationScenarioExitFirst
}

func validASRCorrelationGitID(value string) bool {
	return len(value) == 40 && isLowerHex(value)
}

func validASRCorrelationDigest(value string) bool {
	return len(value) == sha256.Size*2 && isLowerHex(value)
}

func isLowerHex(value string) bool {
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return value != ""
}

func validASRCorrelationPort(port int) bool {
	return port >= 1024 && port <= 65535
}

func asrCorrelationEndpointSHA256(endpoint ASRLiveCorrelationEndpoint) string {
	identity := fmt.Sprintf("%s\x00%s\x00%d\x00%d", endpoint.Network, endpoint.AddressClass, endpoint.Port, endpoint.ListenerProcessID)
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func parseASRCorrelationEndpointAddress(address string) (string, int, error) {
	if address != strings.TrimSpace(address) {
		return "", 0, errors.New("invalid ASR correlation endpoint")
	}
	parsed, err := url.Parse(address)
	if err != nil || parsed.Scheme != "grpc" || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Path != "" || parsed.RawPath != "" || parsed.ForceQuery || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", 0, errors.New("invalid ASR correlation endpoint")
	}
	host, portText, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return "", 0, errors.New("invalid ASR correlation endpoint")
	}
	if portText == "" {
		return "", 0, errors.New("invalid ASR correlation endpoint port")
	}
	for _, character := range portText {
		if character < '0' || character > '9' {
			return "", 0, errors.New("invalid ASR correlation endpoint port")
		}
	}
	portNumber, err := strconv.ParseUint(portText, 10, 16)
	port := int(portNumber)
	if err != nil || !validASRCorrelationPort(port) || port == 7437 ||
		(host != "127.0.0.1" && host != "::1") {
		return "", 0, errors.New("invalid ASR correlation endpoint port")
	}
	return host, port, nil
}
