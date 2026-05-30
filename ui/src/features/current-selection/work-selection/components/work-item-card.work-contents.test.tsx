import { render, screen, within } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { DashboardWorkItemRef } from "../../../../api/dashboard/types";
import {
  getSelectedWorkItemFixture,
  multimodalSelectedWorkPayloadOverrides,
  workstationRequest,
} from "../../base/components/detail-card-test-helpers";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";
import { WorkItemDetailCard } from "./work-item-card";

function renderWorkItemDetailCard(
  workItemOverrides: Partial<DashboardWorkItemRef> = {},
) {
  const { dispatchID, execution, selectedNode, workItem } =
    getSelectedWorkItemFixture();
  const selectedWorkItem = { ...workItem, ...workItemOverrides };

  render(
    <WorkItemDetailCard
      dispatchAttempts={[]}
      executionDetails={selectWorkItemExecutionDetails({
        activeExecution: execution,
        dispatchID,
        selectedNode,
        workItem: selectedWorkItem,
      })}
      selectedNode={selectedNode}
      selection={{
        dispatchId: dispatchID,
        execution,
        kind: "work-item",
        nodeId: selectedNode.node_id,
        workItem: selectedWorkItem,
      }}
      workstationRequests={[workstationRequest(dispatchID)]}
    />,
  );
}

function workContentsRegion() {
  return screen.getByRole("region", { name: "Work contents" });
}

describe("WorkItemDetailCard work contents", () => {
  it("renders resolved payload parts in an accessible Work contents region outside dispatch history", () => {
    renderWorkItemDetailCard(multimodalSelectedWorkPayloadOverrides());

    expect(
      within(workContentsRegion()).getByText("Image: screenshot.png"),
    ).toBeTruthy();

    const workContents = workContentsRegion();
    const dispatchHistory = screen.getByRole("region", {
      name: "Workstation dispatches",
    });

    expect(
      within(workContents).getByText("Primary selected-work payload text"),
    ).toBeTruthy();
    expect(within(workContents).getByText(/"priority": 1/)).toBeTruthy();
    expect(
      within(dispatchHistory).queryByText("Primary selected-work payload text"),
    ).toBeNull();
  });

  it("renders unavailable payload status with reason text", () => {
    renderWorkItemDetailCard({
      payload_status: "UNAVAILABLE",
      payload_unavailable_reason: "selected-work lineage snapshot missing",
    });

    expect(
      within(workContentsRegion()).getByText(
        "Work content is unavailable. selected-work lineage snapshot missing",
      ),
    ).toBeTruthy();
  });

  it("renders a loading-state message", () => {
    renderWorkItemDetailCard({
      payload_status: "LOADING",
    });

    expect(
      within(workContentsRegion()).getByText("Loading work content…"),
    ).toBeTruthy();
  });

  it("renders an error-state message with reason text", () => {
    renderWorkItemDetailCard({
      payload_status: "ERROR",
      payload_unavailable_reason: "projection failed",
    });

    expect(
      within(workContentsRegion()).getByText(
        "Work content could not be loaded. projection failed",
      ),
    ).toBeTruthy();
  });

  it("renders an empty-state message when resolved content has no parts", () => {
    renderWorkItemDetailCard({
      content: [],
      payload_status: "RESOLVED",
    });

    expect(
      within(workContentsRegion()).getByText(
        "No work content items are available for this selection.",
      ),
    ).toBeTruthy();
  });
});
