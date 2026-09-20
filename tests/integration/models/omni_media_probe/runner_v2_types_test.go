package omni_media_probe

import (
	"context"

	"github.com/portpowered/infinite-you/tests/internal/localai/corpusv2"
)

const (
	ProbeInputSchemaV2  = "you.localai.omni-video-corpus-runner-input.v2"
	ProbeReportSchemaV2 = "you.localai.omni-video-corpus-runner-report.v2"

	ProbeV2MaxHeavyProcesses        int64 = 1
	ProbeV2MaxCompilerTestProcesses int64 = 4
	ProbeV2MaxDiskBytes             int64 = 3 << 30
	ProbeV2PerCallTimeoutSeconds    int64 = 180
	ProbeV2MaxCalls                 int64 = 10
	ProbeV2MaxRetries               int64 = 0
	ProbeV2CorpusInputMaxDiskBytes  int64 = 1 << 30
)

const (
	CodeProbeCorpusAuthority ValidationCode = "probe_corpus_authority"
	CodeProbeCorpusPolicy    ValidationCode = "probe_corpus_policy"
)

// ProbeInputV2 is the private, strict admission contract for the corpus
// runner. It is intentionally separate from the retired fixture-based v1
// contract while both implementations remain available during the handoff.
type ProbeInputV2 struct {
	SchemaVersion string             `json:"schemaVersion"`
	RunID         string             `json:"runId"`
	Build         ProbeBuildIdentity `json:"build"`
	Dependencies  ProbeDependencies  `json:"dependencies"`
	CorpusInput   ProbeFileIdentity  `json:"corpusInput"`
	ProbeRoot     string             `json:"probeRoot"`
	Limits        ProbeLimitsV2      `json:"limits"`
}

type ProbeLimitsV2 struct {
	PerCallTimeoutSeconds    int64   `json:"perCallTimeoutSeconds"`
	MaxHeavyProcesses        int64   `json:"maxHeavyProcesses"`
	MaxCompilerTestProcesses int64   `json:"maxCompilerTestProcesses"`
	MaxDiskBytes             int64   `json:"maxDiskBytes"`
	MaxDownloadBytes         int64   `json:"maxDownloadBytes"`
	MaxPaidUSD               float64 `json:"maxPaidUsd"`
	MaxCalls                 int64   `json:"maxCalls"`
	MaxRetries               int64   `json:"maxRetries"`
	ForbiddenPort            int     `json:"forbiddenPort"`
	NetworkPolicy            string  `json:"networkPolicy"`
}

// CorpusInputV2 is the identity-bearing file referenced by ProbeInputV2.
// Its corpus and sample policy fields must match the single shared authority.
type CorpusInputV2 struct {
	SchemaVersion string               `json:"schemaVersion"`
	RunID         string               `json:"runId"`
	Build         ProbeFileIdentity    `json:"build"`
	Dependencies  ProbeDependencies    `json:"dependencies"`
	Corpus        CorpusAuthorityV2    `json:"corpus"`
	SamplePolicy  CorpusSamplePolicyV2 `json:"samplePolicy"`
	Limits        CorpusInputLimitsV2  `json:"limits"`
}

type CorpusAuthorityV2 struct {
	Repository  string `json:"repository"`
	Commit      string `json:"commit"`
	IndexPath   string `json:"indexPath"`
	IndexSHA256 string `json:"indexSha256"`
	Mode        string `json:"mode"`
}

type CorpusSamplePolicyV2 struct {
	Studies                 []string `json:"studies"`
	RepresentativesPerStudy int      `json:"representativesPerStudy"`
	Ordering                string   `json:"ordering"`
}

type CorpusInputLimitsV2 struct {
	PerInvocationTimeoutSeconds int64   `json:"perInvocationTimeoutSeconds"`
	Retries                     int64   `json:"retries"`
	MaxHeavyProcesses           int64   `json:"maxHeavyProcesses"`
	MaxDiskBytes                int64   `json:"maxDiskBytes"`
	MaxDownloadBytes            int64   `json:"maxDownloadBytes"`
	MaxPaidUSD                  float64 `json:"maxPaidUsd"`
	ForbiddenPort               int     `json:"forbiddenPort"`
	NetworkPolicy               string  `json:"networkPolicy"`
}

type ProbeReportV2 struct {
	SchemaVersion string                  `json:"schemaVersion"`
	RunID         string                  `json:"runId"`
	Mode          string                  `json:"mode"`
	Status        string                  `json:"status"`
	Build         RecordedIdentity        `json:"build"`
	Dependencies  ProbeReportDependencies `json:"dependencies"`
	Corpus        CorpusReportV2          `json:"corpus"`
	Policy        ProbePolicyV2           `json:"policy"`
	Calls         []ProbeCallReportV2     `json:"calls"`
	Outputs       []RecordedIdentity      `json:"outputs"`
	Processes     []ProcessEvidence       `json:"processes"`
	Cleanup       CleanupEvidence         `json:"cleanup"`
	Failure       *ReportFailure          `json:"failure"`
}

type CorpusReportV2 struct {
	RepositoryIdentity string                 `json:"repositoryIdentity"`
	Commit             string                 `json:"commit"`
	IndexPathIdentity  string                 `json:"indexPathIdentity"`
	IndexSHA256        string                 `json:"indexSha256"`
	UniqueClips        int                    `json:"uniqueClips"`
	UniquePrompts      int                    `json:"uniquePrompts"`
	SelectedSamples    []CorpusSampleReportV2 `json:"selectedSamples"`
	CopiedBytes        int64                  `json:"copiedBytes"`
	UploadedBytes      int64                  `json:"uploadedBytes"`
	ReadOnly           bool                   `json:"readOnly"`
}

type CorpusSampleReportV2 struct {
	Study        string               `json:"study"`
	Band         string               `json:"band"`
	Attempt      string               `json:"attempt"`
	Clip         RecordedIdentity     `json:"clip"`
	Prompt       RecordedIdentity     `json:"prompt"`
	Stream       CorpusStreamReportV2 `json:"stream"`
	SourceCommit string               `json:"sourceCommit"`
}

type CorpusStreamReportV2 struct {
	Codec          string `json:"codec"`
	Width          int    `json:"width"`
	Height         int    `json:"height"`
	FrameRate      string `json:"frameRate"`
	DurationMillis int64  `json:"durationMillis"`
	Frames         int64  `json:"frames"`
}

type ProbePolicyV2 struct {
	RootIdentities        []string `json:"rootIdentities"`
	Port                  int      `json:"port"`
	PerCallTimeoutSeconds int64    `json:"perCallTimeoutSeconds"`
	NetworkPolicy         string   `json:"networkPolicy"`
	DownloadBytes         int64    `json:"downloadBytes"`
	PaidUSD               float64  `json:"paidUsd"`
	MaxHeavyProcesses     int64    `json:"maxHeavyProcesses"`
	MaxCalls              int64    `json:"maxCalls"`
	MaxRetries            int64    `json:"maxRetries"`
}

type ProbeCallReportV2 struct {
	Ordinal        int                `json:"ordinal"`
	Kind           string             `json:"kind"`
	SampleIdentity *string            `json:"sampleIdentity"`
	Status         string             `json:"status"`
	Command        []string           `json:"command"`
	Inputs         []RecordedIdentity `json:"inputs"`
	Output         *RecordedIdentity  `json:"output"`
	Failure        *ReportFailure     `json:"failure"`
}

type corpusV2ManifestReader func(context.Context, corpusv2.CorpusV2Authority) (corpusv2.CorpusV2Manifest, error)

// RunnerV2 admits and records the exact corpus before any executor call.
// CorpusReader replaces only the local corpus observation boundary in tests.
type RunnerV2 struct {
	Executor     Executor
	corpusReader corpusV2ManifestReader
}

func NewRunnerV2(executor Executor) RunnerV2 {
	return RunnerV2{Executor: executor}
}
