import { render, screen } from "@testing-library/react";
import { CurrentSelectionExecutionPill } from "../../../base/components/presentation/current-selection-pill";
import { CurrentSelectionSupportingText } from "../../../base/components/presentation/current-selection-supporting-text";
import { WorkstationDispatchRow } from "../detail-card/workstation-dispatch-row";

describe("WorkstationDispatchRow", () => {
  it("renders title, status, actions, and supporting content in the active-work row shell", () => {
    render(
      <ul>
        <WorkstationDispatchRow
          actions={<button type="button">Open request</button>}
          status={
            <CurrentSelectionExecutionPill>
              Elapsed: 15s
            </CurrentSelectionExecutionPill>
          }
          supportingContent={
            <CurrentSelectionSupportingText tone="status">
              Request details unavailable for dispatch-1
            </CurrentSelectionSupportingText>
          }
          title="Review Story"
        />
      </ul>,
    );

    const item = screen.getByRole("listitem");
    expect(item.className).toContain("rounded-lg");
    expect(screen.getByText("Review Story")).toBeTruthy();
    expect(screen.getByText("Elapsed: 15s")).toBeTruthy();
    expect(screen.getByRole("button", { name: "Open request" })).toBeTruthy();
    expect(
      screen.getByText("Request details unavailable for dispatch-1"),
    ).toBeTruthy();
  });
});
