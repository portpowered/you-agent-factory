import { TraceGridBentoCard } from "./trace-grid-card";
import type { DashboardTrace } from "../../api/dashboard/types";

const populatedTrace: DashboardTrace = {
  dispatches: [
    {
      input_items: [
        {
          display_name: "Active Story",
          current_chaining_trace_id: "trace-active-story-chain",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      current_chaining_trace_id: "trace-review-chain",
      dispatch_id: "dispatch-review-active",
      duration_millis: 1000,
      end_time: "2026-04-08T12:00:01Z",
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Reviewed Story",
          current_chaining_trace_id: "trace-review-chain",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      start_time: "2026-04-08T12:00:00Z",
      transition_id: "plan",
      workstation_name: "Plan",
    },
    {
      input_items: [
        {
          display_name: "Reviewed Story",
          current_chaining_trace_id: "trace-review-chain",
          work_id: "work-reviewed-story",
          work_type_id: "story",
        },
      ],
      current_chaining_trace_id: "trace-implement-chain",
      dispatch_id: "dispatch-implement-active",
      duration_millis: 2000,
      end_time: "2026-04-08T12:00:04Z",
      outcome: "ACCEPTED",
      output_items: [
        {
          display_name: "Implemented Story",
          current_chaining_trace_id: "trace-implement-chain",
          work_id: "work-implemented-story",
          work_type_id: "story",
        },
      ],
      previous_chaining_trace_ids: ["trace-review-chain"],
      start_time: "2026-04-08T12:00:02Z",
      transition_id: "implement",
      workstation_name: "Implement",
    },
  ],
  work_items: [
    {
      display_name: "Active Story",
      work_id: "work-active-story",
      work_type_id: "story",
    },
    {
      display_name: "Reviewed Story",
      work_id: "work-reviewed-story",
      work_type_id: "story",
    },
    {
      display_name: "Implemented Story",
      work_id: "work-implemented-story",
      work_type_id: "story",
    },
  ],
  trace_id: "trace-active-story",
  relations: [
    {
      request_id: "request-story-batch",
      required_state: "DONE",
      source_work_id: "work-active-story",
      source_work_name: "Active Story",
      target_work_id: "work-reviewed-story",
      target_work_name: "Reviewed Story",
      type: "PARENT_CHILD",
    },
  ],
  transition_ids: ["plan", "implement"],
  work_ids: ["work-active-story"],
  workstation_sequence: ["Plan", "Implement"],
};

export default {
  title: "Agent Factory/Dashboard/Trace Grid Bento Card",
  component: TraceGridBentoCard,
};

export const PopulatedTrace = {
  args: {
    state: { status: "ready", trace: populatedTrace },
    widgetId: "trace-story",
  },
};

export const EmptyTrace = {
  args: {
    state: { status: "empty", workID: "work-missing" },
    widgetId: "trace-empty-story",
  },
};

export const LoadingTrace = {
  args: {
    state: { status: "loading", workID: "work-active-story" },
    widgetId: "trace-loading-story",
  },
};

export const TraceError = {
  args: {
    state: { status: "error", message: "dashboard event history is unavailable" },
    widgetId: "trace-error-story",
  },
};

