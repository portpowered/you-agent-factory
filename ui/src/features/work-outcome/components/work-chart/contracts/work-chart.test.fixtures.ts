// Shared fixture models for focused WorkChart component contracts.
import { FACTORY_EVENT_TYPES, type FactoryEvent } from "../../../../../api/events";
import { getDashboardWorkChartSeriesStyle } from "../../../lib/chart-contract";
import {
  createMaterializedWorkOutcomeState,
  reduceMaterializedWorkOutcomeEvents,
  selectMaterializedWorkOutcomeSamples,
} from "../../../lib/materializer/materialized-work-outcome";
import { buildWorkChartModel, type WorkChartModel } from "../../../lib/trends";
import type { WorkChartSeriesDefinition } from "../work-chart";

export const sparseWorkChartModel: WorkChartModel = {
  delta: {
    queued: 1,
    inFlight: 2,
    completed: 3,
    failed: 0,
  },
  failureGroups: [],
  points: [
    { label: "Tick 10", observedAt: 1000, order: 0, tick: 10 },
    { label: "Tick 20", observedAt: 2000, order: 1, tick: 20 },
    { label: "Tick 40", observedAt: 3000, order: 2, tick: 40 },
  ],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [
    {
      completedCount: 1,
      dispatchedCount: 0,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 1,
      observedAt: 1000,
      queuedCount: 3,
      tick: 10,
    },
    {
      completedCount: 3,
      dispatchedCount: 1,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 2000,
      queuedCount: 2,
      tick: 20,
    },
    {
      completedCount: 5,
      dispatchedCount: 2,
      failedByWorkType: {},
      failedCount: 0,
      failedWorkLabels: [],
      inFlightCount: 2,
      observedAt: 3000,
      queuedCount: 1,
      tick: 40,
    },
  ],
  series: [
    {
      key: "queued",
      label: "Queued",
      unit: "count",
      points: [
        { label: "Queued: 3", observedAt: 1000, order: 0, value: 3 },
        { label: "Queued: 1", observedAt: 3000, order: 2, value: 1 },
      ],
    },
    {
      key: "inFlight",
      label: "In-flight",
      unit: "count",
      points: [
        { label: "In-flight: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "In-flight: 2", observedAt: 3000, order: 2, value: 2 },
      ],
    },
    {
      key: "completed",
      label: "Completed",
      unit: "count",
      points: [
        { label: "Completed: 1", observedAt: 1000, order: 0, value: 1 },
        { label: "Completed: 3", observedAt: 2000, order: 2, value: 3 },
      ],
    },
    {
      key: "failed",
      label: "Failed/retried",
      unit: "count",
      points: [],
    },
  ],
};

export const emptyWorkChartModel: WorkChartModel = {
  delta: { queued: 0, inFlight: 0, completed: 0, failed: 0 },
  failureGroups: [],
  points: [],
  rangeID: "15m",
  rangeLabel: "15m",
  samples: [],
  series: [],
};

export const zeroValuedFailedSeriesModel: WorkChartModel = {
  ...sparseWorkChartModel,
  series: sparseWorkChartModel.series.map((seriesEntry) =>
    seriesEntry.key === "failed"
      ? {
          ...seriesEntry,
          points: [
            { label: "Failed: 0", observedAt: 2000, order: 1, value: 0 },
          ],
        }
      : seriesEntry,
  ),
};

export const edgeZoomWorkChartModel: WorkChartModel = {
  delta: { queued: -25, inFlight: 0, completed: 25, failed: 0 },
  failureGroups: [],
  points: Array.from({ length: 26 }, (_, index) => ({
    label: `Tick ${index}`,
    observedAt: index * 1000,
    order: index,
    tick: index,
  })),
  rangeID: "session",
  rangeLabel: "Session",
  samples: Array.from({ length: 26 }, (_, index) => ({
    completedCount: index,
    dispatchedCount: index,
    failedByWorkType: {},
    failedCount: 0,
    failedWorkLabels: [],
    inFlightCount: 1,
    observedAt: index * 1000,
    queuedCount: 25 - index,
    tick: index,
  })),
  series: [
    {
      key: "queued",
      label: "Queued",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: `Queued: ${25 - index}`,
        observedAt: index * 1000,
        order: index,
        value: 25 - index,
      })),
      unit: "count",
    },
    {
      key: "completed",
      label: "Completed",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: `Completed: ${index}`,
        observedAt: index * 1000,
        order: index,
        value: index,
      })),
      unit: "count",
    },
    {
      key: "inFlight",
      label: "In-flight",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: "In-flight: 1",
        observedAt: index * 1000,
        order: index,
        value: 1,
      })),
      unit: "count",
    },
    {
      key: "failed",
      label: "Failed/retried",
      points: Array.from({ length: 26 }, (_, index) => ({
        label: "Failed: 0",
        observedAt: index * 1000,
        order: index,
        value: 0,
      })),
      unit: "count",
    },
  ],
};

export const liveSessionLikeModel = buildWorkChartModel(
  selectMaterializedWorkOutcomeSamples(
    reduceMaterializedWorkOutcomeEvents(createMaterializedWorkOutcomeState(), [
      workOutcomeEvent("run-started", 0, FACTORY_EVENT_TYPES.runRequest, {
        factory: {
          resources: [{ capacity: 10, name: "executor-slot" }],
          workTypes: [
            {
              name: "plan",
              states: [
                { name: "init", type: "INITIAL" },
                { name: "complete", type: "TERMINAL" },
                { name: "failed", type: "FAILED" },
              ],
            },
          ],
          workers: [],
          workstations: [],
        },
        recordedAt: "2026-06-04T10:28:25.842914Z",
      }),
      workOutcomeEvent("work-request", 1, FACTORY_EVENT_TYPES.workRequest, {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Plan A",
            traceId: "trace-plan-a",
            workId: "plan-a",
            workTypeName: "plan",
          },
          {
            name: "Plan B",
            traceId: "trace-plan-b",
            workId: "plan-b",
            workTypeName: "plan",
          },
        ],
      }),
      workOutcomeEvent(
        "dispatch-request-plan-a",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "plan-a" }],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-a" },
      ),
      workOutcomeEvent(
        "dispatch-request-plan-b",
        2,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "plan-b" }],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-b" },
      ),
      workOutcomeEvent(
        "dispatch-response-plan-a",
        4,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 100,
          outcome: "ACCEPTED",
          outputWork: [
            {
              name: "Plan A",
              state: "complete",
              traceId: "trace-plan-a",
              workId: "plan-a",
              workTypeName: "plan",
            },
          ],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-a" },
      ),
      workOutcomeEvent(
        "dispatch-response-plan-b",
        6,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 100,
          failureMessage: "setup failed",
          failureReason: "workspace bootstrap failed",
          outcome: "FAILED",
          outputWork: [
            {
              name: "Plan B",
              state: "failed",
              traceId: "trace-plan-b",
              workId: "plan-b",
              workTypeName: "plan",
            },
          ],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-b" },
      ),
      workOutcomeEvent("work-request-2", 7, FACTORY_EVENT_TYPES.workRequest, {
        type: "FACTORY_REQUEST_BATCH",
        works: [
          {
            name: "Plan C",
            traceId: "trace-plan-c",
            workId: "plan-c",
            workTypeName: "plan",
          },
        ],
      }),
      workOutcomeEvent(
        "dispatch-request-plan-c",
        11,
        FACTORY_EVENT_TYPES.dispatchRequest,
        {
          inputs: [{ workId: "plan-c" }],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-c" },
      ),
      workOutcomeEvent(
        "dispatch-response-plan-c",
        12,
        FACTORY_EVENT_TYPES.dispatchResponse,
        {
          durationMillis: 100,
          outcome: "ACCEPTED",
          outputWork: [
            {
              name: "Plan C",
              state: "complete",
              traceId: "trace-plan-c",
              workId: "plan-c",
              workTypeName: "plan",
            },
          ],
          transitionId: "setup-workspace",
        },
        { dispatchId: "dispatch-plan-c" },
      ),
    ]),
    12,
  ),
  "session",
  0,
  "en",
);

export const OUTCOME_SERIES: readonly WorkChartSeriesDefinition[] = [
  {
    key: "queued",
    label: "Queued",
    ...getDashboardWorkChartSeriesStyle("queued"),
  },
  {
    key: "completed",
    label: "Completed",
    ...getDashboardWorkChartSeriesStyle("completed"),
  },
  {
    key: "inFlight",
    label: "In-flight",
    ...getDashboardWorkChartSeriesStyle("inFlight"),
  },
  {
    key: "failed",
    label: "Failed",
    ...getDashboardWorkChartSeriesStyle("failed"),
  },
];

function workOutcomeEvent(
  id: string,
  tick: number,
  type: FactoryEvent["type"],
  payload: FactoryEvent["payload"],
  context: Partial<FactoryEvent["context"]> = {},
): FactoryEvent {
  return {
    context: {
      eventTime: `2026-06-04T10:28:${String(tick).padStart(2, "0")}.000Z`,
      sequence: tick,
      tick,
      ...context,
    },
    id,
    payload,
    type,
  };
}
