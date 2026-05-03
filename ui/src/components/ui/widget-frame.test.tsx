import { render, screen, within } from "@testing-library/react";

import {
  DashboardWidgetFrame,
  DETAIL_CARD_CLASS,
  DETAIL_COPY_CLASS,
  EMPTY_STATE_CLASS,
  WIDGET_SUBTITLE_CLASS,
} from "./widget-frame";

describe("DashboardWidgetFrame", () => {
  it("renders the shared widget frame contract with dashboard copy styles intact", () => {
    render(
      <DashboardWidgetFrame title="Submit work" widgetId="submit-work">
        <p className={WIDGET_SUBTITLE_CLASS}>Queue a new request</p>
        <p className={DETAIL_COPY_CLASS}>Submissions stay inside the shared layout frame.</p>
        <div className={EMPTY_STATE_CLASS}>
          <h3>No active submission</h3>
        </div>
      </DashboardWidgetFrame>,
    );

    const card = screen.getByRole("article", { name: "Submit work" });
    const subtitle = within(card).getByText("Queue a new request");
    const bodyCopy = within(card).getByText("Submissions stay inside the shared layout frame.");
    const emptyHeading = within(card).getByRole("heading", { name: "No active submission" });

    expect(card.className).toContain("min-w-0");
    expect(card.className).toContain(DETAIL_CARD_CLASS);
    expect(subtitle.className).toContain(WIDGET_SUBTITLE_CLASS);
    expect(bodyCopy.className).toContain(DETAIL_COPY_CLASS);
    expect(emptyHeading.parentElement?.className).toContain(EMPTY_STATE_CLASS);
  });
});

