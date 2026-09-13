package tts_clean_install

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
)

func TestU05ControlledJourneyProjectsOrderedSemanticEvidence(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}

	observation := completeJourneyObservation(t)
	report, err := BuildControlledJourneyReport(preflight.Plan, observation)
	if err != nil {
		t.Fatalf("BuildControlledJourneyReport: %v", err)
	}
	if report.Verdict != journeyInconclusive {
		t.Fatalf("controlled report verdict = %q, want INCONCLUSIVE for the real TTS edge", report.Verdict)
	}
	for index, step := range report.Journeys.Steps {
		if step.Name != journeyStepOrder[index] || step.Status != journeyPass || len(step.Evidence) == 0 {
			t.Fatalf("step %d = %#v, want ordered PASS evidence", index, step)
		}
	}
	for name, phase := range map[string]PhaseEvidence{
		"install": report.Journeys.Install, "discovery": report.Journeys.Discovery,
		"cold": report.Journeys.Cold, "warm-offline": report.Journeys.WarmOffline,
		"removal": report.Journeys.Removal,
	} {
		if phase.Status != journeyPass || len(phase.Evidence) == 0 {
			t.Fatalf("%s phase = %#v, want complete PASS evidence", name, phase)
		}
	}
	if len(report.Audio) != 2 {
		t.Fatalf("audio records = %d, want two named outputs", len(report.Audio))
	}
	for _, audio := range report.Audio {
		if audio.MediaType != "audio/wav" || audio.Bytes <= 0 || !audio.RIFFDecoded || audio.DurationMillis <= 0 || audio.NonSilent <= 0 {
			t.Fatalf("audio = %#v, want positive non-silent RIFF semantics", audio)
		}
	}
	if report.Audio[0].SHA256 != sha256Hex(observation.Audio[0].Data) || report.Audio[1].SHA256 != sha256Hex(observation.Audio[1].Data) {
		t.Fatal("audio report hashes do not match the exact output bytes")
	}
	if report.Readiness[1].Phase != "warm-offline" || !report.Readiness[1].CacheReused || report.Readiness[1].CacheBytes <= 0 {
		t.Fatalf("warm readiness = %#v, want cache reuse evidence", report.Readiness[1])
	}
	if report.Network.WarmOfflineAttempts != 0 || report.Cleanup.SurvivingProcesses != 0 || report.Cleanup.SurvivingListeners != 0 {
		t.Fatalf("network/cleanup evidence = %#v / %#v, want offline and zero survivors", report.Network, report.Cleanup)
	}
	for _, id := range []string{"LA-02", "LA-06", "LA-09", "LA-14", "LA-15"} {
		if got := criterionForTest(report, id).Verdict; got == journeyPass {
			t.Fatalf("controlled evidence marked %s PASS", id)
		}
	}
	if err := ValidateReport(report); err != nil {
		t.Fatalf("controlled report validation: %v", err)
	}
}

func TestU05InvalidAudioBlocksJourneyPass(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data []byte
		mime string
	}{
		{name: "malformed header", data: []byte("not wav"), mime: "audio/wav"},
		{name: "truncated", data: func() []byte {
			valid := makePCM16WAV([]int16{1, 2, 3}, 8000)
			return valid[:len(valid)-1]
		}(), mime: "audio/wav"},
		{name: "unsupported format", data: unsupportedWAV(), mime: "audio/wav"},
		{name: "zero duration", data: makePCM16WAV(nil, 8000), mime: "audio/wav"},
		{name: "silent", data: makePCM16WAV([]int16{0, 0, 0}, 8000), mime: "audio/wav"},
		{name: "unsupported media", data: makePCM16WAV([]int16{1, 2}, 8000), mime: "audio/ogg"},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			preflight, err := Preflight(f.invocation)
			if err != nil {
				t.Fatalf("Preflight: %v", err)
			}
			observation := completeJourneyObservation(t)
			observation.Audio[0] = AudioArtifact{Name: "cold-tts.wav", MediaType: testCase.mime, Data: testCase.data}
			report, err := BuildControlledJourneyReport(preflight.Plan, observation)
			if err != nil {
				t.Fatalf("BuildControlledJourneyReport: %v", err)
			}
			if report.Verdict != journeyFail {
				t.Fatalf("report verdict = %q, want FAIL", report.Verdict)
			}
			if err := ValidateReport(report); err != nil {
				t.Fatalf("failure report validation: %v", err)
			}
			if criterionForTest(report, "LA-06").Verdict == journeyPass {
				t.Fatal("invalid controlled audio marked real semantic acceptance PASS")
			}
		})
	}
}

func TestU05JourneyRejectsOutOfOrderSteps(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	observation := completeJourneyObservation(t)
	observation.Steps[2], observation.Steps[3] = observation.Steps[3], observation.Steps[2]
	report, err := BuildControlledJourneyReport(preflight.Plan, observation)
	if err == nil {
		t.Fatal("BuildControlledJourneyReport accepted out-of-order public steps")
	}
	if report.Verdict != journeyFail {
		t.Fatalf("out-of-order report verdict = %q, want FAIL", report.Verdict)
	}
	if err := ValidateReport(report); err != nil {
		t.Fatalf("out-of-order failure report validation: %v", err)
	}
}

func TestU07WarmOfflineAndReportRedactionAreFailClosed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	const (
		token       = "token-secret-001"
		signedURL   = "https://example.invalid/model?X-Amz-Signature=signed-secret"
		ambientPath = `C:\Users\operator\profile\cache`
		rawText     = "Read the release summary exactly"
		voiceMarker = "voice-marker-secret"
	)
	observation := completeJourneyObservation(t)
	observation.Network.WarmOfflineAttempts = 1
	observation.Redactions = RedactionValues{
		Tokens: []string{token}, SignedURLs: []string{signedURL},
		AmbientPaths: []string{ambientPath}, RawText: []string{rawText},
		VoiceMarkers: []string{voiceMarker},
	}
	observation.Commands[0].Argv = []string{
		"you", "text=" + rawText, "voice=" + voiceMarker,
		"token=" + token, "url=" + signedURL, "path=" + ambientPath,
	}
	observation.Steps[0].Evidence = []string{rawText + " " + voiceMarker + " " + signedURL + " " + ambientPath}

	report, err := BuildControlledJourneyReport(preflight.Plan, observation)
	if err != nil {
		t.Fatalf("BuildControlledJourneyReport: %v", err)
	}
	if report.Verdict != journeyFail {
		t.Fatalf("warm-offline report verdict = %q, want FAIL", report.Verdict)
	}
	body, err := MarshalReport(report)
	if err != nil {
		t.Fatalf("MarshalReport: %v", err)
	}
	for _, secret := range []string{token, signedURL, ambientPath, rawText, voiceMarker} {
		if bytes.Contains(body, []byte(secret)) {
			t.Fatalf("report leaked sensitive value %q: %s", secret, body)
		}
	}
	if !bytes.Contains(body, []byte(`"stdoutBytes"`)) || !bytes.Contains(body, []byte(`"stdoutSha256"`)) || !bytes.Contains(body, []byte("redacted-token")) {
		t.Fatalf("report lost safe command facts or redaction classification: %s", body)
	}
	if err := ValidateReport(report); err != nil {
		t.Fatalf("redacted failure report validation: %v", err)
	}
}

func TestU07WarmOfflineRequiresSeparateOfflineCommand(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	preflight, err := Preflight(f.invocation)
	if err != nil {
		t.Fatalf("Preflight: %v", err)
	}
	observation := completeJourneyObservation(t)
	for index := range observation.Commands {
		if observation.Commands[index].Phase == JourneyStepWarmOfflineInvoke {
			observation.Commands[index].Argv = []string{"you", "models", "invoke"}
		}
	}
	report, err := BuildControlledJourneyReport(preflight.Plan, observation)
	if err != nil {
		t.Fatalf("BuildControlledJourneyReport: %v", err)
	}
	if report.Verdict != journeyFail {
		t.Fatalf("warm command report verdict = %q, want FAIL", report.Verdict)
	}
	if !findingForTest(report, "warm-offline-flag") {
		t.Fatalf("report findings = %#v, want warm-offline-flag", report.Findings)
	}
}

func TestU07PassRequiresCompleteJourneyEvidence(t *testing.T) {
	t.Parallel()
	report := newReport()
	report.Verdict = journeyPass
	if err := ValidateReport(report); err == nil {
		t.Fatal("ValidateReport accepted an incomplete PASS report")
	}
}

func completeJourneyObservation(t *testing.T) JourneyObservation {
	t.Helper()
	steps := make([]JourneyStep, 0, len(journeyStepOrder))
	for _, name := range journeyStepOrder {
		steps = append(steps, JourneyStep{Name: name, Status: journeyPass, Evidence: []string{name + " observed"}})
	}
	commands := make([]CommandEvidence, 0, len(journeyStepOrder))
	for _, name := range journeyStepOrder {
		argv := []string{"you", name}
		if name == JourneyStepWarmOfflineInvoke {
			argv = append(argv, "--offline")
		}
		commands = append(commands, CommandEvidence{
			Phase: name, Argv: argv, ExitCode: 0,
			StartedAt: "2026-09-13T07:00:00Z", EndedAt: "2026-09-13T07:00:01Z",
			StdoutBytes: 12, StderrBytes: 0,
			StdoutSHA256: sha256Hex([]byte("safe stdout")), StderrSHA256: sha256Hex(nil),
		})
	}
	return JourneyObservation{
		Steps: steps,
		InstalledIdentity: &Identity{
			Name: "installed-build", Identity: "build-test-001",
			Path: "sha256:" + strings.Repeat("1", 64), Bytes: 64,
			SHA256: sha256Hex([]byte("fake immutable executable\n")),
		},
		Commands: commands,
		Audio: []AudioArtifact{
			{Name: "cold-tts.wav", MediaType: "audio/wav", Data: makePCM16WAV([]int16{1, 2, 3, 4, 5, 6, 7, 8}, 8000)},
			{Name: "warm-offline-tts.wav", MediaType: "audio/wav", Data: makePCM16WAV([]int16{8, 7, 6, 5, 4, 3, 2, 1}, 8000)},
		},
		Readiness: []ReadinessEvidence{
			{Phase: "cold", ReadinessState: "READY", LifecycleState: "LOADED", CacheBytes: 0, CacheReused: false},
			{Phase: "warm-offline", ReadinessState: "READY", LifecycleState: "LOADED", CacheBytes: 128, CacheReused: true},
		},
		Network: NetworkEvidence{Policy: "none"},
		Cleanup: CleanupEvidence{OwnedRoots: 7, RemovedRuntimeRoots: 7},
	}
}

func makePCM16WAV(samples []int16, sampleRate uint32) []byte {
	data := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(data[index*2:], uint16(sample))
	}
	result := make([]byte, 12, 12+8+16+8+len(data))
	copy(result[0:4], "RIFF")
	copy(result[8:12], "WAVE")
	appendChunk := func(name string, body []byte) {
		result = append(result, name...)
		length := make([]byte, 4)
		binary.LittleEndian.PutUint32(length, uint32(len(body)))
		result = append(result, length...)
		result = append(result, body...)
		if len(body)%2 == 1 {
			result = append(result, 0)
		}
	}
	fmtChunk := make([]byte, 16)
	binary.LittleEndian.PutUint16(fmtChunk[0:2], 1)
	binary.LittleEndian.PutUint16(fmtChunk[2:4], 1)
	binary.LittleEndian.PutUint32(fmtChunk[4:8], sampleRate)
	binary.LittleEndian.PutUint32(fmtChunk[8:12], sampleRate*2)
	binary.LittleEndian.PutUint16(fmtChunk[12:14], 2)
	binary.LittleEndian.PutUint16(fmtChunk[14:16], 16)
	appendChunk("fmt ", fmtChunk)
	appendChunk("data", data)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}

func unsupportedWAV() []byte {
	result := makePCM16WAV([]int16{1, 2}, 8000)
	binary.LittleEndian.PutUint16(result[20:22], 3)
	return result
}

func criterionForTest(report Report, id string) CriterionEvidence {
	for _, criterion := range report.Criteria {
		if criterion.ID == id {
			return criterion
		}
	}
	return CriterionEvidence{ID: id, Verdict: "MISSING", Evidence: []string{fmt.Sprintf("%s missing", id)}}
}

func findingForTest(report Report, check string) bool {
	for _, finding := range report.Findings {
		if finding.Check == check {
			return true
		}
	}
	return false
}
