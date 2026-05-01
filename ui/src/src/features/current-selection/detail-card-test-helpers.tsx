import { render } from "@testing-library/react";
import {
  buildDashboardInferenceAttemptFixture,
  buildDashboardWorkstationRequestFixture,
} from "../../components/dashboard/fixtures";
import { semanticWorkflowDashboardSnapshot } from "../../components/dashboard/test-fixtures";
import { selectWorkItemExecutionDetails } from "../../state/executionDetails";
import type {
  DashboardInferenceAttempt,
  DashboardWorkstationRequest,
} from "../../../api/dashboard/types";
import { WorkItemDetailCard } from "./work-item-card";

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

export function renderSelectedWorkItemWithInferenceAttempts(
  attemptsByRequestID: Record<string, DashboardInferenceAttempt>,
) {
  const { dispatchID, execution, selectedNode, workItem } = getSelectedWorkItemFixture();

  render(
    <WorkItemDetailCard
      dispatchAttempts={[]}
      executionDetails={selectWorkItemExecutionDetails({
        activeExecution: execution,
        dispatchID,
        inferenceAttemptsByDispatchID: {
          [dispatchID]: attemptsByRequestID,
        },
        selectedNode,
        workItem,
      })}
      now={DETAIL_CARD_NOW}
      selectedNode={selectedNode}
      selection={{
        dispatchId: dispatchID,
        execution,
        kind: "work-item",
        nodeId: selectedNode.node_id,
        workItem,
      }}
      workstationRequests={[]}
    />,
  );

  return {
    dispatchID,
    execution,
    selectedNode,
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
