import { fireEvent, render, screen } from "@testing-library/react";
import type { DashboardWorkstationRequest } from "../../api/dashboard/types";
import { ProviderSessionAttempts } from "./provider-session-attempts";

describe("ProviderSessionAttempts", () => {
  it("uses the default workstation-detail helper messages when no localized messages are provided", () => {
    const onSelectWorkID = vi.fn();
    const onSelectWorkstationRequest = vi.fn();
    const request: DashboardWorkstationRequest = {
      dispatch_id: "dispatch-review-active",
      dispatched_request_count: 1,
      errored_request_count: 0,
      inference_attempts: [],
      prompt: "Review the active story and decide whether it is ready.",
      responded_request_count: 1,
      transition_id: "transition-review",
      work_items: [
        {
          display_name: "Active Story",
          trace_id: "trace-active-story",
          work_id: "work-active-story",
          work_type_id: "story",
        },
      ],
      workstation_name: "Review",
      workstation_node_id: "workstation-review",
    };

    render(
      <ProviderSessionAttempts
        attempts={[
          {
            dispatch_id: "dispatch-review-active",
            outcome: "ACCEPTED",
            transition_id: "transition-review",
            workstation_name: "Review",
            work_items: [
              {
                display_name: "Active Story",
                trace_id: "trace-active-story",
                work_id: "work-active-story",
                work_type_id: "story",
              },
            ],
          },
          {
            dispatch_id: "dispatch-review-missing-details",
            outcome: "FAILED",
            transition_id: "transition-review",
            workstation_name: "Review",
          },
        ]}
        currentDispatchID="dispatch-review-active"
        emptyMessage="No workstation runs have been recorded for this workstation yet."
        onSelectWorkID={onSelectWorkID}
        onSelectWorkstationRequest={onSelectWorkstationRequest}
        renderHeading={(attempt) => attempt.dispatch_id}
        workstationKind="standard"
        workstationRequestsByDispatchID={{
          [request.dispatch_id]: request,
        }}
      />,
    );

    expect(screen.getByText("Current dispatch")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Select work item Active Story" }),
    ).toBeTruthy();
    expect(screen.getByText("Open Active Story")).toBeTruthy();
    expect(
      screen.getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    ).toBeTruthy();
    expect(screen.getByText("Open request details")).toBeTruthy();
    expect(screen.getAllByText("Session log unavailable")).toHaveLength(2);
    expect(
      screen.getByText(
        "Work details unavailable for dispatch dispatch-review-missing-details.",
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        "Request details unavailable for dispatch dispatch-review-missing-details.",
      ),
    ).toBeTruthy();

    fireEvent.click(
      screen.getByRole("button", { name: "Select work item Active Story" }),
    );
    fireEvent.click(
      screen.getByRole("button", {
        name: "Select workstation request dispatch-review-active",
      }),
    );

    expect(onSelectWorkID).toHaveBeenCalledWith("work-active-story");
    expect(onSelectWorkstationRequest).toHaveBeenCalledWith(request);
  });
});
