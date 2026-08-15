import { expect, userEvent, within } from "storybook/test";

import { buildDashboardWorkstationRequestFixture } from "../../../../../../components/dashboard/fixtures";
import { getWorkstationDetailMessages } from "../../../messages/workstation-detail";
import { WorkstationRequestHistorySection } from "../workstation-request-history-section";

const boundedRequestHistory = Array.from({ length: 12 }, (_, index) => {
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

export default {
  title: "Agent Factory/Dashboard/Workstation Request History",
  component: WorkstationRequestHistorySection,
};

export const BoundedHistory = {
  tags: ["test"],
  render: () => (
    <div style={{ maxWidth: "960px" }}>
      <WorkstationRequestHistorySection
        messages={getWorkstationDetailMessages("en")}
        now={Date.parse("2026-04-08T12:00:04Z")}
        requests={boundedRequestHistory}
        resetKey="review"
      />
    </div>
  ),
  play: async ({ canvasElement }: { canvasElement: HTMLElement }) => {
    const canvas = within(canvasElement);
    const requestHistorySection = (
      await canvas.findByRole("heading", { name: "Request history" })
    ).closest("section");
    if (!(requestHistorySection instanceof HTMLElement)) {
      throw new Error("expected request history section");
    }

    await userEvent.click(
      within(requestHistorySection).getByRole("button", { name: "Expand" }),
    );

    const requestHistoryList = within(requestHistorySection).getByRole("list");
    await expect(
      within(requestHistoryList).getAllByRole("listitem"),
    ).toHaveLength(10);
    await expect(
      within(requestHistorySection).getByText(
        "Showing 10 of 12 requests. 2 remaining.",
      ),
    ).toBeVisible();

    const revealAction = within(requestHistorySection).getByRole("button", {
      name: "Show 2 more requests",
    });
    revealAction.focus();
    await userEvent.keyboard("{Enter}");

    await expect(
      within(requestHistoryList).getAllByRole("listitem"),
    ).toHaveLength(12);
    await expect(
      within(requestHistorySection).getByText("request-review-history-12"),
    ).toBeVisible();
    expect(
      within(requestHistorySection).queryByRole("button", {
        name: "Show 2 more requests",
      }),
    ).toBeNull();
    expect(document.activeElement).toBe(requestHistoryList);
  },
};
