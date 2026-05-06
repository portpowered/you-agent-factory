import type { DashboardRuntimeWorkstationRequest } from "../../../api/dashboard";

import { runtimeDetailsFixtureIDs } from "./runtime-details-events";

export const runtimeDetailsBackendWorkstationRequestsByDispatchID = {
  [runtimeDetailsFixtureIDs.activeDispatchID]: {
    counts: {
      dispatched_count: 0,
      errored_count: 0,
      responded_count: 0,
    },
    dispatch_id: runtimeDetailsFixtureIDs.activeDispatchID,
    request: {
      input_work_items: [
        {
          display_name: runtimeDetailsFixtureIDs.activeWorkLabel,
          trace_id: runtimeDetailsFixtureIDs.activeTraceID,
          work_id: runtimeDetailsFixtureIDs.activeWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      started_at: "2026-04-18T12:00:03Z",
      trace_ids: [runtimeDetailsFixtureIDs.activeTraceID],
    },
    transition_id: "review",
    workstation_name: "Review",
  },
  [runtimeDetailsFixtureIDs.completedDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 0,
      responded_count: 1,
    },
    dispatch_id: runtimeDetailsFixtureIDs.completedDispatchID,
    request: {
      input_work_items: [
        {
          display_name: runtimeDetailsFixtureIDs.completedWorkLabel,
          trace_id: runtimeDetailsFixtureIDs.completedTraceID,
          work_id: runtimeDetailsFixtureIDs.completedWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      started_at: "2026-04-18T12:00:04Z",
      trace_ids: [runtimeDetailsFixtureIDs.completedTraceID],
    },
    response: {
      duration_millis: runtimeDetailsFixtureIDs.completedDurationMillis,
      end_time: "2026-04-18T12:00:07Z",
      outcome: "ACCEPTED",
    },
    transition_id: "review",
    workstation_name: "Review",
  },
  [runtimeDetailsFixtureIDs.failedDispatchID]: {
    counts: {
      dispatched_count: 1,
      errored_count: 1,
      responded_count: 0,
    },
    dispatch_id: runtimeDetailsFixtureIDs.failedDispatchID,
    request: {
      input_work_items: [
        {
          display_name: runtimeDetailsFixtureIDs.failedWorkLabel,
          trace_id: runtimeDetailsFixtureIDs.failedTraceID,
          work_id: runtimeDetailsFixtureIDs.failedWorkID,
          work_type_id: "story",
        },
      ],
      input_work_type_ids: ["story"],
      started_at: "2026-04-18T12:00:08Z",
      trace_ids: [runtimeDetailsFixtureIDs.failedTraceID],
    },
    response: {
      duration_millis: runtimeDetailsFixtureIDs.failedDurationMillis,
      end_time: "2026-04-18T12:00:11Z",
      failure_message: runtimeDetailsFixtureIDs.failedFailureMessage,
      failure_reason: runtimeDetailsFixtureIDs.failedFailureReason,
      outcome: "FAILED",
    },
    transition_id: "review",
    workstation_name: "Review",
  },
} satisfies Record<string, DashboardRuntimeWorkstationRequest>;
