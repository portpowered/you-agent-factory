import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi } from "bun:test";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";
import { workstationRequest } from "./detail-card-test-helpers";

describe("WorkstationRequestDetailCard consumed payloads", () => {
  it("renders lineage-resolved consumed payload content inline inside request details", () => {
    render(
      <WorkstationRequestDetailCard
        request={workstationRequest("dispatch-review-consumed-payload", {
          request_view: {
            input_work_items: [
              {
                content: [
                  { text: "Dispatch-time consumed payload", type: "text" },
                ],
                display_name: "Blocked Story",
                payload_status: "RESOLVED",
                state: "review",
                trace_id: "trace-blocked",
                work_id: "work-blocked-story",
                work_type_id: "story",
              },
            ],
          },
        })}
      />,
    );

    const requestDetails = within(
      screen.getByRole("region", { name: "Request details" }),
    );
    expect(requestDetails.getByText("Consumed payload")).toBeTruthy();
    expect(
      requestDetails.getByText("Dispatch-time consumed payload"),
    ).toBeTruthy();
    expect(requestDetails.getByText("State: review")).toBeTruthy();
    expect(requestDetails.getByText("Work type: story")).toBeTruthy();
  });

  it("renders an explicit unavailable consumed payload state without breaking work selection", () => {
    const onSelectWorkID = vi.fn();

    render(
      <WorkstationRequestDetailCard
        onSelectWorkID={onSelectWorkID}
        request={workstationRequest("dispatch-review-consumed-unavailable", {
          request_view: {
            input_work_items: [
              {
                display_name: "Missing lineage story",
                payload_status: "UNAVAILABLE",
                payload_unavailable_reason:
                  "no lineage snapshot was recorded before this dispatch consumed the work item",
                trace_id: "trace-missing-lineage",
                work_id: "work-missing-lineage",
                work_type_id: "story",
              },
            ],
          },
        })}
      />,
    );

    const requestDetails = within(
      screen.getByRole("region", { name: "Request details" }),
    );
    expect(
      requestDetails.getByText(
        /Consumed payload details are unavailable for this work item\./,
      ),
    ).toBeTruthy();
    expect(
      requestDetails.getByText(
        /no lineage snapshot was recorded before this dispatch consumed the work item/,
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
