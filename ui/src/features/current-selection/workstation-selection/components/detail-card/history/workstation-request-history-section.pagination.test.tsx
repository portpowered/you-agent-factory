import "../../../../../../testing/vitest-dom-capabilities.setup";

import { fireEvent, render, screen, within } from "@testing-library/react";
import { buildDashboardWorkstationRequestFixture } from "../../../../../../components/dashboard/fixtures";
import { getWorkstationDetailMessages } from "../../../messages/workstation-detail";
import { WorkstationRequestHistorySection } from "../workstation-request-history-section";

const DETAIL_CARD_NOW = Date.parse("2026-04-08T12:00:04Z");

describe("WorkstationRequestHistorySection bounded history", () => {
  it("reveals bounded request history without skipping or duplicating rows", () => {
    const requestHistory = Array.from({ length: 12 }, (_, index) => {
      const requestNumber = index + 1;
      return buildDashboardWorkstationRequestFixture(
        `dispatch-review-history-${requestNumber}`,
        {
          request_id: `request-review-history-${requestNumber}`,
          work_items: [
            {
              display_name: `History Story ${requestNumber}`,
              trace_id: `trace-history-story-${requestNumber}`,
              work_id: `work-history-story-${requestNumber}`,
              work_type_id: "story",
            },
          ],
        },
      );
    });

    render(
      <WorkstationRequestHistorySection
        messages={getWorkstationDetailMessages("en")}
        now={DETAIL_CARD_NOW}
        onSelectWorkstationRequest={() => {}}
        requests={requestHistory}
        resetKey="review"
        selectedRequest={requestHistory[0]}
      />,
    );

    const requestHistorySection = screen
      .getByRole("heading", { name: "Request history" })
      .closest("section");
    if (!requestHistorySection) {
      throw new Error("expected request history section");
    }
    fireEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    const requestHistoryList = within(requestHistorySection).getByRole("list");
    expect(within(requestHistoryList).getAllByRole("listitem")).toHaveLength(
      10,
    );
    expect(
      within(requestHistorySection).getByText(
        "Showing 10 of 12 requests. 2 remaining.",
      ),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getByText("request-review-history-10"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection)
        .getByRole("button", {
          name: "Select workstation request dispatch-review-history-1",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      within(requestHistorySection).queryByText("request-review-history-11"),
    ).toBeNull();

    const revealAction = within(requestHistorySection).getByRole("button", {
      name: "Show 2 more requests",
    });
    revealAction.focus();
    fireEvent.click(revealAction);

    expect(within(requestHistoryList).getAllByRole("listitem")).toHaveLength(
      12,
    );
    expect(
      within(requestHistorySection).getByText("request-review-history-11"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection).getByText("request-review-history-12"),
    ).toBeTruthy();
    expect(
      within(requestHistorySection)
        .getByRole("button", {
          name: "Select workstation request dispatch-review-history-1",
        })
        .getAttribute("aria-pressed"),
    ).toBe("true");
    expect(
      within(requestHistorySection).queryByText(
        "Showing 10 of 12 requests. 2 remaining.",
      ),
    ).toBeNull();
    expect(
      within(requestHistorySection).queryByRole("button", {
        name: "Show 2 more requests",
      }),
    ).toBeNull();
    expect(document.activeElement).toBe(requestHistoryList);
  });
});
