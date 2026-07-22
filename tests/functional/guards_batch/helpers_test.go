package guards_batch

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sync"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const executionIDTagKey = "_execution_id"

type fanoutParserExecutor struct {
	mu         sync.Mutex
	calls      int
	childCount int
}

func workerGeneratedBatchOutput(works []work.Work) string {
	encoded, err := json.Marshal(struct {
		Request work.WorkRequest `json:"request"`
	}{
		Request: work.WorkRequest{
			Type:  work.WorkRequestTypeFactoryRequestBatch,
			Works: works,
		},
	})
	if err != nil {
		panic(fmt.Sprintf("encode generated Work Request fixture: %v", err))
	}
	return string(encoded)
}

func (e *fanoutParserExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.mu.Lock()
	e.calls++
	e.mu.Unlock()

	parentTags := map[string]string{}
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		if token.Color.DataType != workers.DataTypeWork {
			continue
		}
		if len(token.Color.Tags) > 0 {
			parentTags = maps.Clone(token.Color.Tags)
		}
		break
	}

	generated := make([]work.Work, e.childCount)
	for i := range generated {
		generated[i] = work.Work{
			Name:       fmt.Sprintf("page-%d", i+1),
			WorkTypeID: "page",
			WorkID:     fmt.Sprintf("page-%d", i+1),
			Tags:       maps.Clone(parentTags),
		}
	}
	output := ""
	if len(generated) > 0 {
		output = workerGeneratedBatchOutput(generated)
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       output,
	}, nil
}

func (e *fanoutParserExecutor) callCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.calls
}

func executionIDFromDispatch(dispatch work.WorkDispatch) string {
	for _, token := range workers.WorkDispatchInputTokens(dispatch) {
		if token.Color.Tags[executionIDTagKey] != "" {
			return token.Color.Tags[executionIDTagKey]
		}
	}
	return ""
}

type execDirObservingProcessor struct {
	factoryDir      string
	wantExecutionID string

	mu                  sync.Mutex
	dispatchCount       int
	sawExecutionChannel bool
}

func (p *execDirObservingProcessor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	p.mu.Lock()
	p.dispatchCount++
	p.mu.Unlock()

	channelDir := filepath.Join(p.factoryDir, interfaces.InputsDir, "chapter", p.wantExecutionID)
	if st, err := os.Stat(channelDir); err == nil && st.IsDir() {
		p.mu.Lock()
		p.sawExecutionChannel = true
		p.mu.Unlock()
	}
	if got := executionIDFromDispatch(dispatch); got != p.wantExecutionID {
		return workerexecution.WorkResult{}, fmt.Errorf("page dispatch execution ID = %q, want %q", got, p.wantExecutionID)
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

func (p *execDirObservingProcessor) dispatchCountValue() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.dispatchCount
}

func (p *execDirObservingProcessor) sawExecutionChannelValue() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.sawExecutionChannel
}

type gatedProcessor struct {
	release <-chan struct{}

	mu            sync.Mutex
	dispatchCount int
}

func (p *gatedProcessor) Execute(ctx context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	select {
	case <-p.release:
	case <-ctx.Done():
		return workerexecution.WorkResult{}, ctx.Err()
	}

	p.mu.Lock()
	p.dispatchCount++
	p.mu.Unlock()

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

type multiChapterParserExecutor struct {
	mu          sync.Mutex
	calls       int
	childCounts []int
}

func (e *multiChapterParserExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	e.mu.Lock()
	call := e.calls
	e.calls++
	e.mu.Unlock()

	parentWorkID := ""
	if len(dispatch.InputTokens) > 0 {
		parentWorkID = support.FirstInputToken(dispatch.InputTokens).Color.WorkID
	}

	childCount := 0
	if call < len(e.childCounts) {
		childCount = e.childCounts[call]
	}

	generated := make([]work.Work, childCount)
	for i := range generated {
		generated[i] = work.Work{
			Name:       fmt.Sprintf("%s-page-%d", parentWorkID, i+1),
			WorkTypeID: "page",
			WorkID:     fmt.Sprintf("%s-page-%d", parentWorkID, i+1),
		}
	}
	output := ""
	if len(generated) > 0 {
		output = workerGeneratedBatchOutput(generated)
	}

	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       output,
	}, nil
}
