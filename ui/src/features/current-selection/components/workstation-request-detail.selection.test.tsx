import { fireEvent, render, screen, within } from "@testing-library/react";
import { vi } from "vitest";
import { workstationRequest } from "./detail-card-test-helpers";
import { WorkstationRequestDetailCard } from "./workstation-request-detail";

describe("WorkstationRequestDetailCard work-item selection", () => {
  it("lets consumed work items become the current selection from request details", () => {
    const onSelectWorkID = vi.fn();

    render(
      <WorkstationRequestDetailCard
        onSelectWorkID={onSelectWorkID}
        request={workstationRequest("dispatch-review-consumed", {
          request_view: {
            input_work_items: [
              {
                display_name: "Blocked Story",
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
    expect(
      requestDetails.getByRole("button", {
        name: "Select work item Blocked Story",
      }).textContent,
    ).toBe("Blocked Story");

    fireEvent.click(
      requestDetails.getByRole("button", {
        name: "Select work item Blocked Story",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-blocked-story");
  });
});
