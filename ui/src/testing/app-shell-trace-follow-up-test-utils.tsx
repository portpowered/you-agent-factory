import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { FactoryEvent } from "../api/events";
import { FACTORY_EVENT_TYPES } from "../api/events";
import { TraceDrilldownWidget, useTraceDrilldown } from "../features/trace-drilldown/public";
import { useFactoryTimelineStore } from "../features/timeline/state/factoryTimelineStore";

export const activeWorkID = "work-active-story";
export const fanInResultLabel = "Implemented Story";
export const fanInResultWorkID = "work-result";

function factoryEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-04-16T12:00:0${tick}Z`,
      sequence: tick,
      tick,
    },
    id,
    payload,
    type,
  };
}

function withFactoryEventContext(
  event: FactoryEvent,
  context: Partial<FactoryEvent["context"]>,
): FactoryEvent {
  return {
    ...event,
    context: {
      ...event.context,
      ...context,
    },
  };
}

const reviewWorkstation = {
  id: "review",
  inputs: [{ state: "new", workType: "story" }],
  name: "Review",
  outputs: [{ state: "review", workType: "story" }],
  worker: "reviewer",
} as const;

const completeWorkstation = {
  id: "complete",
  inputs: [{ state: "review", workType: "story" }],
  name: "Complete",
  outputs: [{ state: "active", workType: "story" }],
  worker: "completer",
} as const;

function traceFanInFactory(): FactoryEvent {
  return factoryEvent("trace-fan-in-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
    factory: {
      workTypes: [
        {
          name: "story",
          states: [
            { name: "new", type: "INITIAL" },
            { name: "review", type: "PROCESSING" },
            { name: "active", type: "PROCESSING" },
          ],
        },
      ],
      workstations: [reviewWorkstation, completeWorkstation],
    },
  });
}

function traceFanInRequestEvents(): FactoryEvent[] {
  return [
    withFactoryEventContext(
      factoryEvent("trace-fan-in-2", 2, FACTORY_EVENT_TYPES.workRequest, {
        source: "api",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Plan Input",
            trace_id: "chain-a",
            work_id: "work-plan-input",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Input",
            trace_id: "chain-b",
            work_id: "work-research-input",
            work_type_id: "story",
          },
        ],
      }),
      {
        requestId: "request-chain",
        traceIds: ["chain-a", "chain-b"],
        workIds: ["work-plan-input", "work-research-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-plan",
        inputs: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Plan Input",
            trace_id: "chain-a",
            work_id: "work-plan-input",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: reviewWorkstation,
      }),
      {
        dispatchId: "dispatch-plan",
        traceIds: ["chain-a"],
        workIds: ["work-plan-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-plan",
        durationMillis: 450,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Reviewed Story",
            trace_id: "chain-a",
            work_id: "work-reviewed-story",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: reviewWorkstation,
      }),
      {
        dispatchId: "dispatch-plan",
        traceIds: ["chain-a"],
        workIds: ["work-plan-input"],
      },
    ),
  ];
}

function traceFanInResearchAndMergeEvents(): FactoryEvent[] {
  return [
    withFactoryEventContext(
      factoryEvent("trace-fan-in-5", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
        current_chaining_trace_id: "chain-b",
        dispatchId: "dispatch-research",
        inputs: [
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Input",
            trace_id: "chain-b",
            work_id: "work-research-input",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: reviewWorkstation,
      }),
      {
        dispatchId: "dispatch-research",
        traceIds: ["chain-b"],
        workIds: ["work-research-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-6", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
        current_chaining_trace_id: "chain-b",
        dispatchId: "dispatch-research",
        durationMillis: 420,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Context",
            trace_id: "chain-b",
            work_id: "work-research-context",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: reviewWorkstation,
      }),
      {
        dispatchId: "dispatch-research",
        traceIds: ["chain-b"],
        workIds: ["work-research-input"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-7", 7, FACTORY_EVENT_TYPES.dispatchRequest, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-implement",
        inputs: [
          {
            current_chaining_trace_id: "chain-a",
            name: "Reviewed Story",
            trace_id: "chain-a",
            work_id: "work-reviewed-story",
            work_type_id: "story",
          },
          {
            current_chaining_trace_id: "chain-b",
            name: "Research Context",
            trace_id: "chain-b",
            work_id: "work-research-context",
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        transitionId: "complete",
        workstation: completeWorkstation,
      }),
      {
        dispatchId: "dispatch-implement",
        traceIds: ["chain-a", "chain-b"],
        workIds: ["work-reviewed-story", "work-research-context"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-fan-in-8", 8, FACTORY_EVENT_TYPES.dispatchResponse, {
        current_chaining_trace_id: "chain-a",
        dispatchId: "dispatch-implement",
        durationMillis: 900,
        outcome: "ACCEPTED",
        outputWork: [
          {
            current_chaining_trace_id: "chain-a",
            name: fanInResultLabel,
            previous_chaining_trace_ids: ["chain-a", "chain-b"],
            trace_id: "chain-a",
            work_id: fanInResultWorkID,
            work_type_id: "story",
          },
        ],
        previous_chaining_trace_ids: ["chain-a", "chain-b"],
        transitionId: "complete",
        workstation: completeWorkstation,
      }),
      {
        dispatchId: "dispatch-implement",
        traceIds: ["chain-a", "chain-b"],
        workIds: ["work-reviewed-story", "work-research-context"],
      },
    ),
  ];
}

export function buildTraceFanInTimelineEvents(): FactoryEvent[] {
  return [
    traceFanInFactory(),
    ...traceFanInRequestEvents(),
    ...traceFanInResearchAndMergeEvents(),
  ];
}

export function buildLegacyTraceTimelineEvents(): FactoryEvent[] {
  return [
    factoryEvent("trace-legacy-1", 1, FACTORY_EVENT_TYPES.initialStructureRequest, {
      factory: {
        workTypes: [
          {
            name: "story",
            states: [
              { name: "new", type: "INITIAL" },
              { name: "review", type: "PROCESSING" },
              { name: "active", type: "PROCESSING" },
            ],
          },
        ],
        workstations: [reviewWorkstation, completeWorkstation],
      },
    }),
    withFactoryEventContext(
      factoryEvent("trace-legacy-2", 2, FACTORY_EVENT_TYPES.workRequest, {
        source: "api",
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Legacy Story",
            trace_id: "trace-legacy",
            work_id: "work-legacy",
            work_type_id: "story",
          },
        ],
      }),
      { requestId: "request-legacy", traceIds: ["trace-legacy"], workIds: ["work-legacy"] },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-3", 3, FACTORY_EVENT_TYPES.dispatchRequest, {
        dispatchId: "dispatch-legacy-review",
        inputs: [
          {
            name: "Legacy Story",
            trace_id: "trace-legacy",
            work_id: "work-legacy",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: reviewWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-review",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-4", 4, FACTORY_EVENT_TYPES.dispatchResponse, {
        dispatchId: "dispatch-legacy-review",
        durationMillis: 360,
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "Legacy Review",
            trace_id: "trace-legacy",
            work_id: "work-legacy-reviewed",
            work_type_id: "story",
          },
        ],
        transitionId: "review",
        workstation: reviewWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-review",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-5", 5, FACTORY_EVENT_TYPES.dispatchRequest, {
        dispatchId: "dispatch-legacy-complete",
        inputs: [
          {
            name: "Legacy Review",
            trace_id: "trace-legacy",
            work_id: "work-legacy-reviewed",
            work_type_id: "story",
          },
        ],
        transitionId: "complete",
        workstation: completeWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-complete",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy-reviewed"],
      },
    ),
    withFactoryEventContext(
      factoryEvent("trace-legacy-6", 6, FACTORY_EVENT_TYPES.dispatchResponse, {
        dispatchId: "dispatch-legacy-complete",
        durationMillis: 640,
        outcome: "ACCEPTED",
        outputWork: [
          {
            name: "Legacy Done",
            trace_id: "trace-legacy",
            work_id: "work-legacy-done",
            work_type_id: "story",
          },
        ],
        transitionId: "complete",
        workstation: completeWorkstation,
      }),
      {
        dispatchId: "dispatch-legacy-complete",
        traceIds: ["trace-legacy"],
        workIds: ["work-legacy-reviewed"],
      },
    ),
  ];
}

function TraceDrilldownTestHarness({
  selectedWorkID,
}: {
  selectedWorkID: string;
}) {
  const { traceGridState } = useTraceDrilldown(selectedWorkID);

  return <TraceDrilldownWidget state={traceGridState} />;
}

export function renderTraceDrilldownHarness({
  selectedWorkID,
  timelineEvents,
}: {
  selectedWorkID: string;
  timelineEvents: FactoryEvent[];
}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: {
        gcTime: Infinity,
        retry: false,
      },
    },
  });

  useFactoryTimelineStore.getState().replaceEvents(timelineEvents);

  return render(
    <QueryClientProvider client={queryClient}>
      <TraceDrilldownTestHarness selectedWorkID={selectedWorkID} />
    </QueryClientProvider>,
  );
}
