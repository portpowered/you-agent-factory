import type { DashboardWorkItemRef } from "../../api/dashboard";
import type { FactoryWorkItem } from "../../api/events";
import { dashboardWorkTypeID } from "./systemTime";

export function workRef(item: FactoryWorkItem): DashboardWorkItemRef {
  return {
    ...(item.current_chaining_trace_id
      ? { current_chaining_trace_id: item.current_chaining_trace_id }
      : {}),
    display_name: item.display_name,
    ...(item.previous_chaining_trace_ids
      ? {
          previous_chaining_trace_ids: [...item.previous_chaining_trace_ids],
        }
      : {}),
    trace_id: item.trace_id,
    work_id: item.id,
    work_type_id: dashboardWorkTypeID(item.work_type_id),
  };
}
