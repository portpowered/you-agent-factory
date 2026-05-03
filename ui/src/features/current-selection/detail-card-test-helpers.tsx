import {
  buildDashboardInferenceAttemptFixture,
  buildDashboardWorkstationRequestFixture,
} from "../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import type {
  DashboardInferenceAttempt,
  DashboardWorkstationRequest,
} from "../../api/dashboard/types";

export const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

export function getSelectedWorkItemFixture() {
  const snapshot = semanticWorkflowDashboardSnapshot;
  const dispatchID = snapshot.runtime.active_dispatch_ids?.[0] ?? "";
  const execution = snapshot.runtime.active_executions_by_dispatch_id?.[dispatchID];
  const workItem = execution?.work_items?.[0];
  const selectedNode = snapshot.topology.workstation_nodes_by_id.review;

  if (!execution || !workItem || !selectedNode) {
    throw new Error("expected semantic workflow fixture to include an active selected work item");
  }

  return {
    dispatchID,
    execution,
    selectedNode,
    snapshot,
    workItem,
  };
}

export function inferenceAttempt(
  dispatchID: string,
  overrides: Partial<DashboardInferenceAttempt>,
): DashboardInferenceAttempt {
  return buildDashboardInferenceAttemptFixture(dispatchID, overrides);
}

export function workstationRequest(
  dispatchID: string,
  overrides: Partial<DashboardWorkstationRequest> = {},
): DashboardWorkstationRequest {
  return buildDashboardWorkstationRequestFixture(dispatchID, overrides);
}

