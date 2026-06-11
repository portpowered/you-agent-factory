export type DurableSessionInspectionScenarioPurpose =
  | "running"
  | "completed"
  | "failed-recoverable"
  | "result-not-ready"
  | "result-available"
  | "dispatch-list"
  | "artifact-list"
  | "artifact-detail"
  | "empty-dispatches"
  | "empty-artifacts";

export const DURABLE_SESSION_INSPECTION_SCENARIO_IDS = {
  running: "javascript-running-n-dispatch",
  completed: "javascript-succeeded-two-dispatch",
  "failed-recoverable": "javascript-interrupted-recoverable",
  "result-not-ready": "javascript-sync-timed-out",
  "result-available": "javascript-succeeded-two-dispatch",
  "dispatch-list": "javascript-running-n-dispatch",
  "artifact-list": "javascript-paused-two-dispatch",
  "artifact-detail": "javascript-paused-two-dispatch",
  "empty-dispatches": "javascript-awaiting-approval",
  "empty-artifacts": "javascript-running-n-dispatch",
} as const satisfies Record<DurableSessionInspectionScenarioPurpose, string>;

export const DURABLE_SESSION_INSPECTION_SCENARIO_PURPOSES = Object.keys(
  DURABLE_SESSION_INSPECTION_SCENARIO_IDS,
) as DurableSessionInspectionScenarioPurpose[];

export function durableSessionInspectionScenarioID(
  purpose: DurableSessionInspectionScenarioPurpose,
): string {
  return DURABLE_SESSION_INSPECTION_SCENARIO_IDS[purpose];
}
