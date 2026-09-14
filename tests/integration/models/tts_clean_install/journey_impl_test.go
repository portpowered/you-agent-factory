package tts_clean_install

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const (
	JourneyStepInstall              = "install"
	JourneyStepInstalledIdentity    = "installed-identity"
	JourneyStepHelp                 = "help"
	JourneyStepDocs                 = "docs"
	JourneyStepModelsList           = "models-list"
	JourneyStepModelsInspect        = "models-inspect"
	JourneyStepColdInvoke           = "cold-invoke"
	JourneyStepColdReadiness        = "cold-readiness"
	JourneyStepWarmOfflineInvoke    = "warm-offline-invoke"
	JourneyStepWarmOfflineReadiness = "warm-offline-readiness"
	JourneyStepRemove               = "remove"
	JourneyStepPostRemove           = "post-remove"
	JourneyStepCleanup              = "cleanup"

	journeyPass         = "PASS"
	journeyFail         = "FAIL"
	journeyInconclusive = "INCONCLUSIVE"
	realTTSEdge         = "VAL-TTS-REAL-WINDOWS"
	osSafetyEdge        = "localai-v19-tts-clean-install-probe-preparation-001-003"
)

var journeyStepOrder = []string{
	JourneyStepInstall, JourneyStepInstalledIdentity, JourneyStepHelp,
	JourneyStepDocs, JourneyStepModelsList, JourneyStepModelsInspect, JourneyStepColdInvoke,
	JourneyStepColdReadiness, JourneyStepWarmOfflineInvoke,
	JourneyStepWarmOfflineReadiness, JourneyStepRemove,
	JourneyStepPostRemove, JourneyStepCleanup,
}

type JourneyStep struct {
	Name     string   `json:"name"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

type AudioArtifact struct {
	Name      string
	MediaType string
	Data      []byte
}

type RedactionValues struct {
	Tokens       []string
	SignedURLs   []string
	AmbientPaths []string
	RawText      []string
	VoiceMarkers []string
}

type JourneyObservation struct {
	Steps             []JourneyStep
	InstalledIdentity *Identity
	Commands          []CommandEvidence
	Audio             []AudioArtifact
	Readiness         []ReadinessEvidence
	Network           NetworkEvidence
	Cleanup           CleanupEvidence
	Redactions        RedactionValues
}

type JourneyValidationError struct {
	Check    string
	Expected string
	Observed string
}

func (e *JourneyValidationError) Error() string {
	return fmt.Sprintf("journey %s failed: expected %s; observed %s", e.Check, e.Expected, e.Observed)
}

type AudioValidationError struct {
	Class  string
	Detail string
}

func (e *AudioValidationError) Error() string {
	return e.Class + ": " + e.Detail
}

var (
	redactionURLPattern         = regexp.MustCompile(`(?i)\b(?:https?|file)://[^\s"'<>]+`)
	redactionTokenPattern       = regexp.MustCompile(`(?i)\b((?:hf[_-]?token|token|api[_-]?key)\s*[:=]\s*)[^\s"'<>]+`)
	redactionBearerPattern      = regexp.MustCompile(`(?i)\b(authorization\s*:\s*bearer\s+)[^\s"'<>]+`)
	redactionWindowsPathPattern = regexp.MustCompile(`(?i)(?:[a-z]:[\\/]|\\\\)[^\s"'<>]+`)
	redactionUnixPathPattern    = regexp.MustCompile(`(?:/[^\s"'<>/]+){2,}`)
)

func BuildControlledJourneyReport(plan SealedPlan, observation JourneyObservation) (Report, error) {
	report := reportForPlan(plan)
	policy := observation.Redactions
	report.Journeys.Steps = redactJourneySteps(observation.Steps, policy)
	report.Commands = redactCommands(observation.Commands, policy)
	report.Readiness = redactReadiness(observation.Readiness, policy)
	report.Network = observation.Network
	report.Cleanup = observation.Cleanup
	if observation.InstalledIdentity != nil {
		report.Identities = append(report.Identities, sanitizeIdentity(*observation.InstalledIdentity))
	}

	if err := validateJourneyStepOrder(observation.Steps); err != nil {
		markJourneyIssue(&report, journeyFail, err.Check, err.Expected, err.Observed)
		report = redactReport(report, policy)
		report.Criteria = controlledCriteria(journeyFail, osSafetyEdge)
		return report, err
	}

	report.Journeys.Install = projectPhase(observation.Steps, JourneyStepInstall)
	report.Journeys.Discovery = projectPhase(observation.Steps, JourneyStepHelp, JourneyStepDocs, JourneyStepModelsList, JourneyStepModelsInspect)
	report.Journeys.Cold = projectPhase(observation.Steps, JourneyStepColdInvoke, JourneyStepColdReadiness)
	report.Journeys.WarmOffline = projectPhase(observation.Steps, JourneyStepWarmOfflineInvoke, JourneyStepWarmOfflineReadiness)
	report.Journeys.Removal = projectPhase(observation.Steps, JourneyStepRemove, JourneyStepPostRemove, JourneyStepCleanup)

	audio, audioIssues := projectAudio(observation.Audio)
	report.Audio = audio
	issues := append([]journeyIssue(nil), audioIssues...)
	issues = append(issues, validateJourneyEvidence(plan, observation, report)...)
	for _, issue := range issues {
		markJourneyIssue(&report, issue.Verdict, issue.Check, issue.Expected, issue.Observed)
	}
	report = redactReport(report, policy)

	probeVerdict := journeyPass
	for _, issue := range issues {
		if issue.Verdict == journeyFail {
			probeVerdict = journeyFail
			break
		}
		if issue.Verdict == journeyInconclusive {
			probeVerdict = journeyInconclusive
		}
	}
	report.Criteria = controlledCriteria(probeVerdict, osSafetyEdge)
	if probeVerdict == journeyFail {
		report.Verdict = journeyFail
	} else {
		// Controlled fixtures prove report mechanics only. Real product and
		// speech semantics remain explicitly unproven for the later gate.
		report.Verdict = journeyInconclusive
	}
	if err := ValidateReport(report); err != nil {
		return report, fmt.Errorf("controlled journey report: %w", err)
	}
	return report, nil
}

func validateJourneyStepOrder(steps []JourneyStep) *JourneyValidationError {
	if len(steps) != len(journeyStepOrder) {
		return &JourneyValidationError{
			Check: "journey-step-count", Expected: fmt.Sprintf("%d ordered public steps", len(journeyStepOrder)),
			Observed: fmt.Sprintf("%d steps", len(steps)),
		}
	}
	for index, step := range steps {
		if step.Name != journeyStepOrder[index] {
			return &JourneyValidationError{
				Check: "journey-step-order", Expected: strings.Join(journeyStepOrder, " > "),
				Observed: fmt.Sprintf("step %d is %q", index, step.Name),
			}
		}
		if step.Status != journeyPass && step.Status != journeyFail && step.Status != journeyInconclusive {
			return &JourneyValidationError{
				Check: "journey-step-status", Expected: "PASS, FAIL, or INCONCLUSIVE",
				Observed: step.Name + "=" + step.Status,
			}
		}
		if !hasEvidence(step.Evidence) {
			return &JourneyValidationError{
				Check: "journey-step-evidence", Expected: step.Name + " has evidence", Observed: "no evidence",
			}
		}
	}
	return nil
}

func hasEvidence(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func projectPhase(steps []JourneyStep, names ...string) PhaseEvidence {
	phase := PhaseEvidence{Status: journeyPass, Evidence: []string{}}
	for _, name := range names {
		for _, step := range steps {
			if step.Name != name {
				continue
			}
			phase.Evidence = append(phase.Evidence, step.Evidence...)
			if step.Status == journeyFail {
				phase.Status = journeyFail
			} else if step.Status == journeyInconclusive && phase.Status == journeyPass {
				phase.Status = journeyInconclusive
			}
		}
	}
	if phase.Status != journeyPass {
		phase.UnprovenEdge = "controlled journey evidence is incomplete"
	}
	return phase
}

type journeyIssue struct {
	Verdict  string
	Check    string
	Expected string
	Observed string
}

func validateJourneyEvidence(plan SealedPlan, observation JourneyObservation, report Report) []journeyIssue {
	issues := []journeyIssue{}
	if observation.InstalledIdentity == nil {
		issues = append(issues, journeyIssue{journeyInconclusive, "installed-identity", "an installed build identity", "identity evidence is missing"})
	} else if observation.InstalledIdentity.Identity != plan.Manifest.Build.Identity || observation.InstalledIdentity.SHA256 != plan.Manifest.Build.SHA256 {
		issues = append(issues, journeyIssue{journeyFail, "installed-identity-match", "installed identity and hash match the sealed build", "installed identity does not match the sealed artifact"})
	}
	if len(observation.Commands) == 0 {
		issues = append(issues, journeyIssue{journeyInconclusive, "commands", "projected public command evidence", "no commands recorded"})
	} else {
		for index, command := range observation.Commands {
			if command.Phase == "" || command.StartedAt == "" || command.EndedAt == "" || command.Argv == nil || !sha256Pattern.MatchString(command.StdoutSHA256) || !sha256Pattern.MatchString(command.StderrSHA256) || command.StdoutBytes < 0 || command.StderrBytes < 0 {
				issues = append(issues, journeyIssue{journeyFail, "command-evidence", "complete safe command evidence", fmt.Sprintf("command %d is incomplete", index)})
			}
		}
	}
	issues = append(issues, validateCommandProjection(observation.Commands)...)
	issues = append(issues, validateReadiness(observation.Readiness)...)
	issues = append(issues, validateNetwork(observation.Network)...)
	issues = append(issues, validateCleanup(observation.Cleanup)...)
	issues = append(issues, validateProjectedPhases(report.Journeys)...)
	return issues
}

func validateCommandProjection(commands []CommandEvidence) []journeyIssue {
	if len(commands) == 0 {
		return nil
	}
	seenCold, seenWarm := false, false
	issues := []journeyIssue{}
	for _, command := range commands {
		switch command.Phase {
		case JourneyStepColdInvoke:
			seenCold = true
			if commandHasFlag(command.Argv, "--offline") {
				issues = append(issues, journeyIssue{journeyFail, "cold-offline-flag", "cold invocation is distinct from warm --offline", "cold command included --offline"})
			}
		case JourneyStepWarmOfflineInvoke:
			seenWarm = true
			if !commandHasFlag(command.Argv, "--offline") {
				issues = append(issues, journeyIssue{journeyFail, "warm-offline-flag", "warm invocation includes --offline", "warm command omitted --offline"})
			}
		}
	}
	if !seenCold || !seenWarm {
		issues = append(issues, journeyIssue{journeyInconclusive, "invoke-command-projection", "separate cold and warm-offline commands", "cold or warm invoke command evidence is missing"})
	}
	return issues
}

func commandHasFlag(argv []string, flag string) bool {
	for _, arg := range argv {
		if strings.EqualFold(arg, flag) || strings.HasPrefix(strings.ToLower(arg), strings.ToLower(flag)+"=") {
			return true
		}
	}
	return false
}

func validateProjectedPhases(journeys JourneyEvidence) []journeyIssue {
	phases := []struct {
		name  string
		phase PhaseEvidence
	}{
		{"install", journeys.Install}, {"discovery", journeys.Discovery},
		{"cold", journeys.Cold}, {"warm-offline", journeys.WarmOffline},
		{"removal", journeys.Removal},
	}
	issues := []journeyIssue{}
	for _, item := range phases {
		if item.phase.Status == journeyFail {
			issues = append(issues, journeyIssue{journeyFail, item.name, "the public phase succeeds", "phase status is FAIL"})
		} else if item.phase.Status != journeyPass {
			issues = append(issues, journeyIssue{journeyInconclusive, item.name, "the public phase succeeds", "phase status is INCONCLUSIVE"})
		}
	}
	return issues
}

func validateReadiness(readiness []ReadinessEvidence) []journeyIssue {
	if len(readiness) == 0 {
		return []journeyIssue{{journeyInconclusive, "readiness", "cold and warm readiness/cache evidence", "readiness evidence is missing"}}
	}
	seen := map[string]ReadinessEvidence{}
	for _, item := range readiness {
		phase := strings.ToLower(strings.TrimSpace(item.Phase))
		if phase == "" || strings.TrimSpace(item.ReadinessState) == "" || strings.TrimSpace(item.LifecycleState) == "" || item.CacheBytes < 0 {
			return []journeyIssue{{journeyFail, "readiness", "complete readiness/cache evidence", "a readiness record is malformed"}}
		}
		if _, exists := seen[phase]; exists {
			return []journeyIssue{{journeyFail, "readiness-duplicate", "one readiness record per phase", phase}}
		}
		seen[phase] = item
	}
	cold, coldOK := seen["cold"]
	warm, warmOK := seen["warm-offline"]
	if !coldOK || !warmOK {
		return []journeyIssue{{journeyInconclusive, "readiness", "cold and warm-offline readiness records", "one or more readiness phases are missing"}}
	}
	if warm.CacheBytes <= 0 || !warm.CacheReused {
		return []journeyIssue{{journeyFail, "warm-cache-reuse", "warm-offline reuses a non-empty cache", fmt.Sprintf("cacheBytes=%d cacheReused=%t", warm.CacheBytes, warm.CacheReused)}}
	}
	if cold.CacheReused {
		return []journeyIssue{{journeyFail, "cold-cache-reuse", "cold invocation does not claim warm reuse", "cold cacheReused=true"}}
	}
	return nil
}

func validateNetwork(network NetworkEvidence) []journeyIssue {
	if network.Policy != "none" && network.Policy != "staged-loopback-and-declared-public-origins" {
		return []journeyIssue{{journeyFail, "network-policy", "a declared network policy", network.Policy}}
	}
	if network.Attempts < 0 || network.WarmOfflineAttempts < 0 {
		return []journeyIssue{{journeyFail, "network-counters", "non-negative network counters", fmt.Sprintf("attempts=%d warm=%d", network.Attempts, network.WarmOfflineAttempts)}}
	}
	if network.WarmOfflineAttempts > 0 {
		return []journeyIssue{{journeyFail, "warm-offline-network", "zero warm-offline network attempts", fmt.Sprintf("%d attempts", network.WarmOfflineAttempts)}}
	}
	if network.Attempts > 0 {
		return []journeyIssue{{journeyFail, "network-budget", "zero external network attempts in controlled preparation", fmt.Sprintf("%d attempts", network.Attempts)}}
	}
	return nil
}

func validateCleanup(cleanup CleanupEvidence) []journeyIssue {
	if cleanup.OwnedProcesses < 0 || cleanup.OwnedListeners < 0 || cleanup.OwnedRoots < 0 || cleanup.SurvivingProcesses < 0 || cleanup.SurvivingListeners < 0 || cleanup.RemovedRuntimeRoots < 0 {
		return []journeyIssue{{journeyFail, "cleanup-counters", "non-negative cleanup counters", "a cleanup counter is negative"}}
	}
	if cleanup.SurvivingProcesses != 0 || cleanup.SurvivingListeners != 0 {
		return []journeyIssue{{journeyFail, "cleanup-survivors", "zero surviving owned resources", fmt.Sprintf("processes=%d listeners=%d", cleanup.SurvivingProcesses, cleanup.SurvivingListeners)}}
	}
	if cleanup.OwnedRoots == 0 || cleanup.RemovedRuntimeRoots == 0 {
		return []journeyIssue{{journeyInconclusive, "cleanup", "owned roots and removal evidence", "cleanup counts are incomplete"}}
	}
	return nil
}

func projectAudio(artifacts []AudioArtifact) ([]AudioEvidence, []journeyIssue) {
	required := []string{"cold-tts.wav", "warm-offline-tts.wav"}
	byName := make(map[string]AudioArtifact, len(artifacts))
	issues := []journeyIssue{}
	for _, artifact := range artifacts {
		if _, exists := byName[artifact.Name]; exists {
			issues = append(issues, journeyIssue{journeyFail, "audio-duplicate", "one record per named audio output", artifact.Name})
			continue
		}
		byName[artifact.Name] = artifact
	}
	for _, artifact := range artifacts {
		if artifact.Name != required[0] && artifact.Name != required[1] {
			issues = append(issues, journeyIssue{journeyFail, "audio-name", "cold-tts.wav and warm-offline-tts.wav only", artifact.Name})
		}
	}
	records := make([]AudioEvidence, 0, len(required))
	for _, name := range required {
		artifact, ok := byName[name]
		if !ok {
			records = append(records, emptyAudioEvidence(name))
			issues = append(issues, journeyIssue{journeyInconclusive, "audio-missing", name + " semantic evidence", "audio artifact is missing"})
			continue
		}
		record, err := ParseWAV(name, artifact.MediaType, artifact.Data)
		records = append(records, record)
		if err != nil {
			issues = append(issues, journeyIssue{journeyFail, "audio-" + name, "decodable non-silent audio/wav", err.Error()})
		}
	}
	return records, issues
}

func ParseWAV(name, mediaType string, data []byte) (AudioEvidence, error) {
	record := AudioEvidence{Name: name, MediaType: mediaType, Bytes: int64(len(data)), SHA256: sha256Hex(data)}
	if name == "" {
		return record, &AudioValidationError{"invalid-name", "audio name is empty"}
	}
	if mediaType != "audio/wav" {
		return record, &AudioValidationError{"unsupported-media-type", "media type must be audio/wav"}
	}
	layout, err := decodePCM16WAV(data)
	if err != nil {
		return record, err
	}
	record.RIFFDecoded = true
	record.DurationMillis = layout.durationMillis
	record.NonSilent = layout.nonSilentSamples
	if record.DurationMillis <= 0 {
		return record, &AudioValidationError{"zero-duration", "audio duration is not positive"}
	}
	if record.NonSilent <= 0 {
		return record, &AudioValidationError{"silent", "audio contains no non-silent samples"}
	}
	return record, nil
}

type pcmWAVLayout struct {
	data             []byte
	channels         uint16
	bitsPerSample    uint16
	sampleRate       uint32
	blockAlign       uint16
	durationMillis   int64
	nonSilentSamples int64
}

func decodePCM16WAV(data []byte) (pcmWAVLayout, error) {
	if len(data) < 12 || !bytes.Equal(data[0:4], []byte("RIFF")) || !bytes.Equal(data[8:12], []byte("WAVE")) {
		return pcmWAVLayout{}, &AudioValidationError{"malformed", "missing RIFF/WAVE header"}
	}
	declaredLength := uint64(binary.LittleEndian.Uint32(data[4:8])) + 8
	if declaredLength != uint64(len(data)) {
		return pcmWAVLayout{}, &AudioValidationError{"truncated", fmt.Sprintf("RIFF length=%d, bytes=%d", declaredLength, len(data))}
	}
	var layout pcmWAVLayout
	seenFormat, seenData := false, false
	dataOffset := 0
	dataLength := 0
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return pcmWAVLayout{}, &AudioValidationError{"truncated", "chunk header is incomplete"}
		}
		chunkID := data[offset : offset+4]
		chunkLength := uint64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		chunkEnd := uint64(offset) + 8 + chunkLength
		if chunkEnd > uint64(len(data)) {
			return pcmWAVLayout{}, &AudioValidationError{"truncated", "chunk extends beyond RIFF bytes"}
		}
		chunk := data[offset+8 : int(chunkEnd)]
		switch string(chunkID) {
		case "fmt ":
			if seenFormat {
				return pcmWAVLayout{}, &AudioValidationError{"malformed", "multiple fmt chunks"}
			}
			if err := parsePCMFormat(chunk, &layout); err != nil {
				return pcmWAVLayout{}, err
			}
			seenFormat = true
		case "data":
			if seenData {
				return pcmWAVLayout{}, &AudioValidationError{"malformed", "multiple data chunks"}
			}
			dataOffset, dataLength, seenData = offset+8, len(chunk), true
		}
		paddedEnd := chunkEnd
		if chunkLength%2 == 1 {
			paddedEnd++
			if paddedEnd > uint64(len(data)) {
				return pcmWAVLayout{}, &AudioValidationError{"truncated", "odd-sized chunk is missing its pad byte"}
			}
		}
		offset = int(paddedEnd)
	}
	if !seenFormat || !seenData {
		return pcmWAVLayout{}, &AudioValidationError{"malformed", "fmt and data chunks are required"}
	}
	if dataLength == 0 {
		return pcmWAVLayout{}, &AudioValidationError{"zero-duration", "data chunk is empty"}
	}
	if uint64(dataLength)%uint64(layout.blockAlign) != 0 {
		return pcmWAVLayout{}, &AudioValidationError{"malformed", "data is not aligned to complete PCM frames"}
	}
	layout.data = data[dataOffset : dataOffset+dataLength]
	frames := uint64(dataLength / int(layout.blockAlign))
	if frames > (^uint64(0)-uint64(layout.sampleRate)+1)/1000 {
		return pcmWAVLayout{}, &AudioValidationError{"malformed", "duration overflows report range"}
	}
	duration := (frames*1000 + uint64(layout.sampleRate) - 1) / uint64(layout.sampleRate)
	if duration > uint64(^uint64(0)>>1) {
		return pcmWAVLayout{}, &AudioValidationError{"malformed", "duration overflows report range"}
	}
	layout.durationMillis = int64(duration)
	layout.nonSilentSamples = countNonSilentSamples(layout.data, layout.bitsPerSample)
	return layout, nil
}

func parsePCMFormat(chunk []byte, layout *pcmWAVLayout) error {
	if len(chunk) < 16 {
		return &AudioValidationError{"truncated", "fmt chunk is shorter than PCM format"}
	}
	format := binary.LittleEndian.Uint16(chunk[0:2])
	if format != 1 {
		return &AudioValidationError{"unsupported-format", fmt.Sprintf("PCM format code=%d", format)}
	}
	layout.channels = binary.LittleEndian.Uint16(chunk[2:4])
	layout.sampleRate = binary.LittleEndian.Uint32(chunk[4:8])
	layout.blockAlign = binary.LittleEndian.Uint16(chunk[12:14])
	layout.bitsPerSample = binary.LittleEndian.Uint16(chunk[14:16])
	if layout.channels == 0 || layout.sampleRate == 0 || layout.blockAlign == 0 {
		return &AudioValidationError{"malformed", "PCM channel, sample-rate, or block-align is zero"}
	}
	if layout.bitsPerSample != 8 && layout.bitsPerSample != 16 && layout.bitsPerSample != 24 && layout.bitsPerSample != 32 {
		return &AudioValidationError{"unsupported-format", fmt.Sprintf("bits per sample=%d", layout.bitsPerSample)}
	}
	wantAlign := uint64(layout.channels) * uint64(layout.bitsPerSample/8)
	if wantAlign != uint64(layout.blockAlign) || uint64(layout.sampleRate)*wantAlign != uint64(binary.LittleEndian.Uint32(chunk[8:12])) {
		return &AudioValidationError{"malformed", "PCM byte-rate or block-align is inconsistent"}
	}
	return nil
}

func countNonSilentSamples(data []byte, bitsPerSample uint16) int64 {
	width := int(bitsPerSample / 8)
	var count int64
	for offset := 0; offset+width <= len(data); offset += width {
		silent := true
		if bitsPerSample == 8 {
			silent = data[offset] == 128
		} else {
			for _, value := range data[offset : offset+width] {
				if value != 0 {
					silent = false
					break
				}
			}
		}
		if !silent {
			count++
		}
	}
	return count
}

func emptyAudioEvidence(name string) AudioEvidence {
	return AudioEvidence{Name: name, MediaType: "audio/wav", SHA256: sha256Hex(nil)}
}

func controlledCriteria(probeVerdict, preparationEdge string) []CriterionEvidence {
	return []CriterionEvidence{
		criterion("TTS-PROBE-01", journeyPass, "sealed immutable inputs and empty output root", nil),
		criterion("TTS-PROBE-02", probeVerdict, "controlled ordered journey/audio/report evidence", edgeFor(probeVerdict, preparationEdge)),
		criterion("TTS-PROBE-03", journeyInconclusive, "OS process and listener safety remains unrun", edgeFor(journeyInconclusive, osSafetyEdge)),
		criterion("LA-02", journeyInconclusive, "controlled evidence cannot prove a real source-blind TTS journey", edgeFor(journeyInconclusive, realTTSEdge)),
		criterion("LA-06", journeyInconclusive, "controlled audio bytes do not prove meaningful speech", edgeFor(journeyInconclusive, realTTSEdge)),
		criterion("LA-09", journeyInconclusive, "no platform conformance artifact was executed", edgeFor(journeyInconclusive, realTTSEdge)),
		criterion("LA-14", journeyInconclusive, "controlled evidence cannot prove an immutable installable build", edgeFor(journeyInconclusive, realTTSEdge)),
		criterion("LA-15", journeyInconclusive, "public clean-install behavior remains a later real gate", edgeFor(journeyInconclusive, realTTSEdge)),
	}
}

func criterion(id, verdict, evidence string, edge *string) CriterionEvidence {
	return CriterionEvidence{ID: id, Verdict: verdict, Evidence: []string{evidence}, UnprovenEdge: edge}
}

func edgeFor(verdict, edge string) *string {
	if verdict == journeyPass {
		none := ""
		return &none
	}
	return &edge
}

func markJourneyIssue(report *Report, verdict, check, expected, observed string) {
	if verdict == journeyFail {
		report.Verdict = journeyFail
	}
	report.Findings = append(report.Findings, Finding{
		ID: "TTS-PROBE-02", Severity: verdict, Check: check,
		Expected: expected, Observed: bounded(observed),
	})
}

func redactJourneySteps(steps []JourneyStep, policy RedactionValues) []JourneyStep {
	result := make([]JourneyStep, 0, len(steps))
	for _, step := range steps {
		result = append(result, JourneyStep{Name: redactText(step.Name, policy), Status: step.Status, Evidence: redactStrings(step.Evidence, policy)})
	}
	return result
}

func redactCommands(commands []CommandEvidence, policy RedactionValues) []CommandEvidence {
	result := make([]CommandEvidence, 0, len(commands))
	for _, command := range commands {
		argv := redactArgv(command.Argv, policy)
		result = append(result, CommandEvidence{
			Phase: redactText(command.Phase, policy), Argv: argv, ExitCode: command.ExitCode,
			StartedAt: command.StartedAt, EndedAt: command.EndedAt,
			StdoutBytes: command.StdoutBytes, StderrBytes: command.StderrBytes,
			StdoutSHA256: command.StdoutSHA256, StderrSHA256: command.StderrSHA256,
			TimedOut: command.TimedOut, Cancelled: command.Cancelled,
		})
	}
	return result
}

func redactReadiness(readiness []ReadinessEvidence, policy RedactionValues) []ReadinessEvidence {
	result := make([]ReadinessEvidence, 0, len(readiness))
	for _, item := range readiness {
		result = append(result, ReadinessEvidence{
			Phase: redactText(item.Phase, policy), ReadinessState: redactText(item.ReadinessState, policy),
			LifecycleState: redactText(item.LifecycleState, policy), CacheBytes: item.CacheBytes, CacheReused: item.CacheReused,
		})
	}
	return result
}

func sanitizeIdentity(identity Identity) Identity {
	if strings.Contains(identity.Path, `\`) || strings.Contains(identity.Path, "/") {
		identity.Path = pathIdentity(identity.Path)
	}
	return identity
}

func redactReport(report Report, policy RedactionValues) Report {
	report.Environment.Platform = redactText(report.Environment.Platform, policy)
	report.Environment.Architecture = redactText(report.Environment.Architecture, policy)
	report.Environment.Roots.Output = redactText(report.Environment.Roots.Output, policy)
	report.Environment.Roots.Work = redactText(report.Environment.Roots.Work, policy)
	report.Environment.Roots.Profile = redactText(report.Environment.Roots.Profile, policy)
	report.Environment.Roots.State = redactText(report.Environment.Roots.State, policy)
	report.Environment.Roots.Cache = redactText(report.Environment.Roots.Cache, policy)
	report.Environment.Roots.Temp = redactText(report.Environment.Roots.Temp, policy)
	report.Environment.Roots.Streams = redactText(report.Environment.Roots.Streams, policy)
	report.Environment.Roots.Runtime = redactText(report.Environment.Roots.Runtime, policy)
	report.Environment.Limits.NetworkPolicy = redactText(report.Environment.Limits.NetworkPolicy, policy)
	for index := range report.Preflight.Checks {
		report.Preflight.Checks[index].ID = redactText(report.Preflight.Checks[index].ID, policy)
		report.Preflight.Checks[index].Expected = redactText(report.Preflight.Checks[index].Expected, policy)
		report.Preflight.Checks[index].Observed = redactText(report.Preflight.Checks[index].Observed, policy)
	}
	for index := range report.Identities {
		report.Identities[index] = sanitizeIdentity(report.Identities[index])
		report.Identities[index].Name = redactText(report.Identities[index].Name, policy)
		report.Identities[index].Identity = redactText(report.Identities[index].Identity, policy)
	}
	report.Commands = redactCommands(report.Commands, policy)
	report.Journeys.Steps = redactJourneySteps(report.Journeys.Steps, policy)
	for _, phase := range []*PhaseEvidence{&report.Journeys.Install, &report.Journeys.Discovery, &report.Journeys.Cold, &report.Journeys.WarmOffline, &report.Journeys.Removal} {
		phase.Evidence = redactStrings(phase.Evidence, policy)
		phase.UnprovenEdge = redactText(phase.UnprovenEdge, policy)
	}
	for index := range report.Audio {
		report.Audio[index].Name = redactText(report.Audio[index].Name, policy)
		report.Audio[index].MediaType = redactText(report.Audio[index].MediaType, policy)
	}
	for index := range report.Readiness {
		report.Readiness[index] = redactReadiness([]ReadinessEvidence{report.Readiness[index]}, policy)[0]
	}
	report.Network.Policy = redactText(report.Network.Policy, policy)
	for index := range report.Criteria {
		report.Criteria[index].ID = redactText(report.Criteria[index].ID, policy)
		report.Criteria[index].Evidence = redactStrings(report.Criteria[index].Evidence, policy)
		if report.Criteria[index].UnprovenEdge != nil {
			value := redactText(*report.Criteria[index].UnprovenEdge, policy)
			report.Criteria[index].UnprovenEdge = &value
		}
	}
	for index := range report.Findings {
		report.Findings[index].ID = redactText(report.Findings[index].ID, policy)
		report.Findings[index].Severity = redactText(report.Findings[index].Severity, policy)
		report.Findings[index].Check = redactText(report.Findings[index].Check, policy)
		report.Findings[index].Expected = redactText(report.Findings[index].Expected, policy)
		report.Findings[index].Observed = redactText(report.Findings[index].Observed, policy)
	}
	return report
}

func MarshalReport(report Report) ([]byte, error) {
	report = redactReport(report, RedactionValues{})
	if err := ValidateReport(report); err != nil {
		return nil, err
	}
	return marshalReportJSON(report)
}

func marshalReportJSON(report Report) ([]byte, error) {
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

func redactStrings(values []string, policy RedactionValues) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, redactText(value, policy))
	}
	return result
}

func redactArgument(value string, policy RedactionValues) string {
	value = redactText(value, policy)
	separator := strings.IndexByte(value, '=')
	if separator < 0 {
		return value
	}
	key := value[:separator]
	normalized := strings.ToLower(strings.TrimLeft(key, "-"))
	switch {
	case strings.Contains(normalized, "token") || strings.Contains(normalized, "api-key") || strings.Contains(normalized, "apikey"):
		return key + "=<redacted-token>"
	case normalized == "text" || normalized == "prompt" || normalized == "content" || normalized == "raw-text":
		return key + "=<redacted-text>"
	case normalized == "voice" || normalized == "voice-marker":
		return key + "=<redacted-voice>"
	case normalized == "path" || normalized == "output" || normalized == "artifact" || normalized == "manifest" || normalized == "report" || normalized == "home" || normalized == "profile" || normalized == "cache":
		return key + "=<redacted-path>"
	}
	return value
}

func redactArgv(argv []string, policy RedactionValues) []string {
	result := make([]string, 0, len(argv))
	for index := 0; index < len(argv); index++ {
		argument := argv[index]
		result = append(result, redactArgument(argument, policy))
		if index+1 >= len(argv) {
			continue
		}
		switch strings.ToLower(argument) {
		case "--text", "--prompt", "--content":
			result = append(result, "<redacted-text>")
			index++
		case "--voice", "--voice-marker":
			result = append(result, "<redacted-voice>")
			index++
		case "--token", "--api-key", "--apikey":
			result = append(result, "<redacted-token>")
			index++
		}
	}
	return result
}

func redactText(value string, policy RedactionValues) string {
	value = replaceRedactions(value, policy.Tokens, "<redacted-token>")
	value = replaceRedactions(value, policy.SignedURLs, "<redacted-url>")
	value = replaceRedactions(value, policy.AmbientPaths, "<redacted-path>")
	value = replaceRedactions(value, policy.RawText, "<redacted-text>")
	value = replaceRedactions(value, policy.VoiceMarkers, "<redacted-voice>")
	value = redactionTokenPattern.ReplaceAllString(value, `${1}<redacted-token>`)
	value = redactionBearerPattern.ReplaceAllString(value, `${1}<redacted-token>`)
	value = redactionURLPattern.ReplaceAllString(value, `<redacted-url>`)
	value = redactionWindowsPathPattern.ReplaceAllString(value, `<redacted-path>`)
	value = redactionUnixPathPattern.ReplaceAllString(value, `<redacted-path>`)
	return value
}

func replaceRedactions(value string, sensitive []string, replacement string) string {
	for _, candidate := range sensitive {
		if candidate != "" {
			value = strings.ReplaceAll(value, candidate, replacement)
		}
	}
	return value
}

func validatePassReport(report Report) error {
	if report.Preflight.ChildStarts != 0 || report.Preflight.ListenerOpens != 0 || report.Preflight.NetworkAttempts != 0 {
		return errors.New("PASS report crossed an effect boundary before preflight completed")
	}
	if len(report.Journeys.Steps) != len(journeyStepOrder) {
		return errors.New("PASS report must contain every ordered journey step")
	}
	if err := validateJourneyStepOrder(report.Journeys.Steps); err != nil {
		return err
	}
	for _, phase := range []PhaseEvidence{report.Journeys.Install, report.Journeys.Discovery, report.Journeys.Cold, report.Journeys.WarmOffline, report.Journeys.Removal} {
		if phase.Status != journeyPass || len(phase.Evidence) == 0 {
			return errors.New("PASS report has an incomplete journey phase")
		}
	}
	if len(report.Commands) == 0 {
		return errors.New("PASS report must contain command evidence")
	}
	for _, command := range report.Commands {
		if command.Phase == "" || command.StartedAt == "" || command.EndedAt == "" || command.Argv == nil || command.TimedOut || command.Cancelled || command.ExitCode != 0 {
			return errors.New("PASS report has incomplete command evidence")
		}
	}
	if issues := validateCommandProjection(report.Commands); len(issues) != 0 {
		return errors.New("PASS report has incomplete cold/warm command projection")
	}
	seenAudio := map[string]bool{}
	for _, audio := range report.Audio {
		if seenAudio[audio.Name] || (audio.Name != "cold-tts.wav" && audio.Name != "warm-offline-tts.wav") {
			return errors.New("PASS report audio names are not the two required outputs")
		}
		seenAudio[audio.Name] = true
		if audio.MediaType != "audio/wav" || audio.Bytes <= 0 || !audio.RIFFDecoded || audio.DurationMillis <= 0 || audio.NonSilent <= 0 {
			return errors.New("PASS report requires valid non-silent RIFF audio")
		}
	}
	if !seenAudio["cold-tts.wav"] || !seenAudio["warm-offline-tts.wav"] {
		return errors.New("PASS report must contain cold and warm-offline audio")
	}
	readinessIssues := validateReadiness(report.Readiness)
	if len(readinessIssues) != 0 {
		return errors.New("PASS report has incomplete readiness or cache reuse evidence")
	}
	if report.Network.WarmOfflineAttempts != 0 || report.Network.Attempts != 0 {
		return errors.New("PASS report cannot contain network attempts")
	}
	if report.Cleanup.OwnedRoots == 0 || report.Cleanup.SurvivingProcesses != 0 || report.Cleanup.SurvivingListeners != 0 || report.Cleanup.RemovedRuntimeRoots == 0 {
		return errors.New("PASS report has incomplete cleanup evidence")
	}
	return nil
}
