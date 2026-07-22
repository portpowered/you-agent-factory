package factorydefinitionfixtures

import (
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

// InvocationInterpolation is a programmable Factory Definitions root fake.
// Consumer tests script the owning service's result without importing or
// reproducing its interpolation implementation.
type InvocationInterpolation struct {
	Validate               func(*factorydefinitions.FactoryConfig, *work.InvocationArguments, factorydefinitions.FileReader) error
	InterpolateWorker      func(factorydefinitions.FactoryWorkerConfig, *work.InvocationArguments, factorydefinitions.FileReader) (factorydefinitions.FactoryWorkerConfig, error)
	InterpolateWorkstation func(factorydefinitions.FactoryWorkstationConfig, *work.InvocationArguments, factorydefinitions.FileReader) (factorydefinitions.FactoryWorkstationConfig, error)
}

func (i InvocationInterpolation) ValidateInvocationInterpolation(
	cfg *factorydefinitions.FactoryConfig,
	arguments *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) error {
	if i.Validate == nil {
		return nil
	}
	return i.Validate(cfg, arguments, readFile)
}

func (i InvocationInterpolation) InterpolateWorkerConfig(
	worker factorydefinitions.FactoryWorkerConfig,
	arguments *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) (factorydefinitions.FactoryWorkerConfig, error) {
	if i.InterpolateWorker == nil {
		return worker, nil
	}
	return i.InterpolateWorker(worker, arguments, readFile)
}

func (i InvocationInterpolation) InterpolateWorkstationConfig(
	workstation factorydefinitions.FactoryWorkstationConfig,
	arguments *work.InvocationArguments,
	readFile factorydefinitions.FileReader,
) (factorydefinitions.FactoryWorkstationConfig, error) {
	if i.InterpolateWorkstation == nil {
		return workstation, nil
	}
	return i.InterpolateWorkstation(workstation, arguments, readFile)
}

// WorkstationExecutionPolicy is a programmable Factory Definitions root fake.
// Consumer tests script resolved durations or failures without reproducing
// Factory Definitions parsing and normalization policy.
type WorkstationExecutionPolicy struct {
	Resolve func(*factorydefinitions.FactoryWorkstationConfig) (time.Duration, error)
}

func (p WorkstationExecutionPolicy) ExecutionTimeout(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) (time.Duration, error) {
	if p.Resolve == nil {
		return 0, nil
	}
	return p.Resolve(workstation)
}

// InvocationOutputShaping is a programmable Factory Definitions root fake.
// Nil callbacks select no shaping, which is useful for consumers whose
// scenario never reaches packaged-output policy.
type InvocationOutputShaping struct {
	FormatSummary      func(*factorydefinitions.FactoryWorkstationConfig) bool
	SummaryContent     func(string, string) ([]work.WorkContentPart, error)
	FormatResponse     func(*factorydefinitions.FactoryWorkstationConfig) bool
	ResponseContent    func(string, string) ([]work.WorkContentPart, error)
	FormatTTS          func(*factorydefinitions.FactoryWorkstationConfig) bool
	TTSBackendLabel    func(*factorydefinitions.FactoryWorkerConfig) string
	TTSMetadataContent func(string, string, string, string) ([]work.WorkContentPart, error)
}

func (s InvocationOutputShaping) ShouldFormatInvocationSummary(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) bool {
	return s.FormatSummary != nil && s.FormatSummary(workstation)
}

func (s InvocationOutputShaping) SummaryContentFromWorkerOutput(
	output string,
	stopToken string,
) ([]work.WorkContentPart, error) {
	if s.SummaryContent == nil {
		return nil, nil
	}
	return s.SummaryContent(output, stopToken)
}

func (s InvocationOutputShaping) ShouldFormatInvocationResponse(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) bool {
	return s.FormatResponse != nil && s.FormatResponse(workstation)
}

func (s InvocationOutputShaping) ResponseContentFromWorkerOutput(
	output string,
	stopToken string,
) ([]work.WorkContentPart, error) {
	if s.ResponseContent == nil {
		return nil, nil
	}
	return s.ResponseContent(output, stopToken)
}

func (s InvocationOutputShaping) ShouldFormatTTSInvocationMetadata(
	workstation *factorydefinitions.FactoryWorkstationConfig,
) bool {
	return s.FormatTTS != nil && s.FormatTTS(workstation)
}

func (s InvocationOutputShaping) TTSBackendLabelFromWorker(
	worker *factorydefinitions.FactoryWorkerConfig,
) string {
	if s.TTSBackendLabel == nil {
		return ""
	}
	return s.TTSBackendLabel(worker)
}

func (s InvocationOutputShaping) TTSMetadataContentFromWorkerOutput(
	output string,
	traceID string,
	sessionID string,
	backendLabel string,
) ([]work.WorkContentPart, error) {
	if s.TTSMetadataContent == nil {
		return nil, nil
	}
	return s.TTSMetadataContent(output, traceID, sessionID, backendLabel)
}

// QuorumPolicy is a programmable Factory Definitions root fake.
type QuorumPolicy struct {
	IsPackaged func(*factorydefinitions.FactoryConfig) bool
	Relations  func(string, string, string, []factorydefinitions.QuorumLineageInput) []work.Relation
}

func (p QuorumPolicy) IsPackagedQuorumFactory(
	cfg *factorydefinitions.FactoryConfig,
) bool {
	return p.IsPackaged != nil && p.IsPackaged(cfg)
}

func (p QuorumPolicy) WorkRelations(
	workstationName string,
	outputParentID string,
	outputWorkTypeID string,
	inputs []factorydefinitions.QuorumLineageInput,
) []work.Relation {
	if p.Relations == nil {
		return nil
	}
	return p.Relations(workstationName, outputParentID, outputWorkTypeID, inputs)
}
