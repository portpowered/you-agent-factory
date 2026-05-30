import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi } from "bun:test";
import { WorkItemDetailCard } from "./work-item-card";
import {
  getSelectedWorkItemFixture,
  workstationRequest,
} from "./detail-card-test-helpers";
import { selectWorkItemExecutionDetails } from "../state/executionDetails";

function renderWorkItemDetailCard({
  onSelectWorkID,
  request,
}: {
  onSelectWorkID?: (workID: string) => void;
  request: ReturnType<typeof workstationRequest>;
}) {
  const { dispatchID, execution, selectedNode, workItem } =
    getSelectedWorkItemFixture();

  render(
    <WorkItemDetailCard
      dispatchAttempts={[]}
      executionDetails={selectWorkItemExecutionDetails({
        activeExecution: execution,
        dispatchID,
        selectedNode,
        workItem,
      })}
      onSelectWorkID={onSelectWorkID}
      selectedNode={selectedNode}
      selection={{
        dispatchId: dispatchID,
        execution,
        kind: "work-item",
        nodeId: selectedNode.node_id,
        workItem,
      }}
      workstationRequests={[request]}
    />,
  );
}

function requestDetailsRegion() {
  const dispatchCard = within(
    screen.getByRole("region", { name: "Workstation dispatches" }),
  ).getAllByRole("article")[0];

  if (!(dispatchCard instanceof HTMLElement)) {
    throw new Error("expected selected-work dispatch history card");
  }

  return within(
    within(dispatchCard).getByRole("region", { name: "Request details" }),
  );
}

describe("WorkItemDetailCard lineage payload rendering", () => {
  it("renders selected-work dispatch request payloads inline from lineage snapshots", () => {
    const { dispatchID, workItem } = getSelectedWorkItemFixture();

    renderWorkItemDetailCard({
      request: workstationRequest(dispatchID, {
        request_view: {
          input_work_items: [
            {
              content: [
                {
                  text: "Historically consumed payload for selected work dispatch",
                  type: "text",
                },
              ],
              display_name: "Blocked Story",
              payload_status: "RESOLVED",
              state: "review",
              trace_id: "trace-blocked-story",
              work_id: "work-blocked-story",
              work_type_id: "story",
            },
          ],
        },
        response_view: {
          outcome: "ACCEPTED",
          output_work_items: [workItem],
        },
        trace_ids: ["trace-active-story"],
        work_items: [workItem],
      }),
    });

    const requestDetails = requestDetailsRegion();

    expect(requestDetails.getByText("Consumed payload")).toBeTruthy();
    expect(
      requestDetails.getByText(
        "Historically consumed payload for selected work dispatch",
      ),
    ).toBeTruthy();
    expect(requestDetails.getByText("State: review")).toBeTruthy();
    expect(requestDetails.getByText("Work type: story")).toBeTruthy();
  });
});

describe("WorkItemDetailCard lineage payload selection", () => {
  it("keeps work selection available when selected-work dispatch payload lineage is unavailable", () => {
    const { dispatchID, workItem } = getSelectedWorkItemFixture();
    const onSelectWorkID = vi.fn();

    renderWorkItemDetailCard({
      onSelectWorkID,
      request: workstationRequest(dispatchID, {
        request_view: {
          input_work_items: [
            {
              display_name: "Missing lineage story",
              payload_status: "UNAVAILABLE",
              payload_unavailable_reason:
                "the consumed snapshot could not be reconstructed for this dispatch",
              trace_id: "trace-missing-lineage",
              work_id: "work-missing-lineage",
              work_type_id: "story",
            },
          ],
        },
        response_view: {
          outcome: "ACCEPTED",
          output_work_items: [workItem],
        },
        trace_ids: ["trace-active-story"],
        work_items: [workItem],
      }),
    });

    const requestDetails = requestDetailsRegion();

    expect(
      requestDetails.getByText(
        /Consumed payload details are unavailable for this work item\./,
      ),
    ).toBeTruthy();
    expect(
      requestDetails.getByText(
        /the consumed snapshot could not be reconstructed for this dispatch/,
      ),
    ).toBeTruthy();

    fireEvent.click(
      requestDetails.getByRole("button", {
        name: "Select work item Missing lineage story",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-missing-lineage");
  });
});
