import "../../styles.css";

import type {
  DashboardTraceDispatch,
  DashboardWorkItemRef,
  DashboardWorkRelation,
} from "../../api/dashboard/types";
import { TraceRelationFlow } from "./trace-relation-flow";
import { TraceWorkstationPath } from "./trace-workstation-path";

function buildWorkItem(
  workID: string,
  displayName: string,
  chainingTraceID?: string,
): DashboardWorkItemRef {
  return {
    current_chaining_trace_id: chainingTraceID,
    display_name: displayName,
    work_id: workID,
    work_type_id: "story",
  };
}

function buildDispatch(
  dispatchID: string,
  overrides: Partial<DashboardTraceDispatch> = {},
): DashboardTraceDispatch {
  return {
    dispatch_id: dispatchID,
    duration_millis: 1000,
    end_time: "2026-04-22T18:00:01Z",
    outcome: "ACCEPTED",
    start_time: "2026-04-22T18:00:00Z",
    transition_id: dispatchID,
    workstation_name: dispatchID,
    ...overrides,
  };
}

const TRACE_DISPATCHES: DashboardTraceDispatch[] = [
  buildDispatch("dispatch-plan", {
    current_chaining_trace_id: "trace-plan-chain",
    output_items: [
      buildWorkItem("work-reviewed", "Reviewed Story", "trace-plan-chain"),
    ],
    workstation_name: "Plan",
  }),
  buildDispatch("dispatch-research", {
    current_chaining_trace_id: "trace-research-chain",
    output_items: [
      buildWorkItem("work-context", "Context Story", "trace-research-chain"),
    ],
    workstation_name: "Research",
  }),
  buildDispatch("dispatch-implement", {
    input_items: [buildWorkItem("work-reviewed", "Reviewed Story")],
    previous_chaining_trace_ids: ["trace-plan-chain", "trace-research-chain"],
    workstation_name: "Implement",
  }),
];

const TRACE_RELATIONS: DashboardWorkRelation[] = [
  {
    request_id: "request-parent-child",
    required_state: "DONE",
    source_work_id: "work-plan",
    source_work_name: "Plan story",
    target_work_id: "work-implement",
    target_work_name: "Implement story",
    type: "PARENT_CHILD",
  },
  {
    request_id: "request-retry",
    required_state: "FAILED",
    source_work_id: "work-implement",
    source_work_name: "Implement story",
    target_work_id: "work-repair",
    target_work_name: "Repair story",
    type: "RETRY",
  },
];

export default {
  title: "Agent Factory/Dashboard/Trace Graph Surfaces",
};

export const VisualConsistency = {
  render: () => (
    <div className="grid gap-4">
      <TraceWorkstationPath dispatches={TRACE_DISPATCHES} />
      <TraceRelationFlow relations={TRACE_RELATIONS} />
    </div>
  ),
};
